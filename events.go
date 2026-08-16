package goheos

import (
	"slices"
	"sync"
)

// Event is a notification delivered through Client.Subscribe. The concrete
// types are ConnectedEvent, DisconnectedEvent, UserCredentialsInvalidEvent,
// ControllerEvent, PlayerEvent and GroupEvent.
type Event interface{ isEvent() }

// ConnectedEvent signals the client (re)connected to a device.
type ConnectedEvent struct{}

// DisconnectedEvent signals the connection was closed. With FromError set the
// disconnection was unexpected; when auto reconnect or failover is enabled
// the subscription stays open and a ConnectedEvent follows once restored.
type DisconnectedEvent struct{ FromError bool }

// UserCredentialsInvalidEvent signals the stored HEOS account credentials
// were rejected by the service.
type UserCredentialsInvalidEvent struct{}

// ControllerEvent signals a system-wide change (players, groups, sources or
// user changed). Update is non-nil only for EventPlayersChanged.
type ControllerEvent struct {
	Command EventCommand
	Update  *PlayerUpdateResult
}

// PlayerEvent signals a change on an individual player. The player's state is
// already updated when the event is delivered.
type PlayerEvent struct {
	PlayerID int
	Command  EventCommand
}

// GroupEvent signals a change on an individual group. The group's state is
// already updated when the event is delivered.
type GroupEvent struct {
	GroupID int
	Command EventCommand
}

func (ConnectedEvent) isEvent()              {}
func (DisconnectedEvent) isEvent()           {}
func (UserCredentialsInvalidEvent) isEvent() {}
func (ControllerEvent) isEvent()             {}
func (PlayerEvent) isEvent()                 {}
func (GroupEvent) isEvent()                  {}

type subscribeOptions struct {
	filter     func(Event) bool
	bound      int
	dropOldest bool
}

// SubscribeOption configures a subscription.
type SubscribeOption func(*subscribeOptions)

// WithEventFilter delivers only events for which f returns true.
func WithEventFilter(f func(Event) bool) SubscribeOption {
	return func(o *subscribeOptions) { o.filter = f }
}

// WithPlayerEvents delivers only PlayerEvent values, optionally restricted to
// the given player ids.
func WithPlayerEvents(playerIDs ...int) SubscribeOption {
	return WithEventFilter(func(e Event) bool {
		pe, ok := e.(PlayerEvent)
		if !ok {
			return false
		}
		return len(playerIDs) == 0 || slices.Contains(playerIDs, pe.PlayerID)
	})
}

// WithGroupEvents delivers only GroupEvent values, optionally restricted to
// the given group ids.
func WithGroupEvents(groupIDs ...int) SubscribeOption {
	return WithEventFilter(func(e Event) bool {
		ge, ok := e.(GroupEvent)
		if !ok {
			return false
		}
		return len(groupIDs) == 0 || slices.Contains(groupIDs, ge.GroupID)
	})
}

// WithBoundedBuffer bounds the subscription's queue to n events. When full,
// dropOldest discards the oldest queued event to make room; otherwise the new
// event is discarded. The default queue is unbounded so no event is ever
// lost while the consumer keeps up.
func WithBoundedBuffer(n int, dropOldest bool) SubscribeOption {
	return func(o *subscribeOptions) {
		o.bound = n
		o.dropOldest = dropOldest
	}
}

// Subscribe returns a channel of events and a cancel function. The channel is
// closed by cancel and by Client.Disconnect; on an unexpected disconnection
// with auto reconnect or failover enabled it stays open across reconnects.
// Events are delivered in order; the client's state is already updated when
// an event is received.
func (c *Client) Subscribe(opts ...SubscribeOption) (<-chan Event, func()) {
	var o subscribeOptions
	for _, opt := range opts {
		opt(&o)
	}
	s := &subscriber{
		ch:         make(chan Event),
		signal:     make(chan struct{}, 1),
		done:       make(chan struct{}),
		filter:     o.filter,
		bound:      o.bound,
		dropOldest: o.dropOldest,
	}
	c.subMu.Lock()
	c.subscribers = append(c.subscribers, s)
	c.subMu.Unlock()
	go s.pump()
	cancel := func() {
		c.removeSubscriber(s)
		s.cancel()
	}
	return s.ch, cancel
}

func (c *Client) removeSubscriber(target *subscriber) {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	for i, s := range c.subscribers {
		if s == target {
			c.subscribers = append(c.subscribers[:i], c.subscribers[i+1:]...)
			return
		}
	}
}

// emit delivers an event to every subscriber without blocking.
func (c *Client) emit(e Event) {
	c.subMu.Lock()
	subscribers := append([]*subscriber(nil), c.subscribers...)
	c.subMu.Unlock()
	for _, s := range subscribers {
		s.push(e)
	}
}

// closeSubscribers gracefully shuts down all subscriptions: queued events are
// delivered, then each channel is closed.
func (c *Client) closeSubscribers() {
	c.subMu.Lock()
	subscribers := c.subscribers
	c.subscribers = nil
	c.subMu.Unlock()
	for _, s := range subscribers {
		s.shutdown()
	}
}

// subscriber owns one subscription: an unbounded (or bounded, opt-in) FIFO
// drained by a pump goroutine into the consumer channel. The dispatcher never
// blocks on a slow consumer.
type subscriber struct {
	ch     chan Event
	signal chan struct{}
	done   chan struct{}

	filter     func(Event) bool
	bound      int
	dropOldest bool

	mu       sync.Mutex
	queue    []Event
	closing  bool
	canceled bool
	dropped  int
}

func (s *subscriber) push(e Event) {
	if s.filter != nil && !s.filter(e) {
		return
	}
	s.mu.Lock()
	if s.closing || s.canceled {
		s.mu.Unlock()
		return
	}
	if s.bound > 0 && len(s.queue) >= s.bound {
		if !s.dropOldest {
			s.dropped++
			s.mu.Unlock()
			return
		}
		s.queue = s.queue[1:]
		s.dropped++
	}
	s.queue = append(s.queue, e)
	s.mu.Unlock()
	select {
	case s.signal <- struct{}{}:
	default:
	}
}

// cancel tears the subscription down immediately, discarding queued events.
func (s *subscriber) cancel() {
	s.mu.Lock()
	if s.canceled {
		s.mu.Unlock()
		return
	}
	s.canceled = true
	s.queue = nil
	s.mu.Unlock()
	close(s.done)
}

// shutdown closes the subscription after the queue drains.
func (s *subscriber) shutdown() {
	s.mu.Lock()
	s.closing = true
	s.mu.Unlock()
	select {
	case s.signal <- struct{}{}:
	default:
	}
}

// pump delivers queued events to the channel in order. It is the only
// goroutine that closes ch.
func (s *subscriber) pump() {
	for {
		s.mu.Lock()
		if s.canceled {
			s.mu.Unlock()
			close(s.ch)
			return
		}
		if len(s.queue) > 0 {
			e := s.queue[0]
			s.queue = s.queue[1:]
			s.mu.Unlock()
			select {
			case s.ch <- e:
				continue
			case <-s.done:
				close(s.ch)
				return
			}
		}
		closing := s.closing
		s.mu.Unlock()
		if closing {
			close(s.ch)
			return
		}
		select {
		case <-s.signal:
		case <-s.done:
		}
	}
}
