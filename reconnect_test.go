package goheos_test

import (
	"context"
	"testing"
	"time"

	"github.com/ezeql/goheos"
	"github.com/ezeql/goheos/internal/heostest"
)

func TestAutoReconnectAfterDeviceRestart(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	registerPlayerCommands(d)
	c := connectClient(t, d, goheos.WithAutoReconnect(100*time.Millisecond, 0))
	if _, err := c.GetPlayers(context.Background()); err != nil {
		t.Fatal(err)
	}
	events, cancel := c.Subscribe()
	t.Cleanup(cancel)

	d.Restart()

	ev := waitFor[goheos.DisconnectedEvent](t, events)
	if !ev.FromError {
		t.Error("restart must disconnect with FromError=true")
	}
	// Players are marked unavailable while disconnected.
	waitFor[goheos.ConnectedEvent](t, events)

	// The reconnection re-registers for change events and refreshes the
	// loaded players.
	update := waitFor[goheos.ControllerEvent](t, events)
	if update.Command != goheos.EventPlayersChanged || update.Update == nil {
		t.Errorf("expected players changed refresh, got %+v", update)
	}
	if c.ConnectionState() != goheos.StateConnected {
		t.Errorf("state = %v", c.ConnectionState())
	}
	// One registration from the initial connect, one after reconnecting.
	if calls := d.RegisterForChangeEventsCalls(); len(calls) != 2 || calls[1] != "on" {
		t.Errorf("register calls after restart = %v", calls)
	}
	if !c.Players()[1].Available() {
		t.Error("players must be available again after reconnect")
	}
}

func TestRebootTriggersReconnect(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	c := connectClient(t, d, goheos.WithAutoReconnect(100*time.Millisecond, 0))
	events, cancel := c.Subscribe()
	t.Cleanup(cancel)

	// Reboot returns immediately; the device drops the connection and comes
	// back, and the client reconnects.
	if err := c.Reboot(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitFor[goheos.DisconnectedEvent](t, events)
	waitFor[goheos.ConnectedEvent](t, events)
}

func TestReconnectGivesUpAfterMaxAttempts(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	c := connectClient(t, d, goheos.WithAutoReconnect(50*time.Millisecond, 2))
	events, cancel := c.Subscribe()
	t.Cleanup(cancel)

	d.Stop()
	waitFor[goheos.DisconnectedEvent](t, events)

	// With the device gone for good, the client stops after 2 attempts.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if c.ConnectionState() == goheos.StateDisconnected {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Errorf("state = %v, want disconnected after giving up", c.ConnectionState())
}

func TestDisconnectAbortsReconnect(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	c := connectClient(t, d, goheos.WithAutoReconnect(100*time.Millisecond, 0))
	events, cancel := c.Subscribe()
	t.Cleanup(cancel)

	d.Stop()
	waitFor[goheos.DisconnectedEvent](t, events)
	// While reconnecting, a deliberate disconnect must stop the attempts and
	// close the subscription.
	if err := c.Disconnect(); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-events:
			if !ok {
				if got := c.ConnectionState(); got != goheos.StateDisconnected {
					t.Errorf("state = %v", got)
				}
				return
			}
		case <-deadline:
			t.Fatal("subscription not closed after Disconnect during reconnect")
		}
	}
}

func TestFailoverToSecondDevice(t *testing.T) {
	t.Parallel()
	primary := heostest.StartDevice(t)
	backup := heostest.StartDevice(t)
	c := connectClient(t, primary,
		goheos.WithAutoReconnect(50*time.Millisecond, 0),
		goheos.WithAutoFailover(backup.Address()))
	events, cancel := c.Subscribe()
	t.Cleanup(cancel)

	primary.Stop()
	waitFor[goheos.DisconnectedEvent](t, events)
	waitFor[goheos.ConnectedEvent](t, events)

	if c.Host() != backup.Address() {
		t.Errorf("host = %q, want backup %q", c.Host(), backup.Address())
	}
	if backup.ConnectionCount() == 0 {
		t.Error("backup device received no connection")
	}
	// Commands work against the backup device.
	backup.Register("player/get_players", nil, "player.get_players")
	if _, err := c.GetSystemInfo(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestFailoverPrefersCurrentHostFirst(t *testing.T) {
	t.Parallel()
	primary := heostest.StartDevice(t)
	backup := heostest.StartDevice(t)
	// The delay is longer than the restart window so the primary is back up
	// before the first attempt, which always targets the current host.
	c := connectClient(t, primary,
		goheos.WithAutoReconnect(500*time.Millisecond, 0),
		goheos.WithAutoFailover(backup.Address()))
	events, cancel := c.Subscribe()
	t.Cleanup(cancel)

	primary.Restart()
	waitFor[goheos.DisconnectedEvent](t, events)
	waitFor[goheos.ConnectedEvent](t, events)
	if c.Host() != primary.Address() {
		t.Errorf("host = %q, want primary %q", c.Host(), primary.Address())
	}
}
