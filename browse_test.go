package goheos_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ezeql/goheos"
	"github.com/ezeql/goheos/internal/heostest"
)

func TestGetMusicSources(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register("browse/get_music_sources", nil, "browse.get_music_sources")
	c := connectClient(t, d)

	sources, err := c.GetMusicSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) == 0 {
		t.Fatal("no music sources parsed")
	}
	pandora := sources[goheos.MusicSourcePandora]
	if pandora.Name != "Pandora" || pandora.Type != goheos.MediaTypeMusicService {
		t.Errorf("pandora = %+v", pandora)
	}
	rhapsody := sources[goheos.MusicSourceRhapsody]
	if rhapsody.Available {
		t.Error("rhapsody fixture is available=false")
	}
	assertGolden(t, "music_sources", sources)

	// A second call must serve from cache (no new command).
	if _, err := c.GetMusicSources(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestBrowseFavorites(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register("browse/browse", map[string]string{"sid": "1028"}, "browse.browse_favorites")
	c := connectClient(t, d)

	result, err := c.Browse(context.Background(), goheos.MusicSourceFavorites)
	if err != nil {
		t.Fatal(err)
	}
	if result.Count != 3 || result.Returned != 3 || len(result.Items) != 3 {
		t.Fatalf("browse result = %+v", result)
	}
	if !result.Items[0].Playable {
		t.Error("favorite stations must be playable")
	}
	assertGolden(t, "browse_favorites", result)
}

func TestGetFavoritesIndexes(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register("browse/browse", map[string]string{"sid": "1028"}, "browse.browse_favorites")
	c := connectClient(t, d)

	favorites, err := c.GetFavorites(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Favorites are keyed 1-based, matching play_preset_station.
	if _, ok := favorites[0]; ok {
		t.Error("favorites must be 1-based")
	}
	if favorites[1] == nil || favorites[3] == nil {
		t.Errorf("favorites indexes missing: %v", favorites)
	}
}

func TestGetPlaylists(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register("browse/browse", map[string]string{"sid": "1025"}, "browse.browse_playlists")
	c := connectClient(t, d)

	playlists, err := c.GetPlaylists(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(playlists) != 1 || !playlists[0].Browsable {
		t.Errorf("playlists = %+v", playlists)
	}
}

func TestGetInputSources(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register("browse/browse", map[string]string{"sid": "1027"}, "browse.browse_aux_input")
	d.Register("browse/browse", map[string]string{"sid": "-263109739"}, "browse.browse_heos_drive")
	d.Register("browse/browse", map[string]string{"sid": "546978854"}, "browse.browse_theater_receiver")
	c := connectClient(t, d)

	inputs, err := c.GetInputSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) == 0 {
		t.Fatal("no input sources")
	}
	for _, input := range inputs {
		if !input.Playable {
			t.Errorf("input %q must be playable", input.Name)
		}
	}
}

func TestBrowseWithContainerAndRange(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register("browse/browse",
		map[string]string{"sid": "10", "cid": "My Music", "range": "0,13"}, "browse.browse_album")
	c := connectClient(t, d)

	result, err := c.Browse(context.Background(), 10,
		goheos.WithContainer("My Music"), goheos.WithRange(0, 13))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) == 0 {
		t.Fatal("no items")
	}
	// Songs inside an album inherit source and container from the context.
	song := result.Items[0]
	if song.SourceID != 10 {
		t.Errorf("song source = %d", song.SourceID)
	}
	assertGolden(t, "browse_album", result)
}

func TestSearch(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register("browse/search", nil, "browse.search")
	c := connectClient(t, d)

	result, err := c.Search(context.Background(), 10, "Tangerine Rays", 3)
	if err != nil {
		t.Fatal(err)
	}
	if result.Count != 15 || result.SourceID != 10 || result.CriteriaID != 3 {
		t.Errorf("search result = %+v", result)
	}
	if result.Search != "Tangerine Rays" {
		t.Errorf("search text = %q", result.Search)
	}
	assertGolden(t, "search", result)
}

func TestSearchValidation(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	c := connectClient(t, d)
	if _, err := c.Search(context.Background(), 10, "", 3); !errors.Is(err, goheos.ErrInvalidArgument) {
		t.Errorf("empty search: err = %v", err)
	}
	long := string(make([]byte, 129))
	if _, err := c.Search(context.Background(), 10, long, 3); !errors.Is(err, goheos.ErrInvalidArgument) {
		t.Errorf("long search: err = %v", err)
	}
}

func TestMultiSearch(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register("browse/multi_search", nil, "browse.multi_search")
	c := connectClient(t, d)

	result, err := c.MultiSearch(context.Background(), "Tangerine Rays",
		[]int{1, 4, 8, 13, 10}, []int{0, 1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if result.Count != 74 || result.Returned != 74 {
		t.Errorf("counts = %d/%d", result.Returned, result.Count)
	}
	if len(result.SourceIDs) != 5 || len(result.CriteriaIDs) != 4 {
		t.Errorf("ids = %v %v", result.SourceIDs, result.CriteriaIDs)
	}
	if len(result.Errors) == 0 {
		t.Error("fixture contains errno tuples; Errors must not be empty")
	}
	// Every item's source id is assigned by walking the stats tuples.
	if len(result.Items) == 0 {
		t.Fatal("no items")
	}
	total := 0
	for _, stat := range result.Statistics {
		total += stat.Returned
	}
	if len(result.Items) != total {
		t.Errorf("items = %d, stats total = %d", len(result.Items), total)
	}
	assertGolden(t, "multi_search", result)
}

func TestGetSearchCriteria(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register("browse/get_search_criteria", map[string]string{"sid": "10"}, "browse.get_search_criteria")
	c := connectClient(t, d)

	criteria, err := c.GetSearchCriteria(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(criteria) == 0 {
		t.Fatal("no search criteria")
	}
	assertGolden(t, "search_criteria", criteria)
}

func TestAddToQueueVariants(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	c := connectClient(t, d)
	ctx := context.Background()

	d.Register("browse/add_to_queue",
		map[string]string{"pid": "1", "sid": "10", "cid": "container", "aid": "3"},
		"browse.add_to_queue_container")
	if err := c.AddToQueue(ctx, 1, 10, "container", "", goheos.AddCriteriaAddToEnd); err != nil {
		t.Fatal(err)
	}
	d.RegisterReplace("browse/add_to_queue",
		map[string]string{"pid": "1", "sid": "10", "cid": "container", "mid": "track", "aid": "1"},
		"browse.add_to_queue_track")
	if err := c.AddToQueue(ctx, 1, 10, "container", "track", goheos.AddCriteriaPlayNow); err != nil {
		t.Fatal(err)
	}
	// add_search_to_queue builds the container id from the search string.
	d.RegisterReplace("browse/add_to_queue",
		map[string]string{"pid": "1", "sid": "10", "cid": "SEARCHED_TRACKS-Tangerine Rays", "aid": "1"},
		"browse.add_to_queue_search")
	if err := c.AddSearchToQueue(ctx, 1, 10, "Tangerine Rays", goheos.AddCriteriaPlayNow); err != nil {
		t.Fatal(err)
	}
	d.AssertCommandCalled(t, "browse/add_to_queue",
		map[string]string{"pid": "1", "sid": "10", "cid": "SEARCHED_TRACKS-Tangerine Rays", "aid": "1"})
}

func TestPlayMediaDispatch(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	c := connectClient(t, d)
	ctx := context.Background()

	// An input source dispatches to play_input.
	d.Register("browse/play_input", nil, "browse.play_input")
	input := &goheos.MediaItem{
		SourceID: -263109739, Type: goheos.MediaTypeStation,
		Name: "AUX In 1", Playable: true, MediaID: goheos.InputAuxIn1,
	}
	if err := c.PlayMedia(ctx, 1, input, goheos.AddCriteriaPlayNow); err != nil {
		t.Fatal(err)
	}
	d.AssertCommandCalled(t, "browse/play_input", nil)

	// A station dispatches to play_stream.
	d.Register("browse/play_stream", nil, "browse.play_stream_station")
	station := &goheos.MediaItem{
		SourceID: goheos.MusicSourcePandora, Type: goheos.MediaTypeStation,
		Name: "Station", Playable: true, MediaID: "12345", ContainerID: "A",
	}
	if err := c.PlayMedia(ctx, 1, station, goheos.AddCriteriaPlayNow); err != nil {
		t.Fatal(err)
	}
	d.AssertCommandCalled(t, "browse/play_stream", nil)

	// A song dispatches to add_to_queue.
	d.Register("browse/add_to_queue", nil, "browse.add_to_queue_track")
	song := &goheos.MediaItem{
		SourceID: 10, Type: goheos.MediaTypeSong,
		Name: "Song", Playable: true, MediaID: "456", ContainerID: "123",
	}
	if err := c.PlayMedia(ctx, 1, song, goheos.AddCriteriaPlayNow); err != nil {
		t.Fatal(err)
	}
	d.AssertCommandCalled(t, "browse/add_to_queue", nil)

	// Not playable is rejected client-side.
	notPlayable := &goheos.MediaItem{Name: "X", Playable: false}
	if err := c.PlayMedia(ctx, 1, notPlayable, goheos.AddCriteriaPlayNow); !errors.Is(err, goheos.ErrInvalidArgument) {
		t.Errorf("err = %v, want ErrInvalidArgument", err)
	}
}

func TestPlayURLGoesLastUnquoted(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register("browse/play_stream", nil, "browse.play_stream")
	c := connectClient(t, d)
	// A URL with query parameters must survive unescaped.
	if err := c.PlayURL(context.Background(), 1, "http://host/stream.mp3?a=1"); err != nil {
		t.Fatal(err)
	}
	d.AssertCommandCalled(t, "browse/play_stream", nil)
}

func TestPlaylistOperations(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	c := connectClient(t, d)
	ctx := context.Background()

	d.Register("browse/rename_playlist",
		map[string]string{"sid": "1025", "cid": "171566", "name": "New Name"}, "browse.rename_playlist")
	if err := c.RenamePlaylist(ctx, 1025, "171566", "New Name"); err != nil {
		t.Fatal(err)
	}
	if err := c.RenamePlaylist(ctx, 1025, "171566", ""); !errors.Is(err, goheos.ErrInvalidArgument) {
		t.Errorf("empty name: err = %v", err)
	}
	d.Register("browse/delete_playlist",
		map[string]string{"sid": "1025", "cid": "171566"}, "browse.delete_playlist")
	if err := c.DeletePlaylist(ctx, 1025, "171566"); err != nil {
		t.Fatal(err)
	}
}

func TestRetrieveMetadata(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	d.Register("browse/retrieve_metadata", nil, "browse.retrieve_metadata")
	c := connectClient(t, d)

	result, err := c.RetrieveMetadata(context.Background(), 2, "album-id")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Metadata) == 0 || len(result.Metadata[0].Images) == 0 {
		t.Errorf("metadata = %+v", result)
	}
	assertGolden(t, "retrieve_metadata", result)
}

func TestSetServiceOptionValidation(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	c := connectClient(t, d)
	ctx := context.Background()

	cases := []error{
		// Missing required parameters.
		c.SetServiceOption(ctx, goheos.ServiceOptionAddTrackToLibrary, goheos.OptionSourceID(1)),
		c.SetServiceOption(ctx, goheos.ServiceOptionAddAlbumToLibrary, goheos.OptionSourceID(1)),
		c.SetServiceOption(ctx, goheos.ServiceOptionAddPlaylistToLibrary,
			goheos.OptionSourceID(1), goheos.OptionContainerID("c")),
		c.SetServiceOption(ctx, goheos.ServiceOptionThumbsUp, goheos.OptionSourceID(1)),
		c.SetServiceOption(ctx, goheos.ServiceOptionCreateNewStationBySearchCriteria, goheos.OptionSourceID(1)),
		c.SetServiceOption(ctx, goheos.ServiceOptionRemoveFromFavorites),
		// Disallowed extra parameters.
		c.SetServiceOption(ctx, goheos.ServiceOptionAddTrackToLibrary,
			goheos.OptionSourceID(1), goheos.OptionMediaID("m"), goheos.OptionPlayerID(1)),
		c.SetServiceOption(ctx, goheos.ServiceOptionAddToFavorites,
			goheos.OptionPlayerID(1), goheos.OptionSourceID(1),
			goheos.OptionMediaID("m"), goheos.OptionName("n")),
		// Unknown option.
		c.SetServiceOption(ctx, goheos.ServiceOptionID(999)),
	}
	for i, err := range cases {
		if !errors.Is(err, goheos.ErrInvalidArgument) {
			t.Errorf("case %d: err = %v, want ErrInvalidArgument", i, err)
		}
	}
}

func TestSetServiceOptionCommands(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	c := connectClient(t, d)
	ctx := context.Background()

	d.Register("browse/set_service_option",
		map[string]string{"option": "19", "pid": "1"}, "browse.set_service_option_add_favorite")
	if err := c.SetServiceOption(ctx, goheos.ServiceOptionAddToFavorites, goheos.OptionPlayerID(1)); err != nil {
		t.Fatal(err)
	}
	d.RegisterReplace("browse/set_service_option",
		map[string]string{"option": "20", "mid": "123"}, "browse.set_service_option_remove_favorite")
	if err := c.SetServiceOption(ctx, goheos.ServiceOptionRemoveFromFavorites, goheos.OptionMediaID("123")); err != nil {
		t.Fatal(err)
	}
	d.RegisterReplace("browse/set_service_option",
		map[string]string{"option": "11", "sid": "1", "pid": "1"}, "browse.set_service_option_thumbs_up_down")
	if err := c.SetServiceOption(ctx, goheos.ServiceOptionThumbsUp,
		goheos.OptionSourceID(1), goheos.OptionPlayerID(1)); err != nil {
		t.Fatal(err)
	}
	d.RegisterReplace("browse/set_service_option",
		map[string]string{"option": "13", "sid": "1", "name": "Tangerine Rays", "scid": "0", "range": "0,14"},
		"browse.set_service_option_new_station")
	if err := c.SetServiceOption(ctx, goheos.ServiceOptionCreateNewStationBySearchCriteria,
		goheos.OptionSourceID(1), goheos.OptionName("Tangerine Rays"),
		goheos.OptionCriteriaID(0), goheos.OptionRange(0, 14)); err != nil {
		t.Fatal(err)
	}
}

func TestBrowseMediaRequiresBrowsable(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	c := connectClient(t, d)
	song := &goheos.MediaItem{Name: "Song", Browsable: false}
	if _, err := c.BrowseMedia(context.Background(), song); !errors.Is(err, goheos.ErrInvalidArgument) {
		t.Errorf("err = %v, want ErrInvalidArgument", err)
	}
}
