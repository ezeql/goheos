package goheos

import (
	"log/slog"
	"time"
)

// Defaults for Client options.
const (
	DefaultTimeout           = 15 * time.Second
	DefaultReconnectDelay    = 1 * time.Second
	DefaultHeartBeatInterval = 10 * time.Second
	DefaultStep              = 5

	maxReconnectDelay    = 600 * time.Second
	defaultEventDebounce = 1 * time.Second
)

// Credentials holds a HEOS account user name and password.
type Credentials struct {
	Username string
	Password string
}

type options struct {
	timeout                  time.Duration
	events                   bool
	allProgressEvents        bool
	autoReconnect            bool
	autoReconnectDelay       time.Duration
	autoReconnectMaxAttempts int
	heartBeat                bool
	heartBeatInterval        time.Duration
	credentials              *Credentials
	autoFailover             bool
	autoFailoverHosts        []string
	eventDebounce            time.Duration
	logger                   *slog.Logger
}

func defaultOptions() options {
	return options{
		timeout:            DefaultTimeout,
		events:             true,
		allProgressEvents:  true,
		autoReconnectDelay: DefaultReconnectDelay,
		heartBeat:          true,
		heartBeatInterval:  DefaultHeartBeatInterval,
		eventDebounce:      defaultEventDebounce,
		logger:             slog.New(slog.DiscardHandler),
	}
}

// Option configures a Client.
type Option func(*options)

// WithTimeout sets the timeout for connection establishment and command
// responses. The default is DefaultTimeout.
func WithTimeout(d time.Duration) Option {
	return func(o *options) { o.timeout = d }
}

// WithoutEvents disables registration for change events: the client will not
// receive state updates from the device.
func WithoutEvents() Option {
	return func(o *options) { o.events = false }
}

// WithChangedProgressEventsOnly processes only the first now-playing progress
// event after each media change instead of every progress tick.
func WithChangedProgressEventsOnly() Option {
	return func(o *options) { o.allProgressEvents = false }
}

// WithCredentials signs in with the HEOS account on every (re)connection.
func WithCredentials(username, password string) Option {
	return func(o *options) {
		o.credentials = &Credentials{Username: username, Password: password}
	}
}

// WithHeartbeat sets the heartbeat interval. A heartbeat command is sent only
// after an interval with no traffic. The default is DefaultHeartBeatInterval.
func WithHeartbeat(interval time.Duration) Option {
	return func(o *options) {
		o.heartBeat = true
		o.heartBeatInterval = interval
	}
}

// WithoutHeartbeat disables the connection heartbeat.
func WithoutHeartbeat() Option {
	return func(o *options) { o.heartBeat = false }
}

// WithAutoReconnect enables automatic reconnection after a connection failure
// with exponential backoff starting at delay (capped at 10 minutes).
// maxAttempts zero means unlimited attempts.
func WithAutoReconnect(delay time.Duration, maxAttempts int) Option {
	return func(o *options) {
		o.autoReconnect = true
		o.autoReconnectDelay = delay
		o.autoReconnectMaxAttempts = maxAttempts
	}
}

// WithAutoFailover rotates through backup hosts when the connection fails.
// With no hosts given, the backup list is discovered from the HEOS system
// itself and kept up to date as players change.
func WithAutoFailover(hosts ...string) Option {
	return func(o *options) {
		o.autoFailover = true
		o.autoFailoverHosts = hosts
	}
}

// WithEventDebounce overrides the debounce delay applied to bursty system
// events (players/sources/groups/user changed). Intended for tests.
func WithEventDebounce(d time.Duration) Option {
	return func(o *options) { o.eventDebounce = d }
}

// WithLogger sets the logger for protocol traces and background errors. The
// default discards all logs.
func WithLogger(l *slog.Logger) Option {
	return func(o *options) { o.logger = l }
}
