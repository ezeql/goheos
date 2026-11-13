package goheos

import (
	"context"
	"errors"
	"slices"
	"sync"
)

// Client controls a HEOS system through a single device connection. Create
// one with New or Connect; all commands take a context and are safe for
// concurrent use (the protocol serializes them, one in flight at a time).
type Client struct {
	opts options
	conn *connection

	mu                 sync.RWMutex
	players            map[int]*Player
	playersLoaded      bool
	groups             map[int]*Group
	groupsLoaded       bool
	musicSources       map[int]MusicSource
	musicSourcesLoaded bool
	signedInUsername   string
	credentials        *Credentials
	failoverHosts      []string

	subMu       sync.Mutex
	subscribers []*subscriber
}

// New creates a client without connecting. host is an IP address or host
// name, optionally with a port ("192.168.1.10" or "192.168.1.10:1255").
func New(host string, opts ...Option) *Client {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	c := &Client{
		opts:         o,
		players:      map[int]*Player{},
		groups:       map[int]*Group{},
		musicSources: map[int]MusicSource{},
		credentials:  o.credentials,
	}
	c.conn = newConnection(host, &o)
	c.conn.onConnected = c.onConnected
	c.conn.onDisconnected = c.onDisconnected
	c.conn.onRecoveryFailed = c.closeSubscribers
	c.conn.onEvent = c.onEvent
	c.conn.onCommandError = c.onCommandError
	return c
}

// Connect creates a client and connects to the device.
func Connect(ctx context.Context, host string, opts ...Option) (*Client, error) {
	c := New(host, opts...)
	if err := c.Connect(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

// ValidateConnection connects to a device without events or heartbeat,
// retrieves the system information, and disconnects.
func ValidateConnection(ctx context.Context, host string) (*System, error) {
	c := New(host, WithoutEvents(), WithoutHeartbeat())
	if err := c.Connect(ctx); err != nil {
		return nil, err
	}
	defer func() { _ = c.Disconnect() }()
	return c.GetSystemInfo(ctx)
}

// Connect establishes the connection. On success the client has signed in
// (when credentials are set), registered for change events, and refreshed any
// previously loaded players and groups.
func (c *Client) Connect(ctx context.Context) error {
	return c.conn.connect(ctx)
}

// Disconnect closes the connection, delivers a DisconnectedEvent and closes
// all subscription channels. It is safe to call more than once and from an
// event consumer.
func (c *Client) Disconnect() error {
	c.conn.disconnect()
	// Already disconnected (e.g. recovery gave up) means no callback ran.
	c.closeSubscribers()
	return nil
}

// ConnectionState returns the current connection state.
func (c *Client) ConnectionState() ConnectionState { return c.conn.currentState() }

// Host returns the host currently connected (or configured); it can change
// when auto failover is enabled.
func (c *Client) Host() string { return c.conn.currentHost() }

// IsSignedIn reports whether a HEOS account is signed in.
func (c *Client) IsSignedIn() bool { return c.SignedInUsername() != "" }

// SignedInUsername returns the signed-in account name, or the empty string.
func (c *Client) SignedInUsername() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.signedInUsername
}

// CurrentCredentials returns the stored account credentials, if any.
func (c *Client) CurrentCredentials() *Credentials {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.credentials == nil {
		return nil
	}
	creds := *c.credentials
	return &creds
}

// SetCredentials replaces the stored account credentials used to sign in on
// reconnection. Pass nil to clear them.
func (c *Client) SetCredentials(credentials *Credentials) { c.setCredentials(credentials) }

func (c *Client) setCredentials(credentials *Credentials) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if credentials == nil {
		c.credentials = nil
		return
	}
	creds := *credentials
	c.credentials = &creds
}

func (c *Client) setSignedInUsername(username string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.signedInUsername = username
}

// FailoverHosts returns the current backup host list used by auto failover.
func (c *Client) FailoverHosts() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]string(nil), c.failoverHosts...)
}

func (c *Client) setFailoverHosts(hosts []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failoverHosts = hosts
}

// onConnected runs on every (re)connection: sign in or check the account,
// update failover hosts, re-register for change events and refresh loaded
// players and groups. Mirrors pyheos Heos._on_connected.
func (c *Client) onConnected() {
	ctx := context.Background()
	events := []Event{ConnectedEvent{}}

	if creds := c.CurrentCredentials(); creds != nil {
		err := c.signIn(ctx, creds.Username, creds.Password, true)
		if errors.Is(err, ErrAuthenticationFailure) {
			c.opts.logger.Debug("failed to sign in to HEOS account after connection", "error", err)
			c.setCredentials(nil)
			events = append(events, UserCredentialsInvalidEvent{})
		} else if err != nil {
			c.opts.logger.Warn("sign-in failed after connection", "error", err)
		}
	} else if _, _, err := c.CheckAccount(ctx); err != nil {
		c.opts.logger.Warn("account check failed after connection", "error", err)
	}

	c.updateFailoverHosts(ctx)

	if err := c.RegisterForChangeEvents(ctx, c.opts.events); err != nil {
		c.opts.logger.Warn("failed to register for change events", "error", err)
	}

	c.mu.RLock()
	playersLoaded, groupsLoaded := c.playersLoaded, c.groupsLoaded
	c.mu.RUnlock()
	if playersLoaded {
		update, err := c.LoadPlayers(ctx)
		if err != nil {
			c.opts.logger.Warn("failed to refresh players after connection", "error", err)
		} else {
			events = append(events, ControllerEvent{Command: EventPlayersChanged, Update: update})
		}
	}
	if groupsLoaded {
		if err := c.RefreshGroups(ctx); err != nil {
			c.opts.logger.Warn("failed to refresh groups after connection", "error", err)
		} else {
			events = append(events, ControllerEvent{Command: EventGroupsChanged})
		}
	}

	for _, e := range events {
		c.emit(e)
	}
}

// onDisconnected marks loaded players unavailable and notifies subscribers.
// The subscription channels stay open only while a reconnect or failover is
// actually running.
func (c *Client) onDisconnected(fromError, recovering bool) {
	for _, player := range c.Players() {
		player.setAvailable(false)
	}
	c.emit(DisconnectedEvent{FromError: fromError})
	if !recovering {
		c.closeSubscribers()
	}
}

// onCommandError signs out and notifies subscribers when a command reports an
// authentication failure, so stale credentials are cleared.
func (c *Client) onCommandError(err *CommandFailedError) {
	if !errors.Is(err, ErrAuthenticationFailure) {
		return
	}
	if err.Command == cmdSignIn || err.Command == cmdSignOut {
		return
	}
	_ = c.SignOut(context.Background())
	c.emit(UserCredentialsInvalidEvent{})
}

// updateFailoverHosts refreshes the backup host list from the system when
// auto failover is enabled without an explicit host list.
func (c *Client) updateFailoverHosts(ctx context.Context) {
	if !c.opts.autoFailover || len(c.opts.autoFailoverHosts) > 0 {
		return
	}
	info, err := c.GetSystemInfo(ctx)
	if err != nil {
		c.opts.logger.Warn("failed to update failover hosts", "error", err)
		return
	}
	hosts := info.IPAddresses()
	current := hostOnly(c.conn.currentHost())
	if i := slices.Index(hosts, current); i >= 0 {
		hosts = slices.Delete(hosts, i, i+1)
	}
	c.setFailoverHosts(hosts)
	c.conn.setFailoverHosts(hosts)
}

// onEvent routes a device event: the matching model is updated first, then
// subscribers are notified. Runs on a single goroutine in socket order.
func (c *Client) onEvent(m *message) {
	event := EventCommand(m.command)
	switch {
	case heosEvents[event]:
		c.onHeosEvent(m, event)
	case playerEvents[event]:
		c.onPlayerEvent(m, event)
	case groupEvents[event]:
		c.onGroupEvent(m, event)
	default:
		c.opts.logger.Debug("unrecognized event", "event", m.command)
	}
}

func (c *Client) onHeosEvent(m *message, event EventCommand) {
	ctx := context.Background()
	var update *PlayerUpdateResult
	c.mu.RLock()
	playersLoaded, groupsLoaded, sourcesLoaded := c.playersLoaded, c.groupsLoaded, c.musicSourcesLoaded
	c.mu.RUnlock()

	switch event {
	case EventPlayersChanged:
		c.updateFailoverHosts(ctx)
		if playersLoaded {
			var err error
			if update, err = c.LoadPlayers(ctx); err != nil {
				c.opts.logger.Warn("failed to refresh players", "error", err)
			}
		}
	case EventSourcesChanged:
		if sourcesLoaded {
			if err := c.RefreshMusicSources(ctx); err != nil {
				c.opts.logger.Warn("failed to refresh music sources", "error", err)
			}
		}
	case EventUserChanged:
		if _, ok := m.message[attrSignedIn]; ok {
			c.setSignedInUsername(m.message[attrUserName])
		} else {
			c.setSignedInUsername("")
			// HEOS does not send a sources changed event upon logout.
			if sourcesLoaded {
				if err := c.RefreshMusicSources(ctx); err != nil {
					c.opts.logger.Warn("failed to refresh music sources", "error", err)
				}
			}
		}
	case EventGroupsChanged:
		// Group ids on players are not evented; refresh them too.
		if playersLoaded {
			if err := c.refreshPlayersBaseInfo(ctx); err != nil {
				c.opts.logger.Warn("failed to refresh player info", "error", err)
			}
		}
		if groupsLoaded {
			if err := c.RefreshGroups(ctx); err != nil {
				c.opts.logger.Warn("failed to refresh groups", "error", err)
			}
		}
	}
	c.emit(ControllerEvent{Command: event, Update: update})
}

func (c *Client) onPlayerEvent(m *message, event EventCommand) {
	playerID, err := m.intValue(attrPlayerID)
	if err != nil {
		return
	}
	c.mu.RLock()
	player := c.players[playerID]
	c.mu.RUnlock()
	if player == nil {
		return
	}
	if player.onHeosEvent(m, c.opts.allProgressEvents) {
		c.emit(PlayerEvent{PlayerID: playerID, Command: event})
	}
}

func (c *Client) onGroupEvent(m *message, event EventCommand) {
	groupID, err := m.intValue(attrGroupID)
	if err != nil {
		return
	}
	c.mu.RLock()
	group := c.groups[groupID]
	c.mu.RUnlock()
	if group == nil {
		return
	}
	if group.onHeosEvent(m) {
		c.emit(GroupEvent{GroupID: groupID, Command: event})
	}
}
