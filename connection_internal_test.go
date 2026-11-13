package goheos

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ezeql/goheos/internal/heostest"
)

func testOptions() *options {
	o := defaultOptions()
	o.timeout = 500 * time.Millisecond
	o.heartBeat = false
	o.eventDebounce = 0
	return &o
}

func connectTestConn(t *testing.T, d *heostest.Device, o *options) *connection {
	t.Helper()
	c := newConnection(d.Address(), o)
	if err := c.connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(c.disconnect)
	return c
}

func TestConnectionCommandSuccess(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register("player/get_volume", map[string]string{"pid": "1"}, "player.get_volume")
	c := connectTestConn(t, d, testOptions())

	resp, err := c.command(context.Background(),
		newCommand("player/get_volume").withIntParam(attrPlayerID, 1))
	if err != nil {
		t.Fatal(err)
	}
	if level, err := resp.intValue(attrLevel); err != nil || level != 36 {
		t.Errorf("level = %d, err = %v", level, err)
	}
	d.AssertCommandCalled(t, "player/get_volume", map[string]string{"pid": "1"})
}

func TestConnectionCommandNotConnected(t *testing.T) {
	t.Parallel()
	c := newConnection("127.0.0.1:1", testOptions())
	_, err := c.command(context.Background(), newCommand(cmdGetPlayers))
	if !errors.Is(err, ErrNotConnected) {
		t.Errorf("err = %v, want ErrNotConnected", err)
	}
	var ce *CommandError
	if !errors.As(err, &ce) || ce.Command != cmdGetPlayers {
		t.Errorf("expected CommandError for %s, got %v", cmdGetPlayers, err)
	}
}

func TestConnectionConnectFailure(t *testing.T) {
	t.Parallel()
	c := newConnection("127.0.0.1:1", testOptions())
	if err := c.connect(context.Background()); err == nil {
		t.Error("expected connection failure")
	}
	if c.currentState() != StateDisconnected {
		t.Errorf("state = %v", c.currentState())
	}
}

func TestConnectionCommandFailure(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register(cmdSignIn, nil, "system.sign_in_failure")
	c := connectTestConn(t, d, testOptions())

	var reported *CommandFailedError
	c.onCommandError = func(e *CommandFailedError) { reported = e }
	_, err := c.command(context.Background(),
		newCommand(cmdSignIn).withParam(attrUserName, "user").withParam(attrPassword, "pass"))
	var failed *CommandFailedError
	if !errors.As(err, &failed) {
		t.Fatalf("err = %v, want CommandFailedError", err)
	}
	if failed.ErrorID != ErrorUserNotFound {
		t.Errorf("ErrorID = %d", failed.ErrorID)
	}
	if reported != failed {
		t.Error("onCommandError was not invoked with the failure")
	}
}

func TestConnectionCommandTimeout(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register(cmdGetPlayers, nil, "player.get_players").WithDelay(2 * time.Second)
	c := connectTestConn(t, d, testOptions())

	start := time.Now()
	_, err := c.command(context.Background(), newCommand(cmdGetPlayers))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 1500*time.Millisecond {
		t.Errorf("timeout took too long: %v", elapsed)
	}
}

func TestConnectionRebootDoesNotWait(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	c := connectTestConn(t, d, testOptions())

	start := time.Now()
	resp, err := c.command(context.Background(), newCommand(cmdReboot))
	if err != nil {
		t.Fatal(err)
	}
	if !resp.result {
		t.Error("synthetic reboot response must be success")
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Errorf("reboot must not wait for a response, took %v", elapsed)
	}
}

func TestConnectionUnderProcessIgnored(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register(cmdClearQueue, nil, "other.command_under_process", "player.clear_queue")
	c := connectTestConn(t, d, testOptions())

	resp, err := c.command(context.Background(),
		newCommand(cmdClearQueue).withIntParam(attrPlayerID, 1))
	if err != nil {
		t.Fatal(err)
	}
	if resp.isUnderProcess() {
		t.Error("under-process response leaked as final response")
	}
}

func TestConnectionReplayedResponseIgnored(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register(cmdGetPlayers, nil, "player.get_players").WithReplay(2)
	d.Register("player/get_volume", nil, "player.get_volume")
	c := connectTestConn(t, d, testOptions())

	if _, err := c.command(context.Background(), newCommand(cmdGetPlayers)); err != nil {
		t.Fatal(err)
	}
	// The duplicated response must not satisfy the next command.
	resp, err := c.command(context.Background(),
		newCommand("player/get_volume").withIntParam(attrPlayerID, 1))
	if err != nil {
		t.Fatal(err)
	}
	if resp.command != "player/get_volume" {
		t.Errorf("response command = %q", resp.command)
	}
}

func TestConnectionEventDelivered(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	o := testOptions()
	c := newConnection(d.Address(), o)
	events := make(chan *message, 10)
	c.onEvent = func(m *message) { events <- m }
	if err := c.connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.disconnect)

	if _, err := c.command(context.Background(),
		newCommand(cmdRegisterForChangeEvents).withParam(attrEnable, valueOn)); err != nil {
		t.Fatal(err)
	}
	d.SendEvent(t, "event.player_volume_changed", map[string]string{
		"player_id": "1", "level": "10", "mute": "off",
	})
	select {
	case m := <-events:
		if m.command != string(EventPlayerVolumeChanged) {
			t.Errorf("event = %q", m.command)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("event not delivered")
	}
}

func TestConnectionDebounceCollapsesBursts(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	o := testOptions()
	o.eventDebounce = 100 * time.Millisecond
	c := newConnection(d.Address(), o)
	var mu sync.Mutex
	var got []string
	c.onEvent = func(m *message) {
		mu.Lock()
		got = append(got, m.command)
		mu.Unlock()
	}
	if err := c.connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.disconnect)
	if _, err := c.command(context.Background(),
		newCommand(cmdRegisterForChangeEvents).withParam(attrEnable, valueOn)); err != nil {
		t.Fatal(err)
	}

	for range 5 {
		d.SendEvent(t, "event.players_changed", nil)
	}
	time.Sleep(400 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Errorf("debounced burst delivered %d times: %v", len(got), got)
	}
}

func TestConnectionSourcesChangedSuppressedDuringRefresh(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register(cmdBrowseGetSources, nil, "browse.get_music_sources").
		WithPreEvents("event.sources_changed").
		WithDelay(50 * time.Millisecond)
	o := testOptions()
	c := newConnection(d.Address(), o)
	events := make(chan *message, 10)
	c.onEvent = func(m *message) { events <- m }
	if err := c.connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.disconnect)
	if _, err := c.command(context.Background(),
		newCommand(cmdRegisterForChangeEvents).withParam(attrEnable, valueOn)); err != nil {
		t.Fatal(err)
	}

	if _, err := c.command(context.Background(),
		newCommand(cmdBrowseGetSources).withParam(attrRefresh, valueOn)); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-events:
		t.Errorf("sources_changed should be suppressed during refresh, got %q", m.command)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestConnectionHeartbeatAfterInactivity(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	heartBeat := d.Register(cmdHeartBeat, nil, "system.heart_beat")
	o := testOptions()
	o.heartBeat = true
	o.heartBeatInterval = 100 * time.Millisecond
	connectTestConn(t, d, o)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if heartBeat.CallCount() > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("heartbeat was never sent")
}

func TestConnectionDroppedTriggersDisconnectedCallback(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	o := testOptions()
	c := newConnection(d.Address(), o)
	disconnected := make(chan bool, 1)
	c.onDisconnected = func(fromError, _ bool) { disconnected <- fromError }
	if err := c.connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.disconnect)

	d.Stop()
	select {
	case fromError := <-disconnected:
		if !fromError {
			t.Error("drop must report fromError=true")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("onDisconnected not invoked")
	}
	if c.currentState() != StateDisconnected {
		t.Errorf("state = %v", c.currentState())
	}
}

func TestConnectionDisconnectFailsInFlightCommand(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register(cmdGetPlayers, nil, "player.get_players").WithDelay(5 * time.Second)
	o := testOptions()
	o.timeout = 10 * time.Second
	c := connectTestConn(t, d, o)

	errs := make(chan error, 1)
	go func() {
		_, err := c.command(context.Background(), newCommand(cmdGetPlayers))
		errs <- err
	}()
	time.Sleep(100 * time.Millisecond)
	c.disconnect()
	select {
	case err := <-errs:
		if !errors.Is(err, ErrDisconnected) {
			t.Errorf("err = %v, want ErrDisconnected", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight command did not fail on disconnect")
	}
}

func TestConnectionDisconnectIdempotent(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	o := testOptions()
	c := newConnection(d.Address(), o)
	calls := make(chan bool, 4)
	c.onDisconnected = func(fromError, _ bool) { calls <- fromError }
	if err := c.connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	c.disconnect()
	c.disconnect()
	if n := len(calls); n != 1 {
		t.Errorf("onDisconnected called %d times, want 1", n)
	}
}
