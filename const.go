package goheos

// TargetVersion is the minimum HEOS firmware version fully supported by the
// library.
const TargetVersion = "3.34.0"

// DefaultPort is the TCP port of the HEOS CLI protocol.
const DefaultPort = 1255

// Protocol framing.
const (
	basePrefix = "heos://"
	separator  = "\r\n"
)

// Message attribute keys (query-string parameter names of the CLI protocol).
const (
	attrAddCriteriaID      = "aid"
	attrAlbum              = "album"
	attrAlbumID            = "album_id"
	attrArtist             = "artist"
	attrAvailable          = "available"
	attrCommand            = "command"
	attrContainer          = "container"
	attrContainerID        = "cid"
	attrControl            = "control"
	attrCount              = "count"
	attrCurrentPosition    = "cur_pos"
	attrDestinationQueueID = "dqid"
	attrDuration           = "duration"
	attrEnable             = "enable"
	attrError              = "error"
	attrErrorID            = "eid"
	attrErrorNumber        = "errno"
	attrGroupID            = "gid"
	attrID                 = "id"
	attrImageURL           = "image_url"
	attrImages             = "images"
	attrInput              = "input"
	attrIPAddress          = "ip"
	attrLevel              = "level"
	attrLineOut            = "lineout"
	attrMediaID            = "mid"
	attrMessage            = "message"
	attrModel              = "model"
	attrMute               = "mute"
	attrName               = "name"
	attrNetwork            = "network"
	attrOptionID           = "option"
	attrOptions            = "options"
	attrPassword           = "pw"
	attrPayload            = "payload"
	attrPlayable           = "playable"
	attrPlayerID           = "pid"
	attrPlayers            = "players"
	attrPreset             = "preset"
	attrQueueID            = "qid"
	attrRange              = "range"
	attrRefresh            = "refresh"
	attrRepeat             = "repeat"
	attrResult             = "result"
	attrReturned           = "returned"
	attrRole               = "role"
	attrSearch             = "search"
	attrSearchCriteriaID   = "scid"
	attrSerial             = "serial"
	attrServiceUserName    = "service_username"
	attrShuffle            = "shuffle"
	attrSignedIn           = "signed_in"
	attrSignedOut          = "signed_out"
	attrSong               = "song"
	attrSourceID           = "sid"
	attrSourcePlayerID     = "spid"
	attrSourceQueueID      = "sqid"
	attrState              = "state"
	attrStats              = "stats"
	attrStation            = "station"
	attrStep               = "step"
	attrSystemErrorNumber  = "syserrno"
	attrText               = "text"
	attrType               = "type"
	attrUpdate             = "update"
	attrURL                = "url"
	attrUserName           = "un"
	attrVersion            = "version"
	attrWidth              = "width"
	attrWildcard           = "wildcard"
)

// Message attribute values.
const (
	valueOn           = "on"
	valueOff          = "off"
	valueTrue         = "true"
	valueFalse        = "false"
	valueYes          = "yes"
	valueNo           = "no"
	valueSuccess      = "success"
	valueLeader       = "leader"
	valueMember       = "member"
	valueUpdateExist  = "update_exist"
	valueUnderProcess = "command under process"
)

// Browse commands.
const (
	cmdBrowseAddToQueue        = "browse/add_to_queue"
	cmdBrowseBrowse            = "browse/browse"
	cmdBrowseDeletePlaylist    = "browse/delete_playlist"
	cmdBrowseGetSearchCriteria = "browse/get_search_criteria"
	cmdBrowseGetSourceInfo     = "browse/get_source_info"
	cmdBrowseGetSources        = "browse/get_music_sources"
	cmdBrowseMultiSearch       = "browse/multi_search"
	cmdBrowsePlayInput         = "browse/play_input"
	cmdBrowsePlayPreset        = "browse/play_preset"
	cmdBrowsePlayStream        = "browse/play_stream"
	cmdBrowseRenamePlaylist    = "browse/rename_playlist"
	cmdBrowseRetrieveMetadata  = "browse/retrieve_metadata"
	cmdBrowseSearch            = "browse/search"
	cmdBrowseSetServiceOption  = "browse/set_service_option"
)

// Player commands.
const (
	cmdCheckUpdate        = "player/check_update"
	cmdClearQueue         = "player/clear_queue"
	cmdGetMute            = "player/get_mute"
	cmdGetNowPlayingMedia = "player/get_now_playing_media"
	cmdGetPlayMode        = "player/get_play_mode"
	cmdGetPlayState       = "player/get_play_state"
	cmdGetPlayerInfo      = "player/get_player_info"
	cmdGetPlayers         = "player/get_players"
	cmdGetQueue           = "player/get_queue"
	cmdGetQuickSelects    = "player/get_quickselects"
	cmdGetVolume          = "player/get_volume"
	cmdMoveQueueItem      = "player/move_queue_item"
	cmdPlayNext           = "player/play_next"
	cmdPlayPrevious       = "player/play_previous"
	cmdPlayQueue          = "player/play_queue"
	cmdPlayQuickSelect    = "player/play_quickselect"
	cmdRemoveFromQueue    = "player/remove_from_queue"
	cmdSaveQueue          = "player/save_queue"
	cmdSetMute            = "player/set_mute"
	cmdSetPlayMode        = "player/set_play_mode"
	cmdSetPlayState       = "player/set_play_state"
	cmdSetQuickSelect     = "player/set_quickselect"
	cmdSetVolume          = "player/set_volume"
	cmdToggleMute         = "player/toggle_mute"
	cmdVolumeDown         = "player/volume_down"
	cmdVolumeUp           = "player/volume_up"
)

// Group commands.
const (
	cmdGetGroupInfo    = "group/get_group_info"
	cmdGetGroupMute    = "group/get_mute"
	cmdGetGroupVolume  = "group/get_volume"
	cmdGetGroups       = "group/get_groups"
	cmdGroupToggleMute = "group/toggle_mute"
	cmdGroupVolumeDown = "group/volume_down"
	cmdGroupVolumeUp   = "group/volume_up"
	cmdSetGroup        = "group/set_group"
	cmdSetGroupMute    = "group/set_mute"
	cmdSetGroupVolume  = "group/set_volume"
)

// System commands.
const (
	cmdAccountCheck            = "system/check_account"
	cmdHeartBeat               = "system/heart_beat"
	cmdRegisterForChangeEvents = "system/register_for_change_events"
	cmdReboot                  = "system/reboot"
	cmdSignIn                  = "system/sign_in"
	cmdSignOut                 = "system/sign_out"
)

// EventCommand identifies a HEOS change event received from the device.
type EventCommand string

// HEOS change events.
const (
	EventPlayerStateChanged       EventCommand = "event/player_state_changed"
	EventPlayerNowPlayingChanged  EventCommand = "event/player_now_playing_changed"
	EventPlayerNowPlayingProgress EventCommand = "event/player_now_playing_progress"
	EventPlayerVolumeChanged      EventCommand = "event/player_volume_changed"
	EventPlayerPlaybackError      EventCommand = "event/player_playback_error"
	EventPlayerQueueChanged       EventCommand = "event/player_queue_changed"
	EventGroupVolumeChanged       EventCommand = "event/group_volume_changed"
	EventRepeatModeChanged        EventCommand = "event/repeat_mode_changed"
	EventShuffleModeChanged       EventCommand = "event/shuffle_mode_changed"
	EventSourcesChanged           EventCommand = "event/sources_changed"
	EventPlayersChanged           EventCommand = "event/players_changed"
	EventGroupsChanged            EventCommand = "event/groups_changed"
	EventUserChanged              EventCommand = "event/user_changed"
)

// debouncedEvents are system-wide events the device emits in bursts; they are
// debounced before dispatch.
var debouncedEvents = map[EventCommand]bool{
	EventSourcesChanged: true,
	EventPlayersChanged: true,
	EventGroupsChanged:  true,
	EventUserChanged:    true,
}

// playerEvents are events routed to an individual player.
var playerEvents = map[EventCommand]bool{
	EventPlayerStateChanged:       true,
	EventPlayerNowPlayingChanged:  true,
	EventPlayerNowPlayingProgress: true,
	EventPlayerVolumeChanged:      true,
	EventRepeatModeChanged:        true,
	EventShuffleModeChanged:       true,
	EventPlayerPlaybackError:      true,
	EventPlayerQueueChanged:       true,
}

// groupEvents are events routed to an individual group.
var groupEvents = map[EventCommand]bool{
	EventGroupVolumeChanged: true,
}

// heosEvents are system-wide (controller) events.
var heosEvents = map[EventCommand]bool{
	EventSourcesChanged: true,
	EventPlayersChanged: true,
	EventGroupsChanged:  true,
	EventUserChanged:    true,
}

// SearchedTracksPrefix is the container id prefix used by add_search_to_queue.
const SearchedTracksPrefix = "SEARCHED_TRACKS-"

// Music source ids (fixed values assigned by the HEOS system).
const (
	MusicSourceConnect     = 0 // TIDAL Connect, possibly Spotify Connect as well
	MusicSourcePandora     = 1
	MusicSourceRhapsody    = 2
	MusicSourceTuneIn      = 3
	MusicSourceSpotify     = 4
	MusicSourceDeezer      = 5
	MusicSourceNapster     = 6
	MusicSourceIHeartRadio = 7
	MusicSourceSiriusXM    = 8
	MusicSourceSoundcloud  = 9
	MusicSourceTidal       = 10
	MusicSourceRdio        = 12
	MusicSourceAmazon      = 13
	MusicSourceMoodmix     = 15
	MusicSourceJuke        = 16
	MusicSourceQQMusic     = 18
	MusicSourceLocalMusic  = 1024
	MusicSourcePlaylists   = 1025
	MusicSourceHistory     = 1026
	MusicSourceAuxInput    = 1027
	MusicSourceFavorites   = 1028
)

// Input source names (fixed values assigned by the HEOS system).
const (
	InputAnalogIn1   = "inputs/analog_in_1"
	InputAnalogIn2   = "inputs/analog_in_2"
	InputAux8K       = "inputs/aux_8k"
	InputAuxIn1      = "inputs/aux_in_1"
	InputAuxIn2      = "inputs/aux_in_2"
	InputAuxIn3      = "inputs/aux_in_3"
	InputAuxIn4      = "inputs/aux_in_4"
	InputAuxInSingle = "inputs/aux_single"
	InputAux1        = "inputs/aux1"
	InputAux2        = "inputs/aux2"
	InputAux3        = "inputs/aux3"
	InputAux4        = "inputs/aux4"
	InputAux5        = "inputs/aux5"
	InputAux6        = "inputs/aux6"
	InputAux7        = "inputs/aux7"
	InputBluray      = "inputs/bluray"
	InputCableSat    = "inputs/cable_sat"
	InputCD          = "inputs/cd"
	InputCoaxIn1     = "inputs/coax_in_1"
	InputCoaxIn2     = "inputs/coax_in_2"
	InputDVD         = "inputs/dvd"
	InputGame        = "inputs/game"
	InputGame2       = "inputs/game2"
	InputHDRadio     = "inputs/hdradio"
	InputHDMIARC1    = "inputs/hdmi_arc_1"
	InputHDMIIn1     = "inputs/hdmi_in_1"
	InputHDMIIn2     = "inputs/hdmi_in_2"
	InputHDMIIn3     = "inputs/hdmi_in_3"
	InputHDMIIn4     = "inputs/hdmi_in_4"
	InputLineIn1     = "inputs/line_in_1"
	InputLineIn2     = "inputs/line_in_2"
	InputLineIn3     = "inputs/line_in_3"
	InputLineIn4     = "inputs/line_in_4"
	InputMediaPlayer = "inputs/mediaplayer"
	InputOpticalIn1  = "inputs/optical_in_1"
	InputOpticalIn2  = "inputs/optical_in_2"
	InputOpticalIn3  = "inputs/optical_in_3"
	InputPhono       = "inputs/phono"
	InputRecorderIn1 = "inputs/recorder_in_1"
	InputSource1     = "inputs/source1"
	InputSource2     = "inputs/source2"
	InputSource3     = "inputs/source3"
	InputSource4     = "inputs/source4"
	InputSource5     = "inputs/source5"
	InputSource6     = "inputs/source6"
	InputSource7     = "inputs/source7"
	InputSource8     = "inputs/source8"
	InputSource9     = "inputs/source9"
	InputSource10    = "inputs/source10"
	InputSource11    = "inputs/source11"
	InputSource12    = "inputs/source12"
	InputSource13    = "inputs/source13"
	InputSource14    = "inputs/source14"
	InputSource15    = "inputs/source15"
	InputSource16    = "inputs/source16"
	InputSource17    = "inputs/source17"
	InputSource18    = "inputs/source18"
	InputTuner       = "inputs/tuner"
	InputTV          = "inputs/tv"
	InputTVAudio     = "inputs/tvaudio"
	InputUSBDAC      = "inputs/usbdac"
)

// validInputs is the set of all known input source names.
var validInputs = map[string]bool{
	InputAnalogIn1: true, InputAnalogIn2: true, InputAux8K: true,
	InputAuxIn1: true, InputAuxIn2: true, InputAuxIn3: true, InputAuxIn4: true,
	InputAuxInSingle: true, InputAux1: true, InputAux2: true, InputAux3: true,
	InputAux4: true, InputAux5: true, InputAux6: true, InputAux7: true,
	InputBluray: true, InputCableSat: true, InputCD: true, InputCoaxIn1: true,
	InputCoaxIn2: true, InputDVD: true, InputGame: true, InputGame2: true,
	InputHDRadio: true, InputHDMIARC1: true, InputHDMIIn1: true, InputHDMIIn2: true,
	InputHDMIIn3: true, InputHDMIIn4: true, InputLineIn1: true, InputLineIn2: true,
	InputLineIn3: true, InputLineIn4: true, InputMediaPlayer: true,
	InputOpticalIn1: true, InputOpticalIn2: true, InputOpticalIn3: true,
	InputPhono: true, InputRecorderIn1: true, InputSource1: true, InputSource2: true,
	InputSource3: true, InputSource4: true, InputSource5: true, InputSource6: true,
	InputSource7: true, InputSource8: true, InputSource9: true, InputSource10: true,
	InputSource11: true, InputSource12: true, InputSource13: true, InputSource14: true,
	InputSource15: true, InputSource16: true, InputSource17: true, InputSource18: true,
	InputTuner: true, InputTV: true, InputTVAudio: true, InputUSBDAC: true,
}

// ServiceOptionID identifies a service option of browse/set_service_option.
type ServiceOptionID int

// Service options (fixed values assigned by the HEOS system).
const (
	ServiceOptionAddTrackToLibrary                ServiceOptionID = 1
	ServiceOptionAddAlbumToLibrary                ServiceOptionID = 2
	ServiceOptionAddStationToLibrary              ServiceOptionID = 3
	ServiceOptionAddPlaylistToLibrary             ServiceOptionID = 4
	ServiceOptionRemoveTrackFromLibrary           ServiceOptionID = 5
	ServiceOptionRemoveAlbumFromLibrary           ServiceOptionID = 6
	ServiceOptionRemoveStationFromLibrary         ServiceOptionID = 7
	ServiceOptionRemovePlaylistFromLibrary        ServiceOptionID = 8
	ServiceOptionThumbsUp                         ServiceOptionID = 11
	ServiceOptionThumbsDown                       ServiceOptionID = 12
	ServiceOptionCreateNewStationBySearchCriteria ServiceOptionID = 13
	ServiceOptionAddToFavorites                   ServiceOptionID = 19
	ServiceOptionRemoveFromFavorites              ServiceOptionID = 20
)
