package goheos

import (
	"context"
	"fmt"
	"strconv"
)

type browseOpts struct {
	containerID  string
	hasContainer bool
	rangeStart   int
	rangeEnd     int
	hasRange     bool
}

// BrowseOption refines a browse, search or queue request.
type BrowseOption func(*browseOpts)

// WithContainer browses inside the given container instead of the source
// root.
func WithContainer(containerID string) BrowseOption {
	return func(o *browseOpts) {
		o.containerID = containerID
		o.hasContainer = true
	}
}

// WithRange limits the response to the items between start and end
// (inclusive, 0-based).
func WithRange(start, end int) BrowseOption {
	return func(o *browseOpts) {
		o.rangeStart = start
		o.rangeEnd = end
		o.hasRange = true
	}
}

func browseSettings(opts []BrowseOption) browseOpts {
	var o browseOpts
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

func (o browseOpts) rangeParam() string {
	return strconv.Itoa(o.rangeStart) + "," + strconv.Itoa(o.rangeEnd)
}

// MusicSources returns the loaded music sources without contacting the
// device. The map is a copy.
func (c *Client) MusicSources() map[int]MusicSource {
	c.mu.RLock()
	defer c.mu.RUnlock()
	sources := make(map[int]MusicSource, len(c.musicSources))
	for id, s := range c.musicSources {
		sources[id] = s
	}
	return sources
}

// GetMusicSources returns the available music sources, loading them from the
// device on first use. Use RefreshMusicSources to force a refresh.
func (c *Client) GetMusicSources(ctx context.Context) (map[int]MusicSource, error) {
	c.mu.RLock()
	loaded := c.musicSourcesLoaded
	c.mu.RUnlock()
	if !loaded {
		if err := c.loadMusicSources(ctx, false); err != nil {
			return nil, err
		}
	}
	return c.MusicSources(), nil
}

// RefreshMusicSources reloads the music sources, asking the device to refresh
// its own list.
func (c *Client) RefreshMusicSources(ctx context.Context) error {
	return c.loadMusicSources(ctx, true)
}

func (c *Client) loadMusicSources(ctx context.Context, refresh bool) error {
	cmd := newCommand(cmdBrowseGetSources)
	if refresh {
		cmd = cmd.withParam(attrRefresh, valueOn)
	}
	resp, err := c.conn.command(ctx, cmd)
	if err != nil {
		return err
	}
	payload, err := decodePayloadList(resp.payload)
	if err != nil {
		return err
	}
	sources := make(map[int]MusicSource, len(payload))
	for _, data := range payload {
		source := musicSourceFromData(data)
		sources[source.SourceID] = source
	}
	c.mu.Lock()
	c.musicSources = sources
	c.musicSourcesLoaded = true
	c.mu.Unlock()
	return nil
}

// GetMusicSourceInfo returns information about a music source, using the
// loaded sources when available.
func (c *Client) GetMusicSourceInfo(ctx context.Context, sourceID int) (MusicSource, error) {
	c.mu.RLock()
	source, ok := c.musicSources[sourceID]
	c.mu.RUnlock()
	if ok {
		return source, nil
	}
	resp, err := c.conn.command(ctx,
		newCommand(cmdBrowseGetSourceInfo).withIntParam(attrSourceID, sourceID))
	if err != nil {
		return MusicSource{}, err
	}
	data, err := decodePayloadObject(resp.payload)
	if err != nil {
		return MusicSource{}, err
	}
	return musicSourceFromData(data), nil
}

// Browse lists the contents of a source, or of a container inside it with
// WithContainer. Use WithRange to page through large containers.
func (c *Client) Browse(ctx context.Context, sourceID int, opts ...BrowseOption) (*BrowseResult, error) {
	settings := browseSettings(opts)
	cmd := newCommand(cmdBrowseBrowse).withIntParam(attrSourceID, sourceID)
	if settings.hasContainer && settings.containerID != "" {
		cmd = cmd.withParam(attrContainerID, settings.containerID)
	}
	if settings.hasRange {
		cmd = cmd.withParam(attrRange, settings.rangeParam())
	}
	resp, err := c.conn.command(ctx, cmd)
	if err != nil {
		return nil, err
	}
	return browseResultFromMessage(resp)
}

// BrowseMedia browses inside a browsable media item (a source or container).
func (c *Client) BrowseMedia(ctx context.Context, media *MediaItem, opts ...BrowseOption) (*BrowseResult, error) {
	if !media.Browsable {
		return nil, fmt.Errorf("%w: only media sources and containers can be browsed", ErrInvalidArgument)
	}
	if media.ContainerID != "" {
		opts = append(opts, WithContainer(media.ContainerID))
	}
	return c.Browse(ctx, media.SourceID, opts...)
}

// GetSearchCriteria returns the supported search criteria of a music source.
func (c *Client) GetSearchCriteria(ctx context.Context, sourceID int) ([]SearchCriteria, error) {
	resp, err := c.conn.command(ctx,
		newCommand(cmdBrowseGetSearchCriteria).withIntParam(attrSourceID, sourceID))
	if err != nil {
		return nil, err
	}
	payload, err := decodePayloadList(resp.payload)
	if err != nil {
		return nil, err
	}
	criteria := make([]SearchCriteria, 0, len(payload))
	for _, data := range payload {
		criteria = append(criteria, searchCriteriaFromData(data))
	}
	return criteria, nil
}

// Search searches a music source with the given criteria.
func (c *Client) Search(ctx context.Context, sourceID int, search string, criteriaID int, opts ...BrowseOption) (*SearchResult, error) {
	if search == "" {
		return nil, fmt.Errorf("%w: search must not be empty", ErrInvalidArgument)
	}
	if len(search) > 128 {
		return nil, fmt.Errorf("%w: search must be less than or equal to 128 characters", ErrInvalidArgument)
	}
	settings := browseSettings(opts)
	cmd := newCommand(cmdBrowseSearch).
		withIntParam(attrSourceID, sourceID).
		withParam(attrSearch, search).
		withIntParam(attrSearchCriteriaID, criteriaID)
	if settings.hasRange {
		cmd = cmd.withParam(attrRange, settings.rangeParam())
	}
	resp, err := c.conn.command(ctx, cmd)
	if err != nil {
		return nil, err
	}
	return searchResultFromMessage(resp)
}

// MultiSearch searches across several music sources at once. Nil sourceIDs or
// criteriaIDs search everything.
func (c *Client) MultiSearch(ctx context.Context, search string, sourceIDs, criteriaIDs []int) (*MultiSearchResult, error) {
	if len(search) > 128 {
		return nil, fmt.Errorf("%w: search must be less than or equal to 128 characters", ErrInvalidArgument)
	}
	cmd := newCommand(cmdBrowseMultiSearch).withParam(attrSearch, search)
	if sourceIDs != nil {
		cmd = cmd.withParam(attrSourceID, joinInts(sourceIDs))
	}
	if criteriaIDs != nil {
		cmd = cmd.withParam(attrSearchCriteriaID, joinInts(criteriaIDs))
	}
	resp, err := c.conn.command(ctx, cmd)
	if err != nil {
		return nil, err
	}
	return multiSearchResultFromMessage(resp)
}

// PlayInputSource plays an input source (one of the Input* constants) on a
// player. Pass a non-zero sourcePlayerID when the input belongs to a
// different player.
func (c *Client) PlayInputSource(ctx context.Context, playerID int, inputName string, sourcePlayerID int) error {
	cmd := newCommand(cmdBrowsePlayInput).
		withIntParam(attrPlayerID, playerID).
		withParam(attrInput, inputName)
	if sourcePlayerID != 0 {
		cmd = cmd.withIntParam(attrSourcePlayerID, sourcePlayerID)
	}
	_, err := c.conn.command(ctx, cmd)
	return err
}

// PlayStation plays a station on a player. containerID may be empty.
func (c *Client) PlayStation(ctx context.Context, playerID, sourceID int, containerID, mediaID string) error {
	cmd := newCommand(cmdBrowsePlayStream).
		withIntParam(attrPlayerID, playerID).
		withIntParam(attrSourceID, sourceID).
		withParam(attrMediaID, mediaID)
	if containerID != "" {
		cmd = cmd.withParam(attrContainerID, containerID)
	}
	_, err := c.conn.command(ctx, cmd)
	return err
}

// PlayPresetStation plays a favorite by its 1-based index.
func (c *Client) PlayPresetStation(ctx context.Context, playerID, index int) error {
	if index < 1 {
		return fmt.Errorf("%w: invalid preset %d", ErrInvalidArgument, index)
	}
	_, err := c.conn.command(ctx, newCommand(cmdBrowsePlayPreset).
		withIntParam(attrPlayerID, playerID).
		withIntParam(attrPreset, index))
	return err
}

// PlayURL plays a URL on a player.
func (c *Client) PlayURL(ctx context.Context, playerID int, playURL string) error {
	_, err := c.conn.command(ctx, newCommand(cmdBrowsePlayStream).
		withIntParam(attrPlayerID, playerID).
		withParam(attrURL, playURL))
	return err
}

// AddToQueue adds a container or track to a player's queue. mediaID is
// required for songs and may be empty for containers.
func (c *Client) AddToQueue(ctx context.Context, playerID, sourceID int, containerID, mediaID string, addCriteria AddCriteriaType) error {
	cmd := newCommand(cmdBrowseAddToQueue).
		withIntParam(attrPlayerID, playerID).
		withIntParam(attrSourceID, sourceID).
		withParam(attrContainerID, containerID).
		withIntParam(attrAddCriteriaID, int(addCriteria))
	if mediaID != "" {
		cmd = cmd.withParam(attrMediaID, mediaID)
	}
	_, err := c.conn.command(ctx, cmd)
	return err
}

// AddSearchToQueue adds all tracks matching a search to a player's queue,
// using the SEARCHED_TRACKS- container convention.
func (c *Client) AddSearchToQueue(ctx context.Context, playerID, sourceID int, search string, addCriteria AddCriteriaType) error {
	return c.AddToQueue(ctx, playerID, sourceID, SearchedTracksPrefix+search, "", addCriteria)
}

// RenamePlaylist renames a HEOS playlist.
func (c *Client) RenamePlaylist(ctx context.Context, sourceID int, containerID, newName string) error {
	if newName == "" {
		return fmt.Errorf("%w: new name must not be empty", ErrInvalidArgument)
	}
	if len(newName) > 128 {
		return fmt.Errorf("%w: new name must be less than or equal to 128 characters", ErrInvalidArgument)
	}
	_, err := c.conn.command(ctx, newCommand(cmdBrowseRenamePlaylist).
		withIntParam(attrSourceID, sourceID).
		withParam(attrContainerID, containerID).
		withParam(attrName, newName))
	return err
}

// DeletePlaylist deletes a HEOS playlist.
func (c *Client) DeletePlaylist(ctx context.Context, sourceID int, containerID string) error {
	_, err := c.conn.command(ctx, newCommand(cmdBrowseDeletePlaylist).
		withIntParam(attrSourceID, sourceID).
		withParam(attrContainerID, containerID))
	return err
}

// RetrieveMetadata retrieves album metadata. Only supported by the
// Rhapsody/Napster music sources.
func (c *Client) RetrieveMetadata(ctx context.Context, sourceID int, containerID string) (*RetrieveMetadataResult, error) {
	resp, err := c.conn.command(ctx, newCommand(cmdBrowseRetrieveMetadata).
		withIntParam(attrSourceID, sourceID).
		withParam(attrContainerID, containerID))
	if err != nil {
		return nil, err
	}
	return retrieveMetadataResultFromMessage(resp)
}

// PlayMedia plays a playable media item on a player, dispatching to the
// appropriate command: input sources play directly, stations stream, and
// songs or containers are added to the queue.
func (c *Client) PlayMedia(ctx context.Context, playerID int, media *MediaItem, addCriteria AddCriteriaType) error {
	if !media.Playable {
		return fmt.Errorf("%w: media %q is not playable", ErrInvalidArgument, media.Name)
	}
	switch {
	case validInputs[media.MediaID]:
		return c.PlayInputSource(ctx, playerID, media.MediaID, media.SourceID)
	case media.Type == MediaTypeStation:
		if media.MediaID == "" {
			return fmt.Errorf("%w: media %q cannot have an empty media id", ErrInvalidArgument, media.Name)
		}
		return c.PlayStation(ctx, playerID, media.SourceID, media.ContainerID, media.MediaID)
	default:
		// Handles both songs and containers.
		if media.ContainerID == "" {
			return fmt.Errorf("%w: media %q cannot have an empty container id", ErrInvalidArgument, media.Name)
		}
		return c.AddToQueue(ctx, playerID, media.SourceID, media.ContainerID, media.MediaID, addCriteria)
	}
}

// GetInputSources browses every aux input device and returns all available
// input sources. Note this issues one browse per device.
func (c *Client) GetInputSources(ctx context.Context) ([]*MediaItem, error) {
	result, err := c.Browse(ctx, MusicSourceAuxInput)
	if err != nil {
		return nil, err
	}
	var inputs []*MediaItem
	for _, item := range result.Items {
		deviceResult, err := c.BrowseMedia(ctx, item)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, deviceResult.Items...)
	}
	return inputs, nil
}

// GetFavorites returns the favorite stations keyed by their 1-based index,
// which is the value to pass to PlayPresetStation.
func (c *Client) GetFavorites(ctx context.Context) (map[int]*MediaItem, error) {
	result, err := c.Browse(ctx, MusicSourceFavorites)
	if err != nil {
		return nil, err
	}
	favorites := make(map[int]*MediaItem, len(result.Items))
	for i, item := range result.Items {
		favorites[i+1] = item
	}
	return favorites, nil
}

// GetPlaylists returns the HEOS playlists.
func (c *Client) GetPlaylists(ctx context.Context) ([]*MediaItem, error) {
	result, err := c.Browse(ctx, MusicSourcePlaylists)
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}

// Player browse conveniences.

// PlayMedia plays a playable media item on this player.
func (p *Player) PlayMedia(ctx context.Context, media *MediaItem, addCriteria AddCriteriaType) error {
	return p.client.PlayMedia(ctx, p.ID(), media, addCriteria)
}

// PlayURL plays a URL on this player.
func (p *Player) PlayURL(ctx context.Context, playURL string) error {
	return p.client.PlayURL(ctx, p.ID(), playURL)
}

// PlayInputSource plays an input source on this player.
func (p *Player) PlayInputSource(ctx context.Context, inputName string, sourcePlayerID int) error {
	return p.client.PlayInputSource(ctx, p.ID(), inputName, sourcePlayerID)
}

// PlayPresetStation plays a favorite by its 1-based index on this player.
func (p *Player) PlayPresetStation(ctx context.Context, index int) error {
	return p.client.PlayPresetStation(ctx, p.ID(), index)
}

// AddToQueue adds a container or track to this player's queue.
func (p *Player) AddToQueue(ctx context.Context, sourceID int, containerID, mediaID string, addCriteria AddCriteriaType) error {
	return p.client.AddToQueue(ctx, p.ID(), sourceID, containerID, mediaID, addCriteria)
}

// AddSearchToQueue adds all tracks matching a search to this player's queue.
func (p *Player) AddSearchToQueue(ctx context.Context, sourceID int, search string, addCriteria AddCriteriaType) error {
	return p.client.AddSearchToQueue(ctx, p.ID(), sourceID, search, addCriteria)
}
