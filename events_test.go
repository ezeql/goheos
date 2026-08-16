package goheos_test

import (
	"context"
	"testing"
	"time"

	"github.com/ezeql/goheos"
	"github.com/ezeql/goheos/internal/heostest"
)

func TestPlayerVolumeChangedEvent(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	registerPlayerCommands(d)
	c := connectClient(t, d)
	players, err := c.GetPlayers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	events, cancel := c.Subscribe()
	t.Cleanup(cancel)

	d.SendEvent(t, "event.player_volume_changed", map[string]string{
		"player_id": "1", "level": "50", "mute": "on",
	})
	ev := waitFor[goheos.PlayerEvent](t, events)
	if ev.PlayerID != 1 || ev.Command != goheos.EventPlayerVolumeChanged {
		t.Errorf("event = %+v", ev)
	}
	// The model is updated before the event is delivered.
	if players[1].Volume() != 50 || !players[1].IsMuted() {
		t.Errorf("player state = %d/%v", players[1].Volume(), players[1].IsMuted())
	}
}

func TestPlayerStateChangedEvent(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	registerPlayerCommands(d)
	c := connectClient(t, d)
	players, err := c.GetPlayers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	events, cancel := c.Subscribe(goheos.WithPlayerEvents(1))
	t.Cleanup(cancel)

	d.SendEvent(t, "event.player_state_changed", map[string]string{
		"player_id": "1", "state": "play",
	})
	ev := waitFor[goheos.PlayerEvent](t, events)
	if ev.Command != goheos.EventPlayerStateChanged {
		t.Errorf("event = %+v", ev)
	}
	if players[1].State() != goheos.PlayStatePlay {
		t.Errorf("state = %v", players[1].State())
	}
}

func TestPlayerNowPlayingChangedIssuesFollowUp(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	registerPlayerCommands(d)
	c := connectClient(t, d)
	players, err := c.GetPlayers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	events, cancel := c.Subscribe()
	t.Cleanup(cancel)

	// The event carries no data: the client must issue a follow-up
	// get_now_playing_media command.
	d.RegisterReplace("player/get_now_playing_media",
		map[string]string{"pid": "1"}, "player.get_now_playing_media_changed")
	d.SendEvent(t, "event.player_now_playing_changed", map[string]string{"player_id": "1"})

	ev := waitFor[goheos.PlayerEvent](t, events)
	if ev.Command != goheos.EventPlayerNowPlayingChanged {
		t.Errorf("event = %+v", ev)
	}
	np := players[1].NowPlaying()
	if np.Song == "" {
		t.Error("now playing was not refreshed from the follow-up command")
	}
	d.AssertCommandCalled(t, "player/get_now_playing_media", map[string]string{"pid": "1"})
}

func TestProgressEventFiltering(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	registerPlayerCommands(d)
	c := connectClient(t, d, goheos.WithChangedProgressEventsOnly())
	players, err := c.GetPlayers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	events, cancel := c.Subscribe(goheos.WithPlayerEvents())
	t.Cleanup(cancel)

	send := func() {
		d.SendEvent(t, "event.player_now_playing_progress", map[string]string{
			"player_id": "1", "cur_pos": "5000", "duration": "180000",
		})
	}
	send()
	ev := waitFor[goheos.PlayerEvent](t, events)
	if ev.Command != goheos.EventPlayerNowPlayingProgress {
		t.Errorf("event = %+v", ev)
	}
	np := players[1].NowPlaying()
	if np.CurrentPosition != 5000 || np.Duration != 180000 {
		t.Errorf("progress = %d/%d", np.CurrentPosition, np.Duration)
	}

	// A second progress event is filtered while progress is already known.
	send()
	select {
	case e := <-events:
		t.Errorf("filtered progress event leaked: %+v", e)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestGroupVolumeChangedEvent(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	registerGroupCommands(d)
	c := connectClient(t, d)
	groups, err := c.GetGroups(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	events, cancel := c.Subscribe(goheos.WithGroupEvents(1))
	t.Cleanup(cancel)

	// The fixture reports gid=1, level=50, mute=on.
	d.SendEvent(t, "event.group_volume_changed", nil)
	ev := waitFor[goheos.GroupEvent](t, events)
	if ev.GroupID != 1 || ev.Command != goheos.EventGroupVolumeChanged {
		t.Errorf("event = %+v", ev)
	}
	if groups[1].Volume() != 50 || !groups[1].IsMuted() {
		t.Errorf("group state = %d/%v", groups[1].Volume(), groups[1].IsMuted())
	}
}

func TestPlayersChangedReloadsPlayers(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	registerPlayerCommands(d)
	c := connectClient(t, d)
	if _, err := c.GetPlayers(context.Background()); err != nil {
		t.Fatal(err)
	}
	events, cancel := c.Subscribe()
	t.Cleanup(cancel)

	// After the event, the new topology has a third player.
	d.RegisterReplace("player/get_players", nil, "player.get_players_changed")
	registerPlayerStateCommands(d, 3)
	d.SendEvent(t, "event.players_changed", nil)

	ev := waitFor[goheos.ControllerEvent](t, events)
	if ev.Command != goheos.EventPlayersChanged {
		t.Errorf("event = %+v", ev)
	}
	if ev.Update == nil {
		t.Fatal("players_changed must carry a PlayerUpdateResult")
	}
	if len(ev.Update.AddedPlayerIDs) != 1 || ev.Update.AddedPlayerIDs[0] != 3 {
		t.Errorf("added = %v", ev.Update.AddedPlayerIDs)
	}
}

func TestGroupsChangedRefreshesGroupsAndPlayers(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	registerPlayerCommands(d)
	registerGroupCommands(d)
	c := connectClient(t, d)
	ctx := context.Background()
	if _, err := c.GetPlayers(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetGroups(ctx); err != nil {
		t.Fatal(err)
	}
	events, cancel := c.Subscribe()
	t.Cleanup(cancel)

	// The players lose their groups; group ids on players are refreshed too.
	d.RegisterReplace("group/get_groups", nil, "group.get_groups_changed")
	d.RegisterReplace("player/get_players", nil, "player.get_players_no_groups")
	d.SendEvent(t, "event.groups_changed", nil)

	ev := waitFor[goheos.ControllerEvent](t, events)
	if ev.Command != goheos.EventGroupsChanged {
		t.Errorf("event = %+v", ev)
	}
	if _, ok := c.Players()[1].GroupID(); ok {
		t.Error("player group id should be cleared after groups changed")
	}
}

func TestUserChangedSignOut(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register("browse/get_music_sources", nil, "browse.get_music_sources")
	c := connectClient(t, d)
	if _, err := c.GetMusicSources(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !c.IsSignedIn() {
		t.Fatal("precondition: signed in after connect")
	}
	events, cancel := c.Subscribe()
	t.Cleanup(cancel)

	d.SendEvent(t, "event.user_changed_signed_out", nil)
	ev := waitFor[goheos.ControllerEvent](t, events)
	if ev.Command != goheos.EventUserChanged {
		t.Errorf("event = %+v", ev)
	}
	if c.IsSignedIn() {
		t.Error("client must be signed out after user_changed signed_out")
	}
}

func TestUserChangedSignIn(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	// Connect signed out, then the sign-in event flips the state.
	d.RegisterReplace("system/check_account", nil, "system.check_account_logged_out")
	c := connectClient(t, d)
	if c.IsSignedIn() {
		t.Fatal("precondition: signed out after connect")
	}
	events, cancel := c.Subscribe()
	t.Cleanup(cancel)

	d.SendEvent(t, "event.user_changed_signed_in", nil)
	waitFor[goheos.ControllerEvent](t, events)
	if c.SignedInUsername() != "example@example.com" {
		t.Errorf("username = %q", c.SignedInUsername())
	}
}

func TestSubscribeCancelClosesChannel(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	c := connectClient(t, d)
	events, cancel := c.Subscribe()
	cancel()
	select {
	case _, ok := <-events:
		if ok {
			t.Error("expected closed channel after cancel")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("channel not closed after cancel")
	}
	// Cancel must be idempotent.
	cancel()
}

func TestSubscribeBoundedBufferDropsNewest(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	registerPlayerCommands(d)
	c := connectClient(t, d)
	if _, err := c.GetPlayers(context.Background()); err != nil {
		t.Fatal(err)
	}
	// A bounded queue with no reader: extra events are dropped, the client
	// never blocks.
	events, cancel := c.Subscribe(goheos.WithBoundedBuffer(1, false))
	t.Cleanup(cancel)
	for range 10 {
		d.SendEvent(t, "event.player_volume_changed", map[string]string{
			"player_id": "1", "level": "10", "mute": "off",
		})
	}
	// Just verify the client still processes commands (not deadlocked).
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	select {
	case <-events:
	case <-deadline.C:
		t.Fatal("no event delivered")
	}
	if _, err := c.GetPlayers(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestEventOrderPreserved(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	registerPlayerCommands(d)
	c := connectClient(t, d)
	if _, err := c.GetPlayers(context.Background()); err != nil {
		t.Fatal(err)
	}
	events, cancel := c.Subscribe(goheos.WithPlayerEvents(1))
	t.Cleanup(cancel)

	// Levels increase monotonically; the state is updated before each event
	// is queued, so the observed volume can never decrease.
	for i := 1; i <= 20; i++ {
		d.SendEvent(t, "event.player_volume_changed", map[string]string{
			"player_id": "1", "level": itoa(i), "mute": "off",
		})
	}
	last := -1
	for range 20 {
		waitFor[goheos.PlayerEvent](t, events)
		vol := c.Players()[1].Volume()
		if vol < last {
			t.Fatalf("out of order: volume %d after %d", vol, last)
		}
		last = vol
	}
}

func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return string(rune('0'+i/10)) + string(rune('0'+i%10))
}
