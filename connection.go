package goheos

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

var separatorBytes = []byte(separator)

// pendingCommand tracks the single in-flight command awaiting its response.
type pendingCommand struct {
	command string
	params  map[string]string
	resp    chan *message // buffered(1)
}

// matches reports whether m answers this command. Responses carry no
// sequence id, so besides the name the player or group id is compared: a
// late reply to a timed-out get_volume for one player must not answer the
// next get_volume for another.
func (p *pendingCommand) matches(m *message) bool {
	if p.command != m.command {
		return false
	}
	for _, key := range []string{attrPlayerID, attrGroupID} {
		want, sent := p.params[key]
		got, echoed := m.message[key]
		// A list (group/set_group?pid=1,2,3) is not an id to compare.
		if sent && echoed && !strings.Contains(want, ",") && want != got {
			return false
		}
	}
	return true
}

// connection manages the TCP session with a HEOS device: framing, the
// one-command-at-a-time lock, response correlation, event debouncing and the
// heartbeat. Reconnect and failover live in reconnect.go.
type connection struct {
	timeout              time.Duration
	heartBeat            bool
	heartBeatInterval    time.Duration
	eventDebounce        time.Duration
	autoReconnect        bool
	reconnectDelay       time.Duration
	reconnectMaxAttempts int
	autoFailover         bool
	logger               *slog.Logger

	// Callbacks are set once before connect and invoked from connection
	// goroutines. onEvent is called sequentially in socket order.
	onEvent     func(*message)
	onConnected func()
	// onDisconnected reports whether a reconnect or failover was started;
	// onRecoveryFailed fires when it gives up without being stopped.
	onDisconnected   func(fromError, recovering bool)
	onRecoveryFailed func()
	onCommandError   func(*CommandFailedError)

	// commandSem serializes commands: the protocol has no sequence ids, so
	// responses are correlated by command name, one in flight at a time. A
	// channel rather than a mutex, so waiting for it honors the caller's ctx.
	commandSem chan struct{}

	mu            sync.Mutex
	host          string
	failoverHosts []string
	state         ConnectionState
	conn          net.Conn
	done          chan struct{} // closed on reset; nil while disconnected
	recoveryStop  chan struct{} // closed to abort reconnect/failover
	lastActivity  time.Time
	pending       *pendingCommand
	debouncers    map[EventCommand]*time.Timer
	events        *eventQueue
	wg            sync.WaitGroup
}

func newConnection(host string, o *options) *connection {
	return &connection{
		timeout:              o.timeout,
		heartBeat:            o.heartBeat,
		heartBeatInterval:    o.heartBeatInterval,
		eventDebounce:        o.eventDebounce,
		autoReconnect:        o.autoReconnect,
		reconnectDelay:       o.autoReconnectDelay,
		reconnectMaxAttempts: o.autoReconnectMaxAttempts,
		autoFailover:         o.autoFailover,
		failoverHosts:        append([]string(nil), o.autoFailoverHosts...),
		logger:               o.logger,
		host:                 host,
		state:                StateDisconnected,
		debouncers:           map[EventCommand]*time.Timer{},
		commandSem:           make(chan struct{}, 1),
	}
}

// hostAddr appends the default CLI port when the host has none.
func hostAddr(host string) string {
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}
	return net.JoinHostPort(host, strconv.Itoa(DefaultPort))
}

func (c *connection) currentState() ConnectionState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

func (c *connection) currentHost() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.host
}

func (c *connection) touchActivity() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastActivity = time.Now()
}

// connect establishes the TCP session and starts the background goroutines.
func (c *connection) connect(ctx context.Context) error {
	c.mu.Lock()
	if c.state == StateConnected {
		c.mu.Unlock()
		return nil
	}
	host := c.host
	c.mu.Unlock()

	dialer := net.Dialer{Timeout: c.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", hostAddr(host))
	if err != nil {
		c.logger.Debug("failed to connect", "host", host, "error", err)
		return fmt.Errorf("goheos: unable to connect to %s: %w", host, err)
	}

	done := make(chan struct{})
	queue := newEventQueue()
	c.mu.Lock()
	c.conn = conn
	c.done = done
	c.events = queue
	c.lastActivity = time.Now()
	c.state = StateConnected
	c.mu.Unlock()

	c.wg.Add(2)
	go c.readLoop(conn, done)
	go c.eventPump(queue)
	if c.heartBeat {
		c.wg.Add(1)
		//nolint:gosec // the heartbeat outlives the connect call by design
		go c.heartBeatLoop(done)
	}
	c.logger.Debug("connected", "host", host)
	if c.onConnected != nil {
		c.onConnected()
	}
	return nil
}

// disconnect closes the session deliberately, aborting any reconnect or
// failover in progress. Safe to call when already disconnected.
func (c *connection) disconnect() {
	recovering := c.stopRecovery()
	if c.reset() || recovering {
		c.logger.Debug("disconnected", "host", c.currentHost())
		if c.onDisconnected != nil {
			c.onDisconnected(false, false)
		}
	}
}

func (c *connection) disconnectFromError(err error) {
	if c.reset() {
		c.logger.Debug("disconnected due to error", "host", c.currentHost(), "error", err)
		recovering := c.startRecovery()
		if c.onDisconnected != nil {
			c.onDisconnected(true, recovering)
		}
	}
}

// reset tears down the session state. It reports whether an active session
// was torn down, so racing error paths trigger callbacks only once.
func (c *connection) reset() bool {
	c.mu.Lock()
	active := c.conn != nil
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
	if c.done != nil {
		close(c.done)
		c.done = nil
	}
	for _, t := range c.debouncers {
		t.Stop()
	}
	clear(c.debouncers)
	if c.events != nil {
		c.events.close()
		c.events = nil
	}
	c.pending = nil
	c.lastActivity = time.Now()
	c.state = StateDisconnected
	c.mu.Unlock()
	return active
}

// command sends a command and waits for its response. The effective timeout
// is the smaller of ctx and the configured timeout.
func (c *connection) command(ctx context.Context, cmd heosCommand) (*message, error) {
	resp, err := c.execute(ctx, cmd)
	if err != nil {
		var failed *CommandFailedError
		// The lock is already released here, so the handler may issue
		// commands (e.g. an automatic sign-out).
		if errors.As(err, &failed) && c.onCommandError != nil {
			c.onCommandError(failed)
		}
	}
	return resp, err
}

func (c *connection) execute(ctx context.Context, cmd heosCommand) (*message, error) {
	select {
	case c.commandSem <- struct{}{}:
		defer func() { <-c.commandSem }()
	case <-ctx.Done():
		return nil, &CommandError{Command: cmd.name, Err: ctx.Err()}
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	c.mu.Lock()
	if c.state != StateConnected || c.conn == nil {
		c.mu.Unlock()
		c.logger.Debug("command failed: not connected", "command", cmd.uri(true))
		return nil, &CommandError{Command: cmd.name, Err: ErrNotConnected}
	}
	conn := c.conn
	done := c.done
	var pending *pendingCommand
	if cmd.name != cmdReboot {
		// Register before writing so a fast response cannot slip past the
		// waiter.
		pending = &pendingCommand{command: cmd.name, params: cmd.parameters, resp: make(chan *message, 1)}
		c.pending = pending
	}
	c.lastActivity = time.Now()
	c.mu.Unlock()

	// A caller that gave up is not a broken connection: return before the
	// write, whose failure tears the session down for every caller.
	if err := ctx.Err(); err != nil {
		c.clearPending(pending)
		return nil, &CommandError{Command: cmd.name, Err: err}
	}
	// The write deadline is the connection's own timeout, never the caller's
	// ctx: only a socket that cannot take a few bytes in that long is dead.
	_ = conn.SetWriteDeadline(time.Now().Add(c.timeout))
	if _, err := conn.Write([]byte(cmd.uri(false) + separator)); err != nil {
		c.clearPending(pending)
		c.logger.Debug("command failed", "command", cmd.uri(true), "error", err)
		// Reset in the background to ensure the connection is torn down.
		go c.disconnectFromError(err)
		return nil, &CommandError{Command: cmd.name, Err: err}
	}

	// A reboot never responds; return a synthetic success immediately.
	if cmd.name == cmdReboot {
		c.logger.Debug("command executed, no response expected", "command", cmd.uri(true))
		return &message{command: cmdReboot, result: true, message: map[string]string{}}, nil
	}

	select {
	case resp := <-pending.resp:
		if !resp.result {
			c.logger.Debug("command failed", "command", cmd.uri(true), "response", resp.raw)
			return nil, newCommandFailedError(resp)
		}
		c.logger.Debug("command executed", "command", cmd.uri(true))
		return resp, nil
	case <-ctx.Done():
		c.clearPending(pending)
		c.logger.Debug("command timed out", "command", cmd.uri(true))
		return nil, &CommandError{Command: cmd.name, Err: ctx.Err()}
	case <-done:
		c.clearPending(pending)
		return nil, &CommandError{Command: cmd.name, Err: ErrDisconnected}
	}
}

func (c *connection) clearPending(p *pendingCommand) {
	if p == nil {
		return
	}
	c.mu.Lock()
	if c.pending == p {
		c.pending = nil
	}
	c.mu.Unlock()
}

// scanCRLF splits frames on the CRLF sequence. Fixtures and device payloads
// may span multiple lines, so a bare newline is not a frame boundary.
func scanCRLF(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.Index(data, separatorBytes); i >= 0 {
		return i + len(separatorBytes), data[:i], nil
	}
	if atEOF {
		// Drop a partial frame at EOF; the read loop treats it as a lost
		// connection, like pyheos' IncompleteReadError.
		return 0, nil, nil
	}
	return 0, nil, nil
}

func (c *connection) readLoop(conn net.Conn, done chan struct{}) {
	defer c.wg.Done()
	scanner := bufio.NewScanner(conn)
	// A large explicit ceiling: big browse responses can exceed the 64KiB
	// default (a latent pyheos bug this port fixes).
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	scanner.Split(scanCRLF)
	for scanner.Scan() {
		c.touchActivity()
		m, err := decodeMessage(scanner.Bytes())
		if err != nil {
			c.logger.Warn("discarding undecodable message", "error", err)
			continue
		}
		c.handleMessage(m)
	}
	err := scanner.Err()
	if err == nil {
		err = io.EOF
	}
	select {
	case <-done:
		// Deliberate reset; no error path.
	default:
		c.disconnectFromError(err)
	}
}

func (c *connection) handleMessage(m *message) {
	if m.isUnderProcess() {
		c.logger.Debug("command under process", "command", m.command)
		return
	}
	if m.isEvent() {
		c.handleEvent(m)
		return
	}
	c.mu.Lock()
	if p := c.pending; p != nil && p.matches(m) {
		c.pending = nil
		c.mu.Unlock()
		p.resp <- m
		return
	}
	c.mu.Unlock()
	c.logger.Debug("unexpected response received", "command", m.command)
}

func (c *connection) handleEvent(m *message) {
	event := EventCommand(m.command)
	c.mu.Lock()
	// browse/get_music_sources?refresh=on floods sources_changed events
	// while it runs; suppress them.
	if event == EventSourcesChanged && c.pending != nil && c.pending.command == cmdBrowseGetSources {
		c.mu.Unlock()
		c.logger.Debug("ignored event during source refresh", "event", m.command)
		return
	}
	queue := c.events
	if queue == nil {
		c.mu.Unlock()
		return
	}
	if !debouncedEvents[event] {
		c.mu.Unlock()
		queue.push(m)
		return
	}
	// Debounced event: cancel the pending timer and rearm. The device emits
	// these in bursts and only the last one matters.
	if t, ok := c.debouncers[event]; ok {
		t.Stop()
		c.logger.Debug("debounced event", "event", m.command)
	}
	c.debouncers[event] = time.AfterFunc(c.eventDebounce, func() {
		queue.push(m)
	})
	c.mu.Unlock()
}

// eventPump delivers queued events to onEvent one at a time, preserving
// socket order without ever blocking the read loop.
func (c *connection) eventPump(queue *eventQueue) {
	defer c.wg.Done()
	for {
		m, ok := queue.pop()
		if !ok {
			return
		}
		if c.onEvent != nil {
			c.onEvent(m)
		}
	}
}

func (c *connection) heartBeatLoop(done chan struct{}) {
	defer c.wg.Done()
	ticker := time.NewTicker(c.heartBeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			c.mu.Lock()
			idle := time.Since(c.lastActivity)
			c.mu.Unlock()
			// Only ping after a quiet interval; regular traffic already
			// proves the connection is alive.
			if idle >= c.heartBeatInterval {
				_, _ = c.command(context.Background(), newCommand(cmdHeartBeat))
			}
		}
	}
}

// eventQueue is an unbounded FIFO with a single consumer.
type eventQueue struct {
	mu     sync.Mutex
	items  []*message
	signal chan struct{}
	closed bool
}

func newEventQueue() *eventQueue {
	return &eventQueue{signal: make(chan struct{}, 1)}
}

func (q *eventQueue) push(m *message) {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.items = append(q.items, m)
	q.mu.Unlock()
	select {
	case q.signal <- struct{}{}:
	default:
	}
}

// pop blocks until an item is available or the queue is closed. Items queued
// at close time are dropped, mirroring pyheos cancelling pending event tasks
// on reset.
func (q *eventQueue) pop() (*message, bool) {
	for {
		q.mu.Lock()
		if q.closed {
			q.mu.Unlock()
			return nil, false
		}
		if len(q.items) > 0 {
			m := q.items[0]
			q.items = q.items[1:]
			q.mu.Unlock()
			return m, true
		}
		q.mu.Unlock()
		<-q.signal
	}
}

func (q *eventQueue) close() {
	q.mu.Lock()
	q.closed = true
	q.items = nil
	q.mu.Unlock()
	select {
	case q.signal <- struct{}{}:
	default:
	}
}
