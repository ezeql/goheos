package heostest

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"
)

func dial(t *testing.T, d *Device) (net.Conn, *bufio.Reader) {
	t.Helper()
	conn, err := net.Dial("tcp", d.Address())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn, bufio.NewReader(conn)
}

func send(t *testing.T, conn net.Conn, line string) {
	t.Helper()
	if _, err := conn.Write([]byte(line + separator)); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// readFrame reads one protocol frame. Fixtures may be pretty-printed
// multi-line JSON, so frames are delimited by the CRLF sequence, never by a
// bare newline.
func readFrame(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	var b strings.Builder
	for {
		chunk, err := r.ReadString('\n')
		b.WriteString(chunk)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if before, ok := strings.CutSuffix(b.String(), separator); ok {
			return before
		}
	}
}

func TestDeviceMatchesRegisteredCommand(t *testing.T) {
	t.Parallel()
	d := StartDevice(t)
	m := d.Register("player/get_volume", map[string]string{"pid": "1"}, "player.get_volume")
	conn, r := dial(t, d)
	send(t, conn, "heos://player/get_volume?pid=1")
	resp := readFrame(t, r)
	if !strings.Contains(resp, `"command": "player/get_volume"`) {
		t.Errorf("unexpected response: %s", resp)
	}
	if !strings.Contains(resp, "pid=1") {
		t.Errorf("token {player_id} not substituted: %s", resp)
	}
	m.AssertCalled(t)
	d.AssertCommandCalled(t, "player/get_volume", map[string]string{"pid": "1"})
}

func TestDeviceUnknownCommand(t *testing.T) {
	t.Parallel()
	d := StartDevice(t)
	conn, r := dial(t, d)
	send(t, conn, "heos://player/bogus?pid=1")
	resp := readFrame(t, r)
	if !strings.Contains(resp, "player/bogus") {
		t.Errorf("unknown command response should echo the command: %s", resp)
	}
}

func TestDeviceRegisterForEventsAndSendEvent(t *testing.T) {
	t.Parallel()
	d := StartDevice(t)
	conn, r := dial(t, d)
	send(t, conn, "heos://system/register_for_change_events?enable=on")
	resp := readFrame(t, r)
	if !strings.Contains(resp, "enable=on") {
		t.Errorf("register response should echo enable: %s", resp)
	}
	frames := make(chan string, 1)
	go func() {
		var b strings.Builder
		for {
			chunk, err := r.ReadString('\n')
			b.WriteString(chunk)
			if err != nil {
				return
			}
			if before, ok := strings.CutSuffix(b.String(), separator); ok {
				frames <- before
				return
			}
		}
	}()
	d.SendEvent(t, "event.player_volume_changed", map[string]string{
		"player_id": "1", "level": "10", "mute": "off",
	})
	var event string
	select {
	case event = <-frames:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for event frame")
	}
	if !strings.Contains(event, "event/player_volume_changed") {
		t.Errorf("unexpected event: %s", event)
	}
	if !strings.Contains(event, "pid=1") {
		t.Errorf("event tokens not substituted: %s", event)
	}
}

func TestDevicePreEventsAndReplay(t *testing.T) {
	t.Parallel()
	d := StartDevice(t)
	d.Register("player/get_players", nil, "player.get_players").WithReplay(2)
	conn, r := dial(t, d)
	send(t, conn, "heos://player/get_players")
	first := readFrame(t, r)
	second := readFrame(t, r)
	if first != second {
		t.Error("replayed responses must be identical")
	}
}

func TestDeviceDelay(t *testing.T) {
	t.Parallel()
	d := StartDevice(t)
	d.Register("player/get_players", nil, "player.get_players").
		WithDelay(150 * time.Millisecond)
	conn, r := dial(t, d)
	start := time.Now()
	send(t, conn, "heos://player/get_players")
	_ = readFrame(t, r)
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Errorf("response arrived too early: %v", elapsed)
	}
}

func TestDeviceRestartKeepsAddressAndMatchers(t *testing.T) {
	t.Parallel()
	d := StartDevice(t)
	d.Register("player/get_players", nil, "player.get_players")
	addr := d.Address()
	conn, r := dial(t, d)
	send(t, conn, "heos://system/reboot")
	// The reboot drops the connection without a response.
	if _, err := r.ReadString('\n'); err == nil {
		t.Error("expected connection drop after reboot")
	}
	_ = conn.Close()

	// Wait for the device to come back on the same address.
	deadline := time.Now().Add(5 * time.Second)
	var newConn net.Conn
	for {
		var err error
		newConn, err = net.Dial("tcp", addr)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("device did not come back: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Cleanup(func() { _ = newConn.Close() })
	send(t, newConn, "heos://player/get_players")
	resp := readFrame(t, bufio.NewReader(newConn))
	if !strings.Contains(resp, "player/get_players") {
		t.Errorf("matcher lost after restart: %s", resp)
	}
}
