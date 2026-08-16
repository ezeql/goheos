package goheos

import (
	"context"
	"time"
)

// Recovery: automatic reconnect and failover after an unexpected disconnect.
// Mirrors pyheos AutoReconnectingBehavior/AutoFailoverConnection: reconnect
// backs off exponentially (×2, capped at 10 minutes); failover rotates
// through the backup hosts with one attempt per host, forever.

func (c *connection) setHost(host string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.host = host
}

func (c *connection) setFailoverHosts(hosts []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failoverHosts = append([]string(nil), hosts...)
}

// startRecovery spawns the reconnect or failover goroutine after an
// error-disconnect, when enabled. Returns whether recovery started.
func (c *connection) startRecovery() bool {
	c.mu.Lock()
	failoverHosts := append([]string(nil), c.failoverHosts...)
	useFailover := c.autoFailover && len(failoverHosts) > 0
	useReconnect := c.autoReconnect
	if !useFailover && !useReconnect {
		c.mu.Unlock()
		return false
	}
	c.state = StateReconnecting
	stop := make(chan struct{})
	c.recoveryStop = stop
	currentHost := c.host
	c.mu.Unlock()

	go func() {
		defer func() {
			c.mu.Lock()
			if c.recoveryStop == stop {
				c.recoveryStop = nil
			}
			c.mu.Unlock()
		}()
		if useFailover {
			c.attemptFailover(stop, currentHost, failoverHosts)
			return
		}
		if !c.attemptReconnect(stop, c.reconnectDelay, c.reconnectMaxAttempts) {
			// Gave up (or was stopped): leave the reconnecting state.
			c.mu.Lock()
			if c.state == StateReconnecting {
				c.state = StateDisconnected
			}
			c.mu.Unlock()
		}
	}()
	return true
}

// stopRecovery aborts a recovery in progress. Returns whether one was
// running.
func (c *connection) stopRecovery() bool {
	c.mu.Lock()
	stop := c.recoveryStop
	c.recoveryStop = nil
	c.mu.Unlock()
	if stop != nil {
		close(stop)
		return true
	}
	return false
}

// attemptReconnect retries the current host with exponential backoff.
// maxAttempts zero means unlimited. Reports whether the connection was
// re-established.
func (c *connection) attemptReconnect(stop chan struct{}, delay time.Duration, maxAttempts int) bool {
	attempts := 0
	if delay > maxReconnectDelay {
		delay = maxReconnectDelay
	}
	for maxAttempts == 0 || attempts < maxAttempts {
		if delay > 0 {
			c.logger.Debug("waiting before reconnect attempt", "delay", delay)
			select {
			case <-time.After(delay):
			case <-stop:
				return false
			}
		}
		c.logger.Debug("attempting reconnect", "attempt", attempts+1, "host", c.currentHost())
		if err := c.connect(context.Background()); err != nil {
			attempts++
			delay = min(delay*2, maxReconnectDelay)
			continue
		}
		select {
		case <-stop:
			// A deliberate disconnect raced the successful attempt.
			c.disconnect()
			return false
		default:
			return true
		}
	}
	return false
}

// attemptFailover rotates through the current host and the backup hosts, one
// attempt per host, until a connection is established or recovery is stopped.
func (c *connection) attemptFailover(stop chan struct{}, currentHost string, failoverHosts []string) {
	cycle := append([]string{currentHost}, failoverHosts...)
	// First retry the current host once.
	if c.attemptReconnect(stop, c.reconnectDelay, 1) {
		return
	}
	index := 0
	for {
		select {
		case <-stop:
			return
		default:
		}
		index = (index + 1) % len(cycle)
		c.logger.Debug("failing over", "from", c.currentHost(), "to", cycle[index])
		c.setHost(cycle[index])
		if c.attemptReconnect(stop, c.reconnectDelay, 1) {
			return
		}
	}
}
