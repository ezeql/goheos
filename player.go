package goheos

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Playback control sets.
var (
	controlsAll = []ControlType{
		ControlPlay, ControlPause, ControlStop, ControlPlayNext, ControlPlayPrevious,
	}
	controlsForwardOnly = []ControlType{
		ControlPlay, ControlPause, ControlStop, ControlPlayNext,
	}
	controlsPlayStop = []ControlType{ControlPlay, ControlStop}
)

// sourceControls maps music source id and media type to the playback controls
// the source supports. This is client-side knowledge, not reported by the
// device.
var sourceControls = map[int]map[MediaType][]ControlType{
	MusicSourceConnect:  {MediaTypeStation: controlsAll},
	MusicSourcePandora:  {MediaTypeStation: controlsForwardOnly},
	MusicSourceRhapsody: {MediaTypeSong: controlsAll, MediaTypeStation: controlsForwardOnly},
	MusicSourceTuneIn:   {MediaTypeSong: controlsAll, MediaTypeStation: controlsPlayStop},
	MusicSourceSpotify:  {MediaTypeSong: controlsAll, MediaTypeStation: controlsForwardOnly},
	MusicSourceDeezer:   {MediaTypeSong: controlsAll, MediaTypeStation: controlsForwardOnly},
	MusicSourceNapster:  {MediaTypeSong: controlsAll, MediaTypeStation: controlsForwardOnly},
	MusicSourceIHeartRadio: {
		MediaTypeSong:    controlsAll,
		MediaTypeStation: controlsPlayStop,
	},
	MusicSourceSiriusXM:   {MediaTypeStation: controlsPlayStop},
	MusicSourceSoundcloud: {MediaTypeSong: controlsAll},
	MusicSourceTidal:      {MediaTypeSong: controlsAll},
	MusicSourceAmazon:     {MediaTypeSong: controlsAll, MediaTypeStation: controlsAll},
	MusicSourceAuxInput:   {MediaTypeStation: controlsPlayStop},
}

// PlayerUpdateResult describes how the set of players changed after a
// refresh. UpdatedPlayerIDs maps a previous player id to its new id: player
// ids can change after a firmware update.
type PlayerUpdateResult struct {
	AddedPlayerIDs   []int
	RemovedPlayerIDs []int
	UpdatedPlayerIDs map[int]int
}

// NowPlayingMedia is a snapshot of what a player is currently playing.
type NowPlayingMedia struct {
	Type     MediaType
	Song     string
	Station  string
	Album    string
	Artist   string
	ImageURL string
	AlbumID  string
	MediaID  string
	// QueueID is 1-based; zero when not playing from the queue.
	QueueID     int
	SourceID    int
	HasSourceID bool
	// CurrentPosition and Duration are reported in milliseconds; zero values
	// with a zero CurrentPositionUpdated mean no progress has been reported.
	CurrentPosition        int
	CurrentPositionUpdated time.Time
	Duration               int
	SupportedControls      []ControlType
	Options                []ServiceOption
}

func (n *NowPlayingMedia) updateFromMessage(m *message) error {
	data, err := decodePayloadObject(m.payload)
	if err != nil {
		return err
	}
	n.Type = MediaType(data.str(attrType))
	n.Song = data.str(attrSong)
	n.Station = data.str(attrStation)
	n.Album = data.str(attrAlbum)
	n.Artist = data.str(attrArtist)
	n.ImageURL = data.str(attrImageURL)
	n.AlbumID = data.str(attrAlbumID)
	n.MediaID = data.str(attrMediaID)
	n.QueueID, _ = data.intVal(attrQueueID)
	n.SourceID, n.HasSourceID = data.intVal(attrSourceID)
	options, err := parseServiceOptions(m.options)
	if err != nil {
		return err
	}
	n.Options = options
	n.updateSupportedControls()
	n.clearProgress()
	return nil
}

func (n *NowPlayingMedia) updateSupportedControls() {
	if !n.HasSourceID {
		n.SupportedControls = nil
		return
	}
	controls := controlsAll
	if n.Type != "" {
		if perType, ok := sourceControls[n.SourceID]; ok {
			if c, ok := perType[n.Type]; ok {
				controls = c
			}
		}
	}
	n.SupportedControls = controls
}

func (n *NowPlayingMedia) clearProgress() {
	n.CurrentPosition = 0
	n.CurrentPositionUpdated = time.Time{}
	n.Duration = 0
}

// onProgressEvent updates progress from an event, reporting whether the
// update was applied. With allProgressEvents disabled only the first progress
// after each media change is applied.
func (n *NowPlayingMedia) onProgressEvent(m *message, allProgressEvents bool) bool {
	if !allProgressEvents && !n.CurrentPositionUpdated.IsZero() {
		return false
	}
	position, err := m.intValue(attrCurrentPosition)
	if err != nil {
		return false
	}
	duration, err := m.intValue(attrDuration)
	if err != nil {
		return false
	}
	n.CurrentPosition = position
	n.CurrentPositionUpdated = time.Now()
	n.Duration = duration
	return true
}

// PlayMode is the repeat and shuffle configuration of a player.
type PlayMode struct {
	Repeat  RepeatType
	Shuffle bool
}

// Player is a HEOS player. Its state is kept up to date by the client while
// change events are enabled; access it through the getter methods, which are
// safe for concurrent use.
type Player struct {
	client *Client

	mu               sync.RWMutex
	name             string
	playerID         int
	model            string
	serial           string
	version          string
	supportedVersion bool
	ipAddress        string
	network          NetworkType
	lineOut          LineOutLevelType
	control          VolumeControlType
	state            PlayState
	volume           int
	isMuted          bool
	repeat           RepeatType
	shuffle          bool
	playbackError    string
	nowPlaying       NowPlayingMedia
	available        bool
	groupID          int
	hasGroupID       bool
}

func playerFromData(data payloadData, client *Client) *Player {
	p := &Player{client: client, available: true, repeat: RepeatOff}
	p.updateFromData(data)
	return p
}

func (p *Player) updateFromData(data payloadData) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.name = data.str(attrName)
	p.playerID, _ = data.intVal(attrPlayerID)
	p.model = data.str(attrModel)
	p.serial = data.str(attrSerial)
	p.version = data.str(attrVersion)
	p.supportedVersion = isSupportedVersion(p.version)
	p.ipAddress = data.str(attrIPAddress)
	p.network = NetworkType(data.str(attrNetwork))
	if p.network == "" {
		p.network = NetworkUnknown
	}
	lineOut, _ := data.intVal(attrLineOut)
	p.lineOut = LineOutLevelType(lineOut)
	control, _ := data.intVal(attrControl)
	p.control = VolumeControlType(control)
	p.groupID, p.hasGroupID = data.intVal(attrGroupID)
}

// ID returns the player identifier. It can change after a firmware update;
// see PlayerUpdateResult.UpdatedPlayerIDs.
func (p *Player) ID() int { p.mu.RLock(); defer p.mu.RUnlock(); return p.playerID }

// Name returns the player name.
func (p *Player) Name() string { p.mu.RLock(); defer p.mu.RUnlock(); return p.name }

// Model returns the device model.
func (p *Player) Model() string { p.mu.RLock(); defer p.mu.RUnlock(); return p.model }

// Serial returns the device serial number, when known.
func (p *Player) Serial() string { p.mu.RLock(); defer p.mu.RUnlock(); return p.serial }

// Version returns the firmware version.
func (p *Player) Version() string { p.mu.RLock(); defer p.mu.RUnlock(); return p.version }

// SupportedVersion reports whether the firmware is at least TargetVersion.
func (p *Player) SupportedVersion() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.supportedVersion
}

// IPAddress returns the device IP address.
func (p *Player) IPAddress() string { p.mu.RLock(); defer p.mu.RUnlock(); return p.ipAddress }

// Network returns how the device is connected to the network.
func (p *Player) Network() NetworkType { p.mu.RLock(); defer p.mu.RUnlock(); return p.network }

// LineOut returns the line out level type.
func (p *Player) LineOut() LineOutLevelType { p.mu.RLock(); defer p.mu.RUnlock(); return p.lineOut }

// Control returns the volume control type.
func (p *Player) Control() VolumeControlType { p.mu.RLock(); defer p.mu.RUnlock(); return p.control }

// State returns the last known play state.
func (p *Player) State() PlayState { p.mu.RLock(); defer p.mu.RUnlock(); return p.state }

// Volume returns the last known volume level (0-100).
func (p *Player) Volume() int { p.mu.RLock(); defer p.mu.RUnlock(); return p.volume }

// IsMuted returns the last known mute state.
func (p *Player) IsMuted() bool { p.mu.RLock(); defer p.mu.RUnlock(); return p.isMuted }

// Repeat returns the last known repeat mode.
func (p *Player) Repeat() RepeatType { p.mu.RLock(); defer p.mu.RUnlock(); return p.repeat }

// Shuffle returns the last known shuffle state.
func (p *Player) Shuffle() bool { p.mu.RLock(); defer p.mu.RUnlock(); return p.shuffle }

// PlaybackError returns the last playback error reported by the device.
func (p *Player) PlaybackError() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.playbackError
}

// Available reports whether the player is currently present in the system.
func (p *Player) Available() bool { p.mu.RLock(); defer p.mu.RUnlock(); return p.available }

// GroupID returns the id of the group the player belongs to, if any.
func (p *Player) GroupID() (int, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.groupID, p.hasGroupID
}

// NowPlaying returns a snapshot of the current media.
func (p *Player) NowPlaying() NowPlayingMedia {
	p.mu.RLock()
	defer p.mu.RUnlock()
	snapshot := p.nowPlaying
	snapshot.SupportedControls = append([]ControlType(nil), p.nowPlaying.SupportedControls...)
	snapshot.Options = append([]ServiceOption(nil), p.nowPlaying.Options...)
	return snapshot
}

func (p *Player) setAvailable(available bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.available = available
}

// onHeosEvent updates the player from a device event, reporting whether state
// changed (a filtered progress event reports false).
func (p *Player) onHeosEvent(m *message, allProgressEvents bool) bool {
	switch EventCommand(m.command) {
	case EventPlayerNowPlayingProgress:
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.nowPlaying.onProgressEvent(m, allProgressEvents)
	case EventPlayerStateChanged:
		state, err := m.value(attrState)
		if err != nil {
			return false
		}
		p.mu.Lock()
		p.state = PlayState(state)
		if p.state == PlayStatePlay {
			p.nowPlaying.clearProgress()
		}
		p.mu.Unlock()
	case EventPlayerNowPlayingChanged:
		// The event carries no data: issue a follow-up command.
		if refreshErr := p.refreshNowPlaying(context.Background()); refreshErr != nil {
			p.client.opts.logger.Warn("failed to refresh now playing media", "error", refreshErr)
		}
	case EventPlayerVolumeChanged:
		level, err := m.intValue(attrLevel)
		if err != nil {
			return false
		}
		mute, err := m.value(attrMute)
		if err != nil {
			return false
		}
		p.mu.Lock()
		p.volume = level
		p.isMuted = mute == valueOn
		p.mu.Unlock()
	case EventRepeatModeChanged:
		repeat, err := m.value(attrRepeat)
		if err != nil {
			return false
		}
		p.mu.Lock()
		p.repeat = RepeatType(repeat)
		p.mu.Unlock()
	case EventShuffleModeChanged:
		shuffle, err := m.value(attrShuffle)
		if err != nil {
			return false
		}
		p.mu.Lock()
		p.shuffle = shuffle == valueOn
		p.mu.Unlock()
	case EventPlayerPlaybackError:
		playbackError, err := m.value(attrError)
		if err != nil {
			return false
		}
		p.mu.Lock()
		p.playbackError = playbackError
		p.mu.Unlock()
	}
	return true
}

// refresh pulls the player's mutable state (play state, now playing, volume,
// mute and play mode) from the device.
func (p *Player) refresh(ctx context.Context) error {
	playerID := p.ID()
	state, err := p.client.GetPlayState(ctx, playerID)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.state = state
	p.mu.Unlock()
	if npErr := p.refreshNowPlaying(ctx); npErr != nil {
		return npErr
	}
	volume, err := p.client.GetVolume(ctx, playerID)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.volume = volume
	p.mu.Unlock()
	muted, err := p.client.GetMute(ctx, playerID)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.isMuted = muted
	p.mu.Unlock()
	mode, err := p.client.GetPlayMode(ctx, playerID)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.repeat = mode.Repeat
	p.shuffle = mode.Shuffle
	p.mu.Unlock()
	return nil
}

func (p *Player) refreshNowPlaying(ctx context.Context) error {
	resp, err := p.client.conn.command(ctx,
		newCommand(cmdGetNowPlayingMedia).withIntParam(attrPlayerID, p.ID()))
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.nowPlaying.updateFromMessage(resp)
}

// Refresh pulls the player's base information and current state.
func (p *Player) Refresh(ctx context.Context) error {
	resp, err := p.client.conn.command(ctx,
		newCommand(cmdGetPlayerInfo).withIntParam(attrPlayerID, p.ID()))
	if err != nil {
		return err
	}
	data, err := decodePayloadObject(resp.payload)
	if err != nil {
		return err
	}
	p.updateFromData(data)
	return p.refresh(ctx)
}

// Convenience methods delegating to the client.

// Play starts playback.
func (p *Player) Play(ctx context.Context) error { return p.SetState(ctx, PlayStatePlay) }

// Pause pauses playback.
func (p *Player) Pause(ctx context.Context) error { return p.SetState(ctx, PlayStatePause) }

// Stop stops playback.
func (p *Player) Stop(ctx context.Context) error { return p.SetState(ctx, PlayStateStop) }

// SetState sets the play state.
func (p *Player) SetState(ctx context.Context, state PlayState) error {
	return p.client.SetPlayState(ctx, p.ID(), state)
}

// SetVolume sets the volume level (0-100).
func (p *Player) SetVolume(ctx context.Context, level int) error {
	return p.client.SetVolume(ctx, p.ID(), level)
}

// SetMute sets the mute state.
func (p *Player) SetMute(ctx context.Context, mute bool) error {
	return p.client.SetMute(ctx, p.ID(), mute)
}

// Mute mutes the player.
func (p *Player) Mute(ctx context.Context) error { return p.SetMute(ctx, true) }

// Unmute unmutes the player.
func (p *Player) Unmute(ctx context.Context) error { return p.SetMute(ctx, false) }

// ToggleMute toggles the mute state.
func (p *Player) ToggleMute(ctx context.Context) error {
	return p.client.ToggleMute(ctx, p.ID())
}

// VolumeUp raises the volume by step (1-10).
func (p *Player) VolumeUp(ctx context.Context, step int) error {
	return p.client.VolumeUp(ctx, p.ID(), step)
}

// VolumeDown lowers the volume by step (1-10).
func (p *Player) VolumeDown(ctx context.Context, step int) error {
	return p.client.VolumeDown(ctx, p.ID(), step)
}

// SetPlayMode sets the repeat and shuffle modes.
func (p *Player) SetPlayMode(ctx context.Context, repeat RepeatType, shuffle bool) error {
	return p.client.SetPlayMode(ctx, p.ID(), repeat, shuffle)
}

// GetQueue returns the play queue.
func (p *Player) GetQueue(ctx context.Context, opts ...BrowseOption) ([]QueueItem, error) {
	return p.client.GetQueue(ctx, p.ID(), opts...)
}

// PlayQueue plays the given queue item.
func (p *Player) PlayQueue(ctx context.Context, queueID int) error {
	return p.client.PlayQueue(ctx, p.ID(), queueID)
}

// RemoveFromQueue removes the given items from the queue.
func (p *Player) RemoveFromQueue(ctx context.Context, queueIDs []int) error {
	return p.client.RemoveFromQueue(ctx, p.ID(), queueIDs)
}

// ClearQueue clears the queue.
func (p *Player) ClearQueue(ctx context.Context) error {
	return p.client.ClearQueue(ctx, p.ID())
}

// SaveQueue saves the queue as a playlist.
func (p *Player) SaveQueue(ctx context.Context, name string) error {
	return p.client.SaveQueue(ctx, p.ID(), name)
}

// MoveQueueItem moves queue items to a new position.
func (p *Player) MoveQueueItem(ctx context.Context, sourceQueueIDs []int, destinationQueueID int) error {
	return p.client.MoveQueueItem(ctx, p.ID(), sourceQueueIDs, destinationQueueID)
}

// PlayNext plays the next item in the queue.
func (p *Player) PlayNext(ctx context.Context) error { return p.client.PlayNext(ctx, p.ID()) }

// PlayPrevious plays the previous item in the queue.
func (p *Player) PlayPrevious(ctx context.Context) error {
	return p.client.PlayPrevious(ctx, p.ID())
}

// SetQuickSelect assigns the current source to a quick select (1-6).
func (p *Player) SetQuickSelect(ctx context.Context, quickSelectID int) error {
	return p.client.SetQuickSelect(ctx, p.ID(), quickSelectID)
}

// PlayQuickSelect plays a quick select (1-6).
func (p *Player) PlayQuickSelect(ctx context.Context, quickSelectID int) error {
	return p.client.PlayQuickSelect(ctx, p.ID(), quickSelectID)
}

// GetQuickSelects returns the quick selects by id.
func (p *Player) GetQuickSelects(ctx context.Context) (map[int]string, error) {
	return p.client.GetQuickSelects(ctx, p.ID())
}

// CheckUpdate reports whether a firmware update is available.
func (p *Player) CheckUpdate(ctx context.Context) (bool, error) {
	return p.client.CheckUpdate(ctx, p.ID())
}

// Client player commands.

// Players returns the loaded players without contacting the device. The map
// is a copy; the players are shared, live objects.
func (c *Client) Players() map[int]*Player {
	c.mu.RLock()
	defer c.mu.RUnlock()
	players := make(map[int]*Player, len(c.players))
	for id, p := range c.players {
		players[id] = p
	}
	return players
}

// GetPlayers returns the available players, loading them from the device on
// first use. Use LoadPlayers to force a refresh.
func (c *Client) GetPlayers(ctx context.Context) (map[int]*Player, error) {
	c.mu.RLock()
	loaded := c.playersLoaded
	c.mu.RUnlock()
	if !loaded {
		if _, err := c.LoadPlayers(ctx); err != nil {
			return nil, err
		}
	}
	return c.Players(), nil
}

// GetPlayerInfo returns the player with the given id, fetching it from the
// device when not already loaded.
func (c *Client) GetPlayerInfo(ctx context.Context, playerID int) (*Player, error) {
	c.mu.RLock()
	player := c.players[playerID]
	c.mu.RUnlock()
	if player != nil {
		return player, nil
	}
	resp, err := c.conn.command(ctx,
		newCommand(cmdGetPlayerInfo).withIntParam(attrPlayerID, playerID))
	if err != nil {
		return nil, err
	}
	data, err := decodePayloadObject(resp.payload)
	if err != nil {
		return nil, err
	}
	player = playerFromData(data, c)
	if err := player.refresh(ctx); err != nil {
		return nil, err
	}
	return player, nil
}

// LoadPlayers refreshes the players from the device and reports what
// changed. Existing Player objects are updated in place: after a firmware
// update a player is matched by serial, then id, then by name with a changed
// version, so its id may change.
func (c *Client) LoadPlayers(ctx context.Context) (*PlayerUpdateResult, error) {
	result := &PlayerUpdateResult{UpdatedPlayerIDs: map[int]int{}}
	resp, err := c.conn.command(ctx, newCommand(cmdGetPlayers))
	if err != nil {
		return nil, err
	}
	payload, err := decodePayloadList(resp.payload)
	if err != nil {
		return nil, err
	}

	c.mu.RLock()
	existing := make([]*Player, 0, len(c.players))
	for _, p := range c.players {
		existing = append(existing, p)
	}
	c.mu.RUnlock()

	players := make(map[int]*Player, len(payload))
	for _, data := range payload {
		playerID, _ := data.intVal(attrPlayerID)
		name := data.str(attrName)
		version := data.str(attrVersion)
		serial := data.str(attrSerial)
		var match *Player
		for _, p := range existing {
			if (serial != "" && p.Serial() == serial) ||
				p.ID() == playerID ||
				(p.Name() == name && p.Version() != version && version != "") {
				match = p
				break
			}
		}
		if match != nil {
			if match.ID() != playerID {
				result.UpdatedPlayerIDs[match.ID()] = playerID
			}
			match.updateFromData(data)
			match.setAvailable(true)
			players[playerID] = match
			existing = removePlayer(existing, match)
		} else {
			player := playerFromData(data, c)
			result.AddedPlayerIDs = append(result.AddedPlayerIDs, playerID)
			players[playerID] = player
		}
	}
	// Anything left was removed from the system: keep it, marked unavailable.
	for _, p := range existing {
		result.RemovedPlayerIDs = append(result.RemovedPlayerIDs, p.ID())
		p.setAvailable(false)
		players[p.ID()] = p
	}

	for _, p := range players {
		if p.Available() {
			if err := p.refresh(ctx); err != nil {
				return nil, err
			}
		}
	}
	c.mu.Lock()
	c.players = players
	c.playersLoaded = true
	c.mu.Unlock()
	return result, nil
}

func removePlayer(players []*Player, target *Player) []*Player {
	for i, p := range players {
		if p == target {
			return append(players[:i], players[i+1:]...)
		}
	}
	return players
}

// refreshPlayersBaseInfo updates base attributes (including group ids) of the
// loaded players without a full reload.
func (c *Client) refreshPlayersBaseInfo(ctx context.Context) error {
	resp, err := c.conn.command(ctx, newCommand(cmdGetPlayers))
	if err != nil {
		return err
	}
	payload, err := decodePayloadList(resp.payload)
	if err != nil {
		return err
	}
	for _, data := range payload {
		playerID, _ := data.intVal(attrPlayerID)
		c.mu.RLock()
		player := c.players[playerID]
		c.mu.RUnlock()
		if player != nil {
			player.updateFromData(data)
		}
	}
	return nil
}

// GetPlayState returns the play state of a player.
func (c *Client) GetPlayState(ctx context.Context, playerID int) (PlayState, error) {
	resp, err := c.conn.command(ctx,
		newCommand(cmdGetPlayState).withIntParam(attrPlayerID, playerID))
	if err != nil {
		return "", err
	}
	state, err := resp.value(attrState)
	if err != nil {
		return "", err
	}
	return PlayState(state), nil
}

// SetPlayState sets the play state of a player.
func (c *Client) SetPlayState(ctx context.Context, playerID int, state PlayState) error {
	if state == PlayStateUnknown {
		return fmt.Errorf("%w: state cannot be set to 'unknown'", ErrInvalidArgument)
	}
	_, err := c.conn.command(ctx, newCommand(cmdSetPlayState).
		withIntParam(attrPlayerID, playerID).
		withParam(attrState, string(state)))
	return err
}

// GetNowPlayingMedia returns the media currently playing on a player.
func (c *Client) GetNowPlayingMedia(ctx context.Context, playerID int) (*NowPlayingMedia, error) {
	resp, err := c.conn.command(ctx,
		newCommand(cmdGetNowPlayingMedia).withIntParam(attrPlayerID, playerID))
	if err != nil {
		return nil, err
	}
	media := &NowPlayingMedia{}
	if err := media.updateFromMessage(resp); err != nil {
		return nil, err
	}
	return media, nil
}

// GetVolume returns the volume level (0-100) of a player.
func (c *Client) GetVolume(ctx context.Context, playerID int) (int, error) {
	resp, err := c.conn.command(ctx,
		newCommand(cmdGetVolume).withIntParam(attrPlayerID, playerID))
	if err != nil {
		return 0, err
	}
	return resp.intValue(attrLevel)
}

// SetVolume sets the volume level (0-100) of a player.
func (c *Client) SetVolume(ctx context.Context, playerID, level int) error {
	if level < 0 || level > 100 {
		return fmt.Errorf("%w: level must be in the range 0-100", ErrInvalidArgument)
	}
	_, err := c.conn.command(ctx, newCommand(cmdSetVolume).
		withIntParam(attrPlayerID, playerID).
		withIntParam(attrLevel, level))
	return err
}

// VolumeUp raises the volume by step (1-10).
func (c *Client) VolumeUp(ctx context.Context, playerID, step int) error {
	if step < 1 || step > 10 {
		return fmt.Errorf("%w: step must be in the range 1-10", ErrInvalidArgument)
	}
	_, err := c.conn.command(ctx, newCommand(cmdVolumeUp).
		withIntParam(attrPlayerID, playerID).
		withIntParam(attrStep, step))
	return err
}

// VolumeDown lowers the volume by step (1-10).
func (c *Client) VolumeDown(ctx context.Context, playerID, step int) error {
	if step < 1 || step > 10 {
		return fmt.Errorf("%w: step must be in the range 1-10", ErrInvalidArgument)
	}
	_, err := c.conn.command(ctx, newCommand(cmdVolumeDown).
		withIntParam(attrPlayerID, playerID).
		withIntParam(attrStep, step))
	return err
}

// GetMute returns the mute state of a player.
func (c *Client) GetMute(ctx context.Context, playerID int) (bool, error) {
	resp, err := c.conn.command(ctx,
		newCommand(cmdGetMute).withIntParam(attrPlayerID, playerID))
	if err != nil {
		return false, err
	}
	state, err := resp.value(attrState)
	if err != nil {
		return false, err
	}
	return state == valueOn, nil
}

// SetMute sets the mute state of a player.
func (c *Client) SetMute(ctx context.Context, playerID int, mute bool) error {
	_, err := c.conn.command(ctx, newCommand(cmdSetMute).
		withIntParam(attrPlayerID, playerID).
		withParam(attrState, onOff(mute)))
	return err
}

// ToggleMute toggles the mute state of a player.
func (c *Client) ToggleMute(ctx context.Context, playerID int) error {
	_, err := c.conn.command(ctx,
		newCommand(cmdToggleMute).withIntParam(attrPlayerID, playerID))
	return err
}

// GetPlayMode returns the repeat and shuffle modes of a player.
func (c *Client) GetPlayMode(ctx context.Context, playerID int) (PlayMode, error) {
	resp, err := c.conn.command(ctx,
		newCommand(cmdGetPlayMode).withIntParam(attrPlayerID, playerID))
	if err != nil {
		return PlayMode{}, err
	}
	repeat, err := resp.value(attrRepeat)
	if err != nil {
		return PlayMode{}, err
	}
	shuffle, err := resp.value(attrShuffle)
	if err != nil {
		return PlayMode{}, err
	}
	return PlayMode{Repeat: RepeatType(repeat), Shuffle: shuffle == valueOn}, nil
}

// SetPlayMode sets the repeat and shuffle modes of a player.
func (c *Client) SetPlayMode(ctx context.Context, playerID int, repeat RepeatType, shuffle bool) error {
	_, err := c.conn.command(ctx, newCommand(cmdSetPlayMode).
		withIntParam(attrPlayerID, playerID).
		withParam(attrRepeat, string(repeat)).
		withParam(attrShuffle, onOff(shuffle)))
	return err
}

// GetQueue returns the play queue of a player, optionally limited with
// WithRange.
func (c *Client) GetQueue(ctx context.Context, playerID int, opts ...BrowseOption) ([]QueueItem, error) {
	cmd := newCommand(cmdGetQueue).withIntParam(attrPlayerID, playerID)
	settings := browseSettings(opts)
	if settings.hasRange {
		cmd = cmd.withParam(attrRange, settings.rangeParam())
	}
	resp, err := c.conn.command(ctx, cmd)
	if err != nil {
		return nil, err
	}
	payload, err := decodePayloadList(resp.payload)
	if err != nil {
		return nil, err
	}
	items := make([]QueueItem, 0, len(payload))
	for _, data := range payload {
		items = append(items, queueItemFromData(data))
	}
	return items, nil
}

// PlayQueue plays the given queue item on a player.
func (c *Client) PlayQueue(ctx context.Context, playerID, queueID int) error {
	_, err := c.conn.command(ctx, newCommand(cmdPlayQueue).
		withIntParam(attrPlayerID, playerID).
		withIntParam(attrQueueID, queueID))
	return err
}

// RemoveFromQueue removes items from the queue of a player.
func (c *Client) RemoveFromQueue(ctx context.Context, playerID int, queueIDs []int) error {
	_, err := c.conn.command(ctx, newCommand(cmdRemoveFromQueue).
		withIntParam(attrPlayerID, playerID).
		withParam(attrQueueID, joinInts(queueIDs)))
	return err
}

// SaveQueue saves the queue of a player as a playlist.
func (c *Client) SaveQueue(ctx context.Context, playerID int, name string) error {
	if len(name) > 128 {
		return fmt.Errorf("%w: name must be less than or equal to 128 characters", ErrInvalidArgument)
	}
	_, err := c.conn.command(ctx, newCommand(cmdSaveQueue).
		withIntParam(attrPlayerID, playerID).
		withParam(attrName, name))
	return err
}

// ClearQueue clears the queue of a player.
func (c *Client) ClearQueue(ctx context.Context, playerID int) error {
	_, err := c.conn.command(ctx,
		newCommand(cmdClearQueue).withIntParam(attrPlayerID, playerID))
	return err
}

// MoveQueueItem moves queue items to a new position in the queue.
func (c *Client) MoveQueueItem(ctx context.Context, playerID int, sourceQueueIDs []int, destinationQueueID int) error {
	_, err := c.conn.command(ctx, newCommand(cmdMoveQueueItem).
		withIntParam(attrPlayerID, playerID).
		withParam(attrSourceQueueID, joinInts(sourceQueueIDs)).
		withIntParam(attrDestinationQueueID, destinationQueueID))
	return err
}

// PlayNext plays the next item in the queue of a player.
func (c *Client) PlayNext(ctx context.Context, playerID int) error {
	_, err := c.conn.command(ctx,
		newCommand(cmdPlayNext).withIntParam(attrPlayerID, playerID))
	return err
}

// PlayPrevious plays the previous item in the queue of a player.
func (c *Client) PlayPrevious(ctx context.Context, playerID int) error {
	_, err := c.conn.command(ctx,
		newCommand(cmdPlayPrevious).withIntParam(attrPlayerID, playerID))
	return err
}

// SetQuickSelect assigns the current source to a quick select (1-6).
func (c *Client) SetQuickSelect(ctx context.Context, playerID, quickSelectID int) error {
	if quickSelectID < 1 || quickSelectID > 6 {
		return fmt.Errorf("%w: quick select id must be in the range 1-6", ErrInvalidArgument)
	}
	_, err := c.conn.command(ctx, newCommand(cmdSetQuickSelect).
		withIntParam(attrPlayerID, playerID).
		withIntParam(attrID, quickSelectID))
	return err
}

// PlayQuickSelect plays a quick select (1-6).
func (c *Client) PlayQuickSelect(ctx context.Context, playerID, quickSelectID int) error {
	if quickSelectID < 1 || quickSelectID > 6 {
		return fmt.Errorf("%w: quick select id must be in the range 1-6", ErrInvalidArgument)
	}
	_, err := c.conn.command(ctx, newCommand(cmdPlayQuickSelect).
		withIntParam(attrPlayerID, playerID).
		withIntParam(attrID, quickSelectID))
	return err
}

// GetQuickSelects returns the quick select names by id.
func (c *Client) GetQuickSelects(ctx context.Context, playerID int) (map[int]string, error) {
	resp, err := c.conn.command(ctx,
		newCommand(cmdGetQuickSelects).withIntParam(attrPlayerID, playerID))
	if err != nil {
		return nil, err
	}
	payload, err := decodePayloadList(resp.payload)
	if err != nil {
		return nil, err
	}
	selects := make(map[int]string, len(payload))
	for _, data := range payload {
		id, _ := data.intVal(attrID)
		selects[id] = data.str(attrName)
	}
	return selects, nil
}

// CheckUpdate reports whether a firmware update is available for a player.
func (c *Client) CheckUpdate(ctx context.Context, playerID int) (bool, error) {
	resp, err := c.conn.command(ctx,
		newCommand(cmdCheckUpdate).withIntParam(attrPlayerID, playerID))
	if err != nil {
		return false, err
	}
	data, err := decodePayloadObject(resp.payload)
	if err != nil {
		return false, err
	}
	return data.str(attrUpdate) == valueUpdateExist, nil
}

func joinInts(values []int) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, ",")
}
