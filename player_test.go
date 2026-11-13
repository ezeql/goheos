package goheos_test

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/ezeql/goheos"
	"github.com/ezeql/goheos/internal/heostest"
)

func TestGetPlayersLoadsAndCaches(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	registerPlayerCommands(d)
	c := connectClient(t, d)

	players, err := c.GetPlayers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(players) != 2 {
		t.Fatalf("players = %d, want 2", len(players))
	}
	p := players[1]
	if p.Name() != "Back Patio" || p.Model() != "HEOS Drive" || p.Serial() != "B1A2C3K" {
		t.Errorf("player 1 base info wrong: %s %s %s", p.Name(), p.Model(), p.Serial())
	}
	if p.Network() != goheos.NetworkWired || p.LineOut() != goheos.LineOutLevelFixed {
		t.Errorf("player 1 network/lineout wrong: %v %v", p.Network(), p.LineOut())
	}
	if !p.SupportedVersion() || p.Version() != "3.34.240" {
		t.Errorf("player 1 version wrong: %s", p.Version())
	}
	if gid, ok := p.GroupID(); !ok || gid != 2 {
		t.Errorf("player 1 group = %d/%v", gid, ok)
	}
	if p.State() != goheos.PlayStateStop || p.Volume() != 36 || p.IsMuted() {
		t.Errorf("player 1 state wrong: %v %d %v", p.State(), p.Volume(), p.IsMuted())
	}
	if p.Repeat() != goheos.RepeatOff || p.Shuffle() {
		t.Errorf("player 1 play mode wrong: %v %v", p.Repeat(), p.Shuffle())
	}
	np := p.NowPlaying()
	if np.Type != goheos.MediaTypeStation || np.SourceID != goheos.MusicSourceAmazon {
		t.Errorf("now playing wrong: %+v", np)
	}
	assertGolden(t, "now_playing_media", np)

	// A second call must not hit the device again.
	first := d.ConnectionCount()
	if _, err := c.GetPlayers(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = first
	d.AssertCommandCalled(t, "player/get_players", nil)
}

func TestNowPlayingSupportedControls(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	registerPlayerCommands(d)
	c := connectClient(t, d)
	players, err := c.GetPlayers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Fixture: station from Amazon (sid 13) -> all controls supported.
	np := players[1].NowPlaying()
	if len(np.SupportedControls) != 5 {
		t.Errorf("supported controls = %v", np.SupportedControls)
	}
}

func TestLoadPlayersFirmwareUpdateChangesIDs(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	registerPlayerCommands(d)
	c := connectClient(t, d)
	if _, err := c.GetPlayers(context.Background()); err != nil {
		t.Fatal(err)
	}

	// After a firmware update the same devices come back with new ids:
	// 1 -> 101 (matched by serial) and 2 -> 102 (matched by name+version).
	d.RegisterReplace("player/get_players", nil, "player.get_players_firmware_update")
	registerPlayerStateCommands(d, 101, 102)
	update, err := c.LoadPlayers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(update.UpdatedPlayerIDs) != 2 || update.UpdatedPlayerIDs[1] != 101 || update.UpdatedPlayerIDs[2] != 102 {
		t.Errorf("updated ids = %v", update.UpdatedPlayerIDs)
	}
	if len(update.AddedPlayerIDs) != 0 || len(update.RemovedPlayerIDs) != 0 {
		t.Errorf("unexpected adds/removes: %+v", update)
	}
	players := c.Players()
	if players[101] == nil || players[101].Name() != "Back Patio" {
		t.Errorf("player 101 missing or wrong: %+v", players[101])
	}
	if _, ok := players[1]; ok {
		t.Error("old player id 1 must be gone from the map")
	}
}

func TestPlayerCommands(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	registerPlayerCommands(d)
	pid := map[string]string{"pid": "1"}
	c := connectClient(t, d)
	players, err := c.GetPlayers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p := players[1]
	ctx := context.Background()

	d.Register("player/set_play_state", map[string]string{"pid": "1", "state": "play"}, "player.set_play_state")
	if err := p.Play(ctx); err != nil {
		t.Fatal(err)
	}
	d.Register("player/set_volume", map[string]string{"pid": "1", "level": "25"}, "player.set_volume")
	if err := p.SetVolume(ctx, 25); err != nil {
		t.Fatal(err)
	}
	d.Register("player/volume_up", map[string]string{"pid": "1", "step": "6"}, "player.volume_up")
	if err := p.VolumeUp(ctx, 6); err != nil {
		t.Fatal(err)
	}
	d.Register("player/volume_down", map[string]string{"pid": "1", "step": "5"}, "player.volume_down")
	if err := p.VolumeDown(ctx, 5); err != nil {
		t.Fatal(err)
	}
	d.Register("player/set_mute", map[string]string{"pid": "1", "state": "on"}, "player.set_mute")
	if err := p.Mute(ctx); err != nil {
		t.Fatal(err)
	}
	d.Register("player/toggle_mute", pid, "player.toggle_mute")
	if err := p.ToggleMute(ctx); err != nil {
		t.Fatal(err)
	}
	d.Register("player/set_play_mode",
		map[string]string{"pid": "1", "repeat": "on_all", "shuffle": "on"}, "player.set_play_mode")
	if err := p.SetPlayMode(ctx, goheos.RepeatOnAll, true); err != nil {
		t.Fatal(err)
	}
	d.Register("player/play_next", pid, "player.play_next")
	if err := p.PlayNext(ctx); err != nil {
		t.Fatal(err)
	}
	d.Register("player/play_previous", pid, "player.play_previous")
	if err := p.PlayPrevious(ctx); err != nil {
		t.Fatal(err)
	}
	d.AssertCommandCalled(t, "player/set_play_state", map[string]string{"pid": "1", "state": "play"})
	d.AssertCommandCalled(t, "player/set_volume", map[string]string{"pid": "1", "level": "25"})
}

func TestPlayerQueueCommands(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	registerPlayerCommands(d)
	c := connectClient(t, d)
	players, err := c.GetPlayers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p := players[1]
	ctx := context.Background()

	d.Register("player/get_queue", nil, "player.get_queue")
	queue, err := p.GetQueue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 11 || queue[0].Song != "Baby" || queue[0].QueueID != 1 {
		t.Errorf("queue = %d items, first %+v", len(queue), queue[0])
	}
	assertGolden(t, "queue", queue)

	d.Register("player/get_queue", map[string]string{"pid": "1", "range": "0,10"}, "player.get_queue")
	if _, err := p.GetQueue(ctx, goheos.WithRange(0, 10)); err != nil {
		t.Fatal(err)
	}

	d.Register("player/play_queue", map[string]string{"pid": "1", "qid": "3"}, "player.play_queue")
	if err := p.PlayQueue(ctx, 3); err != nil {
		t.Fatal(err)
	}
	d.Register("player/remove_from_queue", map[string]string{"pid": "1", "qid": "1,2,3"}, "player.remove_from_queue")
	if err := p.RemoveFromQueue(ctx, []int{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	d.Register("player/save_queue", map[string]string{"pid": "1", "name": "Saved Queue"}, "player.save_queue")
	if err := p.SaveQueue(ctx, "Saved Queue"); err != nil {
		t.Fatal(err)
	}
	d.Register("player/clear_queue", map[string]string{"pid": "1"}, "player.clear_queue")
	if err := p.ClearQueue(ctx); err != nil {
		t.Fatal(err)
	}
	d.Register("player/move_queue_item",
		map[string]string{"pid": "1", "sqid": "2,3", "dqid": "1"}, "player.move_queue_item")
	if err := p.MoveQueueItem(ctx, []int{2, 3}, 1); err != nil {
		t.Fatal(err)
	}
	d.AssertCommandCalled(t, "player/remove_from_queue", map[string]string{"pid": "1", "qid": "1,2,3"})
	d.AssertCommandCalled(t, "player/move_queue_item", map[string]string{"pid": "1", "sqid": "2,3", "dqid": "1"})
}

func TestPlayerClearQueueWithUnderProcess(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	registerPlayerCommands(d)
	c := connectClient(t, d)
	players, err := c.GetPlayers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// The device answers "command under process" first, then the result.
	d.Register("player/clear_queue", map[string]string{"pid": "1"},
		"other.command_under_process", "player.clear_queue")
	if err := players[1].ClearQueue(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPlayerQuickSelects(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	registerPlayerCommands(d)
	c := connectClient(t, d)
	players, err := c.GetPlayers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p := players[1]
	ctx := context.Background()

	d.Register("player/get_quickselects", map[string]string{"pid": "1"}, "player.get_quickselects")
	selects, err := p.GetQuickSelects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(selects) != 6 || selects[1] != "Quick Select 1" {
		t.Errorf("quick selects = %v", selects)
	}
	d.Register("player/set_quickselect", map[string]string{"pid": "1", "id": "2"}, "player.set_quickselect")
	if err := p.SetQuickSelect(ctx, 2); err != nil {
		t.Fatal(err)
	}
	d.Register("player/play_quickselect", map[string]string{"pid": "1", "id": "3"}, "player.play_quickselect")
	if err := p.PlayQuickSelect(ctx, 3); err != nil {
		t.Fatal(err)
	}
}

func TestPlayerCheckUpdate(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	registerPlayerCommands(d)
	c := connectClient(t, d)
	players, err := c.GetPlayers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	d.Register("player/check_update", map[string]string{"pid": "1"}, "player.check_update")
	available, err := players[1].CheckUpdate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !available {
		t.Error("fixture reports update_exist; CheckUpdate must return true")
	}
}

func TestGetPlayerInfoNotLoaded(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	// The fixture reports pid -263109739; the follow-up state refresh uses it.
	const pid = -263109739
	d.Register("player/get_player_info", map[string]string{"pid": strconv.Itoa(pid)}, "player.get_player_info")
	registerPlayerStateCommands(d, pid)
	c := connectClient(t, d)

	p, err := c.GetPlayerInfo(context.Background(), pid)
	if err != nil {
		t.Fatal(err)
	}
	if p.ID() != pid || p.Name() != "Zone 1" {
		t.Errorf("player info wrong: id=%d name=%q", p.ID(), p.Name())
	}
}

// A player answering "Requested data not available" to get_play_mode (a HEOS
// Link HS1 stopped after a power cycle) must not fail loading every player,
// and a later refresh keeps the play mode it already knew (pyheos #172).
func TestPlayModeDataNotAvailable(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register("player/get_play_mode", map[string]string{"pid": "2"}, "player.get_play_mode_data_not_available")
	registerPlayerCommands(d)
	c := connectClient(t, d)
	ctx := context.Background()

	players, err := c.GetPlayers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(players) != 2 || players[2].Repeat() != goheos.RepeatOff || players[2].Shuffle() {
		t.Fatalf("players = %d, player 2 play mode = %v/%v", len(players), players[2].Repeat(), players[2].Shuffle())
	}

	events, cancel := c.Subscribe(goheos.WithPlayerEvents())
	t.Cleanup(cancel)
	d.SendEvent(t, "event.repeat_mode_changed", nil)
	waitFor[goheos.PlayerEvent](t, events)
	p := players[1]
	if p.Repeat() != goheos.RepeatOnAll {
		t.Fatalf("repeat after event = %v", p.Repeat())
	}

	d.RegisterReplace("player/get_play_mode", nil, "player.get_play_mode_data_not_available")
	if _, err := c.LoadPlayers(ctx); err != nil {
		t.Fatal(err)
	}
	if p.Repeat() != goheos.RepeatOnAll {
		t.Errorf("refresh dropped the known repeat mode: %v", p.Repeat())
	}
}

// Any other get_play_mode failure still fails the load.
func TestPlayModeOtherFailureFails(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register("player/get_play_mode", map[string]string{"pid": "2"}, "player.get_play_mode_failed")
	registerPlayerCommands(d)
	c := connectClient(t, d)

	_, err := c.GetPlayers(context.Background())
	var failed *goheos.CommandFailedError
	if !errors.As(err, &failed) || failed.ErrorID != goheos.ErrorInvalidID {
		t.Fatalf("err = %v, want ID Not Valid", err)
	}
}
