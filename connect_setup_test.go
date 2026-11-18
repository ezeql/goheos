package goheos

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ezeql/goheos/internal/heostest"
)

func TestConnectReturnsSetupFailures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, command string
		options       []Option
		playersLoaded bool
		groupsLoaded  bool
	}{
		{name: "sign in", command: cmdSignIn, options: []Option{WithCredentials("user", "password")}},
		{name: "account check", command: cmdAccountCheck},
		{name: "failover hosts", command: cmdGetPlayers, options: []Option{WithAutoFailover()}},
		{name: "event registration", command: cmdRegisterForChangeEvents},
		{name: "player refresh", command: cmdGetPlayers, playersLoaded: true},
		{name: "group refresh", command: cmdGetGroups, groupsLoaded: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := heostest.StartDevice(t)
			d.RegisterReplace(tc.command, nil, "other.unknown_command")
			opts := append([]Option{WithTimeout(time.Second), WithoutHeartbeat()}, tc.options...)
			c := New(d.Address(), opts...)
			c.playersLoaded, c.groupsLoaded = tc.playersLoaded, tc.groupsLoaded
			t.Cleanup(func() { _ = c.Disconnect() })
			events, cancel := c.Subscribe()
			t.Cleanup(cancel)

			err := c.Connect(context.Background())
			var failed *CommandFailedError
			if !errors.As(err, &failed) || failed.Command != tc.command {
				t.Errorf("Connect error = %v, want command failure for %s", err, tc.command)
			}
			if state := c.ConnectionState(); state != StateDisconnected {
				t.Errorf("state = %v, want disconnected after setup failure", state)
			}
			c.closeSubscribers()
			for event := range events {
				if _, ok := event.(ConnectedEvent); ok {
					t.Error("setup failure emitted ConnectedEvent")
				}
			}
		})
	}
}

func TestConnectSetupHonorsContext(t *testing.T) {
	t.Parallel()
	for _, cancelEarly := range []bool{false, true} {
		name := "deadline"
		if cancelEarly {
			name = "cancellation"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := heostest.StartDevice(t)
			registration := d.Register(cmdRegisterForChangeEvents, nil, "system.register_for_change_events").
				WithDelay(400 * time.Millisecond)
			c := New(d.Address(), WithTimeout(time.Second), WithoutHeartbeat())
			t.Cleanup(func() { _ = c.Disconnect() })
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			want := context.DeadlineExceeded
			if cancelEarly {
				want = context.Canceled
				go func() {
					for registration.CallCount() == 0 && ctx.Err() == nil {
						time.Sleep(time.Millisecond)
					}
					cancel()
				}()
			}
			if err := c.Connect(ctx); !errors.Is(err, want) {
				t.Errorf("Connect error = %v, want %v", err, want)
			}
			if registration.CallCount() != 1 {
				t.Error("test must reach post-connect event registration")
			}
			if state := c.ConnectionState(); state != StateDisconnected {
				t.Errorf("state = %v, want disconnected after canceled setup", state)
			}
		})
	}
}

func TestReconnectRetriesSetupFailure(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	o := testOptions()
	o.autoReconnect, o.autoReconnectDelay, o.autoReconnectMaxAttempts = true, time.Millisecond, 2
	c := newConnection(d.Address(), o)
	var attempts atomic.Int32
	c.onConnected = func(context.Context) error {
		if attempts.Add(1) == 2 {
			return errors.New("setup failed")
		}
		return nil
	}
	if err := c.connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.disconnect)
	c.disconnectFromError(errors.New("connection lost"))
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if attempts.Load() == 3 && c.currentState() == StateConnected {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("attempts = %d, state = %v; want successful retry after setup failure", attempts.Load(), c.currentState())
}
