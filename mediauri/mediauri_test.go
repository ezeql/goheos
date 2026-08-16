package mediauri_test

import (
	"testing"

	"github.com/ezeql/goheos"
	"github.com/ezeql/goheos/mediauri"
)

// Expected URIs are copied verbatim from the pyheos test suite to guarantee
// byte-compatible interop.

var mediaItemAlbum = &goheos.MediaItem{
	SourceID:    10,
	Type:        goheos.MediaTypeAlbum,
	Name:        "After Hours",
	ImageURL:    "http://resources.wimpmusic.com/images/bbe7f53c/44f0/41ba/873f/743e3091adde/160x160.jpg",
	Playable:    true,
	Browsable:   true,
	ContainerID: "LIBALBUM-134788273",
	Artist:      "The Weeknd",
}

var mediaItemInput = &goheos.MediaItem{
	SourceID: -263109739,
	Type:     goheos.MediaTypeStation,
	Name:     "HEOS Drive - AUX In 1",
	Playable: true,
	MediaID:  "inputs/aux_in_1",
}

var mediaItemSong = &goheos.MediaItem{
	SourceID:    10,
	Type:        goheos.MediaTypeSong,
	Name:        "Imaginary Parties",
	ImageURL:    "http://resources.wimpmusic.com/images/7e7bacc1/3e75/4761/a822/9342239edfa0/640x640.jpg",
	Playable:    true,
	ContainerID: "123",
	MediaID:     "456",
	Artist:      "Superfruit",
	Album:       "Future Friends",
	AlbumID:     "78374740",
}

func TestIsMediaURI(t *testing.T) {
	t.Parallel()
	if !mediauri.IsMediaURI("heos://media/10/album") {
		t.Error("heos://media/10/album must be a media uri")
	}
	if mediauri.IsMediaURI("http://example.com") {
		t.Error("http://example.com must not be a media uri")
	}
}

func TestMediaItemRoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		media *goheos.MediaItem
		want  string
	}{
		{
			name:  "album",
			media: mediaItemAlbum,
			want:  "heos://media/10/album?name=After+Hours&image_url=http%3A%2F%2Fresources.wimpmusic.com%2Fimages%2Fbbe7f53c%2F44f0%2F41ba%2F873f%2F743e3091adde%2F160x160.jpg&playable=True&browsable=True&container_id=LIBALBUM-134788273&artist=The+Weeknd",
		},
		{
			name:  "input",
			media: mediaItemInput,
			want:  "heos://media/-263109739/station?name=HEOS+Drive+-+AUX+In+1&image_url=&playable=True&browsable=False&media_id=inputs%2Faux_in_1",
		},
		{
			name:  "song",
			media: mediaItemSong,
			want:  "heos://media/10/song?name=Imaginary+Parties&image_url=http%3A%2F%2Fresources.wimpmusic.com%2Fimages%2F7e7bacc1%2F3e75%2F4761%2Fa822%2F9342239edfa0%2F640x640.jpg&playable=True&browsable=False&container_id=123&media_id=456&artist=Superfruit&album=Future+Friends&album_id=78374740",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			uri, err := mediauri.FromMediaItem(tt.media, nil)
			if err != nil {
				t.Fatal(err)
			}
			if uri != tt.want {
				t.Errorf("uri = %q\nwant  %q", uri, tt.want)
			}
			result, err := mediauri.Parse(uri)
			if err != nil {
				t.Fatal(err)
			}
			if result.MediaItem == nil {
				t.Fatal("expected a media item")
			}
			if *result.MediaItem != *tt.media {
				t.Errorf("round trip mismatch:\n got %+v\nwant %+v", *result.MediaItem, *tt.media)
			}
		})
	}
}

func TestMusicSourceRoundTrip(t *testing.T) {
	t.Parallel()
	source := goheos.MusicSource{
		SourceID:        10,
		Type:            goheos.MediaTypeMusicService,
		Name:            "Tidal",
		ImageURL:        "https://production.ws.skyegloup.com:443/media/images/service/logos/tidal.png",
		Available:       true,
		ServiceUsername: "user@example.com",
	}
	want := "heos://media/10/music_service?name=Tidal&image_url=https%3A%2F%2Fproduction.ws.skyegloup.com%3A443%2Fmedia%2Fimages%2Fservice%2Flogos%2Ftidal.png&available=True&service_username=user%40example.com"
	uri, err := mediauri.FromMusicSource(source, nil)
	if err != nil {
		t.Fatal(err)
	}
	if uri != want {
		t.Errorf("uri = %q\nwant  %q", uri, want)
	}
	result, err := mediauri.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	if result.MusicSource == nil {
		t.Fatal("expected a music source")
	}
	if *result.MusicSource != source {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", *result.MusicSource, source)
	}
}

func TestExtraRoundTrip(t *testing.T) {
	t.Parallel()
	type extra struct {
		RangeStart int `json:"range_start"`
		RangeEnd   int `json:"range_end"`
	}
	uri, err := mediauri.FromMediaItem(mediaItemInput, extra{RangeStart: 0, RangeEnd: 10})
	if err != nil {
		t.Fatal(err)
	}
	result, err := mediauri.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Extra) != `{"range_start":0,"range_end":10}` {
		t.Errorf("extra = %s", result.Extra)
	}
}

func TestParsePythonGeneratedExtra(t *testing.T) {
	t.Parallel()
	// URI produced by pyheos (json.dumps spacing, quote_plus escaping).
	uri := "heos://media/-263109739/station?name=HEOS+Drive+-+AUX+In+1&image_url=&playable=True&browsable=False&media_id=inputs%2Faux_in_1&extra=%7B%22range_start%22%3A+0%2C+%22range_end%22%3A+10%7D"
	result, err := mediauri.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	if result.MediaItem == nil || *result.MediaItem != *mediaItemInput {
		t.Errorf("parsed item mismatch: %+v", result.MediaItem)
	}
	if string(result.Extra) != `{"range_start": 0, "range_end": 10}` {
		t.Errorf("extra = %s", result.Extra)
	}
}

func TestParseInvalidURI(t *testing.T) {
	t.Parallel()
	if _, err := mediauri.Parse("http://example.com"); err == nil {
		t.Error("expected error for non-media uri")
	}
}
