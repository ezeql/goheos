package heostest

import (
	"bufio"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

const separator = "\r\n"

// Device is a fake HEOS device backed by a real TCP server.
type Device struct {
	mu            sync.Mutex
	ln            net.Listener
	addr          string
	started       bool
	matchers      []*Matcher
	conns         []*connLog
	registerCalls []string // enable values of register_for_change_events calls
}

// connLog tracks one accepted connection.
type connLog struct {
	conn net.Conn

	mu                    sync.Mutex
	isRegisteredForEvents bool
}

// StartDevice starts a fake device on a dynamic local port and registers its
// shutdown with t.Cleanup. Like pyheos' MockHeosDevice, system/check_account
// is pre-registered with the signed-in fixture.
func StartDevice(t *testing.T) *Device {
	t.Helper()
	d := &Device{}
	if err := d.start("127.0.0.1:0"); err != nil {
		t.Fatalf("heostest: start device: %v", err)
	}
	t.Cleanup(d.Stop)
	return d
}

func (d *Device) start(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.ln = ln
	d.addr = ln.Addr().String()
	d.started = true
	d.matchers = append(d.matchers, &Matcher{
		command:  "system/check_account",
		fixtures: []string{"system.check_account"},
	})
	d.mu.Unlock()
	go d.acceptLoop(ln)
	return nil
}

func (d *Device) acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		log := &connLog{conn: conn}
		d.mu.Lock()
		if !d.started {
			d.mu.Unlock()
			_ = conn.Close()
			return
		}
		d.conns = append(d.conns, log)
		d.mu.Unlock()
		go d.handleConnection(log)
	}
}

// Address returns the "host:port" the device listens on; pass it as the host
// argument of goheos.Connect.
func (d *Device) Address() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.addr
}

// Stop closes the listener and all connections. Safe to call more than once.
func (d *Device) Stop() {
	d.mu.Lock()
	if !d.started {
		d.mu.Unlock()
		return
	}
	d.started = false
	ln := d.ln
	conns := d.conns
	d.conns = nil
	d.mu.Unlock()
	_ = ln.Close()
	for _, c := range conns {
		_ = c.conn.Close()
	}
}

// Restart simulates a device reboot: it drops all connections and, after a
// short delay, starts listening again on the same address. Registered
// matchers are preserved.
func (d *Device) Restart() {
	d.mu.Lock()
	addr := d.addr
	d.mu.Unlock()
	d.Stop()
	time.Sleep(100 * time.Millisecond)
	// The port was just released; retry briefly in case the OS lags.
	var err error
	for range 20 {
		if err = d.start(addr); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	panic(fmt.Sprintf("heostest: restart on %s: %v", addr, err))
}

// Register adds a matcher for a command. A nil args map accepts any
// arguments. The fixtures are returned in order on each match.
func (d *Device) Register(command string, args map[string]string, fixtures ...string) *Matcher {
	m := &Matcher{command: command, args: args, fixtures: fixtures}
	d.mu.Lock()
	d.matchers = append(d.matchers, m)
	d.mu.Unlock()
	return m
}

// RegisterReplace removes existing matchers for the command, then registers.
func (d *Device) RegisterReplace(command string, args map[string]string, fixtures ...string) *Matcher {
	d.mu.Lock()
	kept := d.matchers[:0]
	for _, m := range d.matchers {
		if m.command != command {
			kept = append(kept, m)
		}
	}
	d.matchers = kept
	d.mu.Unlock()
	return d.Register(command, args, fixtures...)
}

// AssertCommandCalled fails the test unless a matcher for the command (and
// args, when non-nil) exists and was hit.
func (d *Device) AssertCommandCalled(t *testing.T, command string, args map[string]string) {
	t.Helper()
	d.mu.Lock()
	matchers := append([]*Matcher(nil), d.matchers...)
	d.mu.Unlock()
	for _, m := range matchers {
		if m.matches(command, args, false) {
			m.AssertCalled(t)
			return
		}
	}
	t.Errorf("command was not registered: %s with args %v", command, args)
}

// RegisterForChangeEventsCalls returns the enable values received by
// system/register_for_change_events, in order.
func (d *Device) RegisterForChangeEventsCalls() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.registerCalls...)
}

// ConnectionCount returns how many connections the device has accepted since
// the last (re)start.
func (d *Device) ConnectionCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.conns)
}

// SendEvent pushes an event fixture through the connection registered for
// change events, applying {token} replacements first. It waits briefly for
// such a connection to exist.
func (d *Device) SendEvent(t *testing.T, fixture string, replacements map[string]string) {
	t.Helper()
	body := replaceTokens(Fixture(fixture), replacements)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if c := d.eventConnection(); c != nil {
			if _, err := c.conn.Write([]byte(body + separator)); err != nil {
				t.Fatalf("heostest: send event %s: %v", fixture, err)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("heostest: no connection registered for change events")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (d *Device) eventConnection() *connLog {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, c := range d.conns {
		c.mu.Lock()
		registered := c.isRegisteredForEvents
		c.mu.Unlock()
		if registered {
			return c
		}
	}
	return nil
}

func (d *Device) handleConnection(log *connLog) {
	reader := bufio.NewReader(log.conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			d.removeConn(log)
			return
		}
		line = strings.TrimRight(line, "\r\n")
		command, query := parseCommandLine(line)

		if m := d.findMatcher(command, query); m != nil {
			delay, replay, preEvents := m.modifiers()
			if delay > 0 {
				time.Sleep(delay)
			}
			for _, event := range preEvents {
				d.writeEventLocked(event)
			}
			responses := m.responses(query)
			for range replay {
				for _, response := range responses {
					if _, err := log.conn.Write([]byte(response + separator)); err != nil {
						d.removeConn(log)
						return
					}
				}
			}
			continue
		}

		var response string
		switch command {
		case "system/reboot":
			// Simulate a reboot: the device drops everything and comes back.
			go d.Restart()
			return
		case "system/register_for_change_events":
			enable := query["enable"]
			log.mu.Lock()
			log.isRegisteredForEvents = enable == "on"
			log.mu.Unlock()
			d.mu.Lock()
			d.registerCalls = append(d.registerCalls, enable)
			d.mu.Unlock()
			response = strings.ReplaceAll(
				Fixture("system.register_for_change_events"), "{enable}", enable)
		default:
			response = strings.ReplaceAll(
				Fixture("other.unknown_command"), "{command}", command)
			response = strings.ReplaceAll(
				response, "{full_command}", url.QueryEscape(line))
		}
		if _, err := log.conn.Write([]byte(response + separator)); err != nil {
			d.removeConn(log)
			return
		}
	}
}

func (d *Device) findMatcher(command string, query map[string]string) *Matcher {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, m := range d.matchers {
		if m.matches(command, query, true) {
			return m
		}
	}
	return nil
}

func (d *Device) writeEventLocked(fixture string) {
	if c := d.eventConnection(); c != nil {
		_, _ = c.conn.Write([]byte(Fixture(fixture) + separator))
	}
}

func (d *Device) removeConn(log *connLog) {
	_ = log.conn.Close()
	d.mu.Lock()
	defer d.mu.Unlock()
	for i, c := range d.conns {
		if c == log {
			d.conns = append(d.conns[:i], d.conns[i+1:]...)
			return
		}
	}
}

// parseCommandLine splits "heos://group/cmd?query" into the command name and
// its decoded query parameters (last value wins, like Python's parse_qsl).
func parseCommandLine(line string) (string, map[string]string) {
	u, err := url.Parse(line)
	if err != nil {
		return line, map[string]string{}
	}
	command := u.Host + u.Path
	query := map[string]string{}
	if u.RawQuery != "" {
		for pair := range strings.SplitSeq(u.RawQuery, "&") {
			if pair == "" {
				continue
			}
			key, value, _ := strings.Cut(pair, "=")
			if k, err := url.QueryUnescape(key); err == nil {
				key = k
			}
			if v, err := url.QueryUnescape(value); err == nil {
				value = v
			}
			query[key] = value
		}
	}
	return command, query
}
