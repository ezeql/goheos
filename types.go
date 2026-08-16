package goheos

// AddCriteriaType defines the options for adding media to the queue.
type AddCriteriaType int

// Add-to-queue criteria.
const (
	AddCriteriaPlayNow        AddCriteriaType = 1
	AddCriteriaPlayNext       AddCriteriaType = 2
	AddCriteriaAddToEnd       AddCriteriaType = 3
	AddCriteriaReplaceAndPlay AddCriteriaType = 4
)

// ConnectionState defines the possible connection states of a Client.
type ConnectionState string

// Connection states.
const (
	StateConnected    ConnectionState = "connected"
	StateDisconnected ConnectionState = "disconnected"
	StateReconnecting ConnectionState = "reconnecting"
)

// LineOutLevelType defines the line out level types.
type LineOutLevelType int

// Line out level types.
const (
	LineOutLevelUnknown  LineOutLevelType = 0
	LineOutLevelVariable LineOutLevelType = 1
	LineOutLevelFixed    LineOutLevelType = 2
)

// VolumeControlType defines the volume control types.
type VolumeControlType int

// Volume control types.
const (
	VolumeControlUnknown VolumeControlType = 0
	VolumeControlNone    VolumeControlType = 1
	VolumeControlIR      VolumeControlType = 2
	VolumeControlTrigger VolumeControlType = 3
	VolumeControlNetwork VolumeControlType = 4
)

// NetworkType defines how a player is connected to the network.
type NetworkType string

// Network types.
const (
	NetworkWired   NetworkType = "wired"
	NetworkWifi    NetworkType = "wifi"
	NetworkUnknown NetworkType = "unknown"
)

// ControlType defines the playback controls supported by the current media.
type ControlType string

// Playback control types.
const (
	ControlPlay         ControlType = "play"
	ControlPause        ControlType = "pause"
	ControlStop         ControlType = "stop"
	ControlPlayNext     ControlType = "play_next"
	ControlPlayPrevious ControlType = "play_previous"
)

// MediaType defines the media types. Unrecognized values received from the
// device are preserved as-is rather than degraded to a sentinel.
type MediaType string

// Media types.
const (
	MediaTypeAlbum        MediaType = "album"
	MediaTypeArtist       MediaType = "artist"
	MediaTypeContainer    MediaType = "container"
	MediaTypeDLNAServer   MediaType = "dlna_server"
	MediaTypeGenre        MediaType = "genre"
	MediaTypeHeosServer   MediaType = "heos_server"
	MediaTypeHeosService  MediaType = "heos_service"
	MediaTypeMusicService MediaType = "music_service"
	MediaTypePlaylist     MediaType = "playlist"
	MediaTypeSong         MediaType = "song"
	MediaTypeStation      MediaType = "station"
)

// PlayState defines the play states of a player.
type PlayState string

// Play states.
const (
	PlayStatePlay    PlayState = "play"
	PlayStatePause   PlayState = "pause"
	PlayStateStop    PlayState = "stop"
	PlayStateUnknown PlayState = "unknown"
)

// RepeatType defines the repeat modes of a player.
type RepeatType string

// Repeat modes.
const (
	RepeatOnAll RepeatType = "on_all"
	RepeatOnOne RepeatType = "on_one"
	RepeatOff   RepeatType = "off"
)
