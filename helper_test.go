package goheos_test

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/ezeql/goheos"
	"github.com/ezeql/goheos/internal/heostest"
)

var update = flag.Bool("update", false, "update golden files")

// connectClient connects a test client with fast timeouts, no heartbeat and
// no event debounce, mirroring the pyheos test fixture configuration.
func connectClient(t *testing.T, d *heostest.Device, opts ...goheos.Option) *goheos.Client {
	t.Helper()
	base := []goheos.Option{
		goheos.WithTimeout(2 * time.Second),
		goheos.WithoutHeartbeat(),
		goheos.WithEventDebounce(0),
	}
	c, err := goheos.Connect(context.Background(), d.Address(), append(base, opts...)...)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = c.Disconnect() })
	return c
}

// registerPlayerCommands registers the command bundle issued when players are
// loaded, like pyheos' calls_player_commands decorator.
func registerPlayerCommands(d *heostest.Device, playerIDs ...int) {
	if len(playerIDs) == 0 {
		playerIDs = []int{1, 2}
	}
	d.Register("player/get_players", nil, "player.get_players")
	registerPlayerStateCommands(d, playerIDs...)
}

func registerPlayerStateCommands(d *heostest.Device, playerIDs ...int) {
	for _, id := range playerIDs {
		pid := map[string]string{"pid": strconv.Itoa(id)}
		d.Register("player/get_play_state", pid, "player.get_play_state")
		d.Register("player/get_now_playing_media", pid, "player.get_now_playing_media")
		d.Register("player/get_volume", pid, "player.get_volume")
		d.Register("player/get_mute", pid, "player.get_mute")
		d.Register("player/get_play_mode", pid, "player.get_play_mode")
	}
}

// registerGroupCommands registers the command bundle issued when groups are
// loaded, like pyheos' calls_group_commands decorator.
func registerGroupCommands(d *heostest.Device) {
	d.Register("group/get_groups", nil, "group.get_groups")
	gid := map[string]string{"gid": "1"}
	d.Register("group/get_volume", gid, "group.get_volume")
	d.Register("group/get_mute", gid, "group.get_mute")
}

// waitFor reads events until one of type T arrives.
func waitFor[T goheos.Event](t *testing.T, events <-chan goheos.Event) T {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case e, ok := <-events:
			if !ok {
				t.Fatal("event channel closed while waiting")
			}
			if v, ok := e.(T); ok {
				return v
			}
		case <-deadline:
			var zero T
			t.Fatalf("timed out waiting for %T", zero)
			return zero
		}
	}
}

// assertGolden compares the JSON serialization of v against a golden file.
// Run `go test -update` to (re)generate golden files.
func assertGolden(t *testing.T, name string, v any) {
	t.Helper()
	got, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "golden", name+".json")
	if *update {
		if writeErr := os.WriteFile(path, got, 0o644); writeErr != nil {
			t.Fatal(writeErr)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden file missing (run `go test -update`): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("golden mismatch for %s:\n got: %s\nwant: %s", name, got, want)
	}
}
