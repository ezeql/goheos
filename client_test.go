package goheos_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ezeql/goheos"
	"github.com/ezeql/goheos/internal/heostest"
)

func TestConnectAndDisconnect(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	c := connectClient(t, d)

	if got := c.ConnectionState(); got != goheos.StateConnected {
		t.Errorf("state = %v", got)
	}
	// The connect flow checks the account and registers for change events.
	if !c.IsSignedIn() || c.SignedInUsername() != "example@example.com" {
		t.Errorf("signed in = %v (%q)", c.IsSignedIn(), c.SignedInUsername())
	}
	d.AssertCommandCalled(t, "system/check_account", nil)
	if calls := d.RegisterForChangeEventsCalls(); len(calls) != 1 || calls[0] != "on" {
		t.Errorf("register_for_change_events calls = %v, want [on]", calls)
	}

	if err := c.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if got := c.ConnectionState(); got != goheos.StateDisconnected {
		t.Errorf("state after disconnect = %v", got)
	}
}

func TestConnectEmitsConnectedEvent(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	c := goheos.New(d.Address(),
		goheos.WithTimeout(2*time.Second), goheos.WithoutHeartbeat(), goheos.WithEventDebounce(0))
	events, cancel := c.Subscribe()
	t.Cleanup(cancel)
	if err := c.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Disconnect() })
	waitFor[goheos.ConnectedEvent](t, events)
}

func TestDisconnectClosesSubscription(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	c := connectClient(t, d)
	events, cancel := c.Subscribe()
	t.Cleanup(cancel)
	_ = c.Disconnect()
	ev := waitFor[goheos.DisconnectedEvent](t, events)
	if ev.FromError {
		t.Error("deliberate disconnect must report FromError=false")
	}
	// The channel must close after the disconnect event drains.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-events:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("channel was not closed after Disconnect")
		}
	}
}

func TestConnectWithoutEvents(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	connectClient(t, d, goheos.WithoutEvents())
	if calls := d.RegisterForChangeEventsCalls(); len(calls) != 1 || calls[0] != "off" {
		t.Errorf("register_for_change_events calls = %v, want [off]", calls)
	}
}

func TestConnectSignsInWithCredentials(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register("system/sign_in", nil, "system.sign_in")
	c := connectClient(t, d, goheos.WithCredentials("example@example.com", "password"))
	if !c.IsSignedIn() {
		t.Error("client should be signed in after connect")
	}
	d.AssertCommandCalled(t, "system/sign_in", nil)
}

func TestConnectWithBadCredentialsEmitsInvalid(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register("system/sign_in", nil, "system.sign_in_failure")
	c := goheos.New(d.Address(),
		goheos.WithTimeout(2*time.Second), goheos.WithoutHeartbeat(), goheos.WithEventDebounce(0),
		goheos.WithCredentials("bad", "credentials"))
	events, cancel := c.Subscribe()
	t.Cleanup(cancel)
	if err := c.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Disconnect() })
	waitFor[goheos.UserCredentialsInvalidEvent](t, events)
	if c.CurrentCredentials() != nil {
		t.Error("invalid credentials must be cleared")
	}
}

func TestSignInAndOut(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register("system/sign_in", nil, "system.sign_in")
	d.Register("system/sign_out", nil, "system.sign_out")
	c := connectClient(t, d)

	if err := c.SignIn(context.Background(), "example@example.com", "password"); err != nil {
		t.Fatal(err)
	}
	if creds := c.CurrentCredentials(); creds == nil || creds.Username != "example@example.com" {
		t.Errorf("credentials not stored: %+v", creds)
	}
	if err := c.SignOut(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.IsSignedIn() || c.CurrentCredentials() != nil {
		t.Error("sign out must clear the session and credentials")
	}
}

func TestSignInFailure(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register("system/sign_in", nil, "system.sign_in_failure")
	c := connectClient(t, d)

	err := c.SignIn(context.Background(), "example@example.com", "bad")
	if !errors.Is(err, goheos.ErrAuthenticationFailure) {
		t.Errorf("err = %v, want authentication failure", err)
	}
	var failed *goheos.CommandFailedError
	if !errors.As(err, &failed) || failed.ErrorID != goheos.ErrorUserNotFound {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAuthErrorTriggersAutoSignOut(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register("browse/browse", nil, "browse.browse_fail_user_not_logged_in")
	d.Register("system/sign_out", nil, "system.sign_out")
	c := connectClient(t, d)
	events, cancel := c.Subscribe()
	t.Cleanup(cancel)

	_, err := c.Browse(context.Background(), goheos.MusicSourceFavorites)
	if !errors.Is(err, goheos.ErrAuthenticationFailure) {
		t.Fatalf("err = %v, want authentication failure", err)
	}
	waitFor[goheos.UserCredentialsInvalidEvent](t, events)
	d.AssertCommandCalled(t, "system/sign_out", nil)
}

func TestCheckAccountLoggedOut(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.RegisterReplace("system/check_account", nil, "system.check_account_logged_out")
	c := connectClient(t, d)
	username, signedIn, err := c.CheckAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if signedIn || username != "" {
		t.Errorf("expected signed out, got %q", username)
	}
	if c.IsSignedIn() {
		t.Error("client must not report signed in")
	}
}

func TestGetSystemInfo(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register("player/get_players", nil, "player.get_players")
	c := connectClient(t, d)
	info, err := c.GetSystemInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsSignedIn || info.SignedInUsername != "example@example.com" {
		t.Errorf("system sign-in state wrong: %+v", info)
	}
	if len(info.Hosts) != 2 {
		t.Fatalf("hosts = %d", len(info.Hosts))
	}
	if len(info.PreferredHosts) != 1 || info.PreferredHosts[0].IPAddress != "127.0.0.1" {
		t.Errorf("preferred hosts = %+v", info.PreferredHosts)
	}
	assertGolden(t, "system_info", info)
}

func TestValidateConnection(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register("player/get_players", nil, "player.get_players")
	info, err := goheos.ValidateConnection(context.Background(), d.Address())
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Hosts) != 2 {
		t.Errorf("hosts = %d", len(info.Hosts))
	}
	if calls := d.RegisterForChangeEventsCalls(); len(calls) != 1 || calls[0] != "off" {
		t.Errorf("register_for_change_events calls = %v, want [off]", calls)
	}
}

func TestRebootReturnsImmediately(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	c := connectClient(t, d)
	start := time.Now()
	if err := c.Reboot(context.Background()); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("reboot must not wait for a response, took %v", elapsed)
	}
}

func TestCommandInvalidArguments(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	c := connectClient(t, d)
	ctx := context.Background()
	checks := []error{
		c.SetVolume(ctx, 1, 101),
		c.SetVolume(ctx, 1, -1),
		c.VolumeUp(ctx, 1, 11),
		c.VolumeDown(ctx, 1, 0),
		c.SetGroupVolume(ctx, 1, 101),
		c.GroupVolumeUp(ctx, 1, 11),
		c.SetQuickSelect(ctx, 1, 7),
		c.PlayQuickSelect(ctx, 1, 0),
		c.SetPlayState(ctx, 1, goheos.PlayStateUnknown),
		c.PlayPresetStation(ctx, 1, 0),
		c.SaveQueue(ctx, 1, string(make([]byte, 129))),
	}
	for i, err := range checks {
		if !errors.Is(err, goheos.ErrInvalidArgument) {
			t.Errorf("check %d: err = %v, want ErrInvalidArgument", i, err)
		}
	}
}

// An impatient caller must fail alone: neither waiting behind a slow command
// nor arriving with its deadline already gone may tear down the session that
// every other caller shares.
func TestImpatientCallerKeepsTheConnection(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register("player/get_volume", map[string]string{"pid": "1"}, "player.get_volume").WithDelay(800 * time.Millisecond)
	d.Register("player/get_volume", map[string]string{"pid": "2"}, "player.get_volume")
	d.Register("system/heart_beat", nil, "system.heart_beat")
	c := connectClient(t, d)
	events, cancel := c.Subscribe()
	t.Cleanup(cancel)

	slow := make(chan error, 1)
	go func() {
		_, err := c.GetVolume(context.Background(), 1)
		slow <- err
	}()
	time.Sleep(50 * time.Millisecond) // let the slow command take the line

	ctx, cancelWait := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancelWait()
	start := time.Now()
	if _, err := c.GetVolume(ctx, 2); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued caller err = %v, want deadline exceeded", err)
	}
	if waited := time.Since(start); waited > 500*time.Millisecond {
		t.Errorf("queued caller waited %v past its 100ms deadline", waited)
	}

	expired, cancelExpired := context.WithCancel(context.Background())
	cancelExpired()
	if err := c.SendHeartBeat(expired); !errors.Is(err, context.Canceled) {
		t.Fatalf("expired caller err = %v, want canceled", err)
	}

	if err := <-slow; err != nil {
		t.Fatalf("slow command failed: %v", err)
	}
	if v, err := c.GetVolume(context.Background(), 2); err != nil || v != 36 {
		t.Fatalf("follow-up GetVolume = %d, %v", v, err)
	}
	if got := c.ConnectionState(); got != goheos.StateConnected {
		t.Errorf("state = %v, want connected", got)
	}
	select {
	case e := <-events:
		t.Errorf("unexpected event %#v", e)
	default:
	}
}

// A response that arrives after its caller timed out must not answer the
// next command with the same name for another player.
func TestLateResponseIsNotHandedToTheNextCaller(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register("player/get_volume", map[string]string{"pid": "1"}, "player.get_volume_10").WithDelay(300 * time.Millisecond)
	d.Register("player/get_volume", map[string]string{"pid": "2"}, "player.get_volume")
	c := connectClient(t, d)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := c.GetVolume(ctx, 1); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
	v, err := c.GetVolume(context.Background(), 2)
	if err != nil || v != 36 {
		t.Fatalf("player 2 volume = %d, %v; want 36 (10 is player 1's late reply)", v, err)
	}
}

// Subscriptions close once nothing will reconnect: when reconnect runs out
// of attempts, and when failover has no other host to try.
func TestSubscriptionsCloseWhenRecoveryEnds(t *testing.T) {
	t.Parallel()
	for name, opt := range map[string]goheos.Option{
		"reconnect gives up":     goheos.WithAutoReconnect(10*time.Millisecond, 2),
		"failover with no hosts": goheos.WithAutoFailover(),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := heostest.StartDevice(t)
			c := connectClient(t, d, opt)
			events, cancel := c.Subscribe()
			t.Cleanup(cancel)

			d.Stop()
			deadline := time.After(5 * time.Second)
			for {
				select {
				case _, ok := <-events:
					if !ok {
						return
					}
				case <-deadline:
					t.Fatalf("subscription still open, state %v", c.ConnectionState())
				}
			}
		})
	}
}
