package goheos

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// QueueItem is an item in a player's queue.
type QueueItem struct {
	QueueID  int
	Song     string
	Album    string
	Artist   string
	ImageURL string
	MediaID  string
	AlbumID  string
}

func queueItemFromData(data payloadData) QueueItem {
	queueID, _ := data.intVal(attrQueueID)
	return QueueItem{
		QueueID:  queueID,
		Song:     data.str(attrSong),
		Album:    data.str(attrAlbum),
		Artist:   data.str(attrArtist),
		ImageURL: data.str(attrImageURL),
		MediaID:  data.str(attrMediaID),
		AlbumID:  data.str(attrAlbumID),
	}
}

// MusicSource is a top-level music source returned by GetMusicSources.
type MusicSource struct {
	SourceID        int
	Name            string
	Type            MediaType
	ImageURL        string
	Available       bool
	ServiceUsername string
}

func musicSourceFromData(data payloadData) MusicSource {
	sourceID, _ := data.intVal(attrSourceID)
	return MusicSource{
		SourceID:        sourceID,
		Name:            data.str(attrName),
		Type:            MediaType(data.str(attrType)),
		ImageURL:        data.str(attrImageURL),
		Available:       data.str(attrAvailable) == valueTrue,
		ServiceUsername: data.str(attrServiceUserName),
	}
}

// MediaItem is a browsable and/or playable media item.
type MediaItem struct {
	SourceID int
	Name     string
	Type     MediaType
	ImageURL string
	Playable bool
	// Browsable is inferred: an item is browsable when it is itself a source
	// or a container.
	Browsable   bool
	ContainerID string
	MediaID     string
	Artist      string
	Album       string
	AlbumID     string
}

// mediaItemFromData builds a MediaItem, filling source and container ids from
// the browse context when the payload does not carry them.
func mediaItemFromData(data payloadData, sourceID int, hasSourceID bool, containerID string) (*MediaItem, error) {
	itemSourceID, ok := data.intVal(attrSourceID)
	if !ok {
		if !hasSourceID {
			return nil, fmt.Errorf("%w: 'source_id' is required when not present in data", ErrInvalidArgument)
		}
		itemSourceID = sourceID
	}
	itemContainerID := containerID
	if data.has(attrContainerID) {
		itemContainerID = data.str(attrContainerID)
	}
	return &MediaItem{
		SourceID:    itemSourceID,
		Name:        unquoteName(data.str(attrName)),
		Type:        MediaType(data.str(attrType)),
		ImageURL:    data.str(attrImageURL),
		Playable:    data.str(attrPlayable) == valueYes,
		Browsable:   data.has(attrSourceID) || data.str(attrContainer) == valueYes,
		ContainerID: itemContainerID,
		MediaID:     data.str(attrMediaID),
		Artist:      data.str(attrArtist),
		Album:       data.str(attrAlbum),
		AlbumID:     data.str(attrAlbumID),
	}, nil
}

// unquoteName percent-decodes an item name, preserving it verbatim when the
// escaping is malformed. '+' is not treated as a space, matching Python's
// unquote.
func unquoteName(name string) string {
	if u, err := url.PathUnescape(name); err == nil {
		return u
	}
	return name
}

// ServiceOption is an option supported by the current media or source, e.g.
// "Add to HEOS Favorites".
type ServiceOption struct {
	Context string
	ID      int
	Name    string
}

// parseServiceOptions flattens the nested options payload:
// [{"play": [{"id": 19, "name": "..."}]}].
func parseServiceOptions(raw json.RawMessage) ([]ServiceOption, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var contexts []map[string][]payloadData
	if err := json.Unmarshal(raw, &contexts); err != nil {
		return nil, fmt.Errorf("goheos: decode options: %w", err)
	}
	var options []ServiceOption
	for _, context := range contexts {
		for contextKey, items := range context {
			for _, item := range items {
				id, _ := item.intVal(attrID)
				options = append(options, ServiceOption{
					Context: contextKey,
					ID:      id,
					Name:    item.str(attrName),
				})
			}
		}
	}
	return options, nil
}

// BrowseResult is the result of a browse operation.
type BrowseResult struct {
	Count       int
	Returned    int
	SourceID    int
	ContainerID string
	Items       []*MediaItem
	Options     []ServiceOption
}

func browseResultFromMessage(m *message) (*BrowseResult, error) {
	sourceID, err := m.intValue(attrSourceID)
	if err != nil {
		return nil, err
	}
	count, err := m.intValue(attrCount)
	if err != nil {
		return nil, err
	}
	returned, err := m.intValue(attrReturned)
	if err != nil {
		return nil, err
	}
	containerID := m.message[attrContainerID]
	list, err := decodePayloadList(m.payload)
	if err != nil {
		return nil, err
	}
	items := make([]*MediaItem, 0, len(list))
	for _, data := range list {
		item, itemErr := mediaItemFromData(data, sourceID, true, containerID)
		if itemErr != nil {
			return nil, itemErr
		}
		items = append(items, item)
	}
	options, err := parseServiceOptions(m.options)
	if err != nil {
		return nil, err
	}
	return &BrowseResult{
		Count:       count,
		Returned:    returned,
		SourceID:    sourceID,
		ContainerID: containerID,
		Items:       items,
		Options:     options,
	}, nil
}

// ImageMetadata is metadata about an album image.
type ImageMetadata struct {
	ImageURL string
	Width    int
}

// AlbumMetadata is metadata about an album, including its images.
type AlbumMetadata struct {
	AlbumID string
	Images  []ImageMetadata
}

// RetrieveMetadataResult is the result of a retrieve metadata operation.
type RetrieveMetadataResult struct {
	SourceID    int
	ContainerID string
	Returned    int
	Count       int
	Metadata    []AlbumMetadata
}

func retrieveMetadataResultFromMessage(m *message) (*RetrieveMetadataResult, error) {
	sourceID, err := m.intValue(attrSourceID)
	if err != nil {
		return nil, err
	}
	containerID, err := m.value(attrContainerID)
	if err != nil {
		return nil, err
	}
	returned, err := m.intValue(attrReturned)
	if err != nil {
		return nil, err
	}
	count, err := m.intValue(attrCount)
	if err != nil {
		return nil, err
	}
	list, err := decodePayloadList(m.payload)
	if err != nil {
		return nil, err
	}
	metadata := make([]AlbumMetadata, 0, len(list))
	for _, data := range list {
		album := AlbumMetadata{AlbumID: data.str(attrAlbumID)}
		if images, ok := data[attrImages].([]any); ok {
			for _, img := range images {
				imageData, ok := img.(map[string]any)
				if !ok {
					continue
				}
				width, _ := payloadData(imageData).intVal(attrWidth)
				album.Images = append(album.Images, ImageMetadata{
					ImageURL: payloadData(imageData).str(attrImageURL),
					Width:    width,
				})
			}
		}
		metadata = append(metadata, album)
	}
	return &RetrieveMetadataResult{
		SourceID:    sourceID,
		ContainerID: containerID,
		Returned:    returned,
		Count:       count,
		Metadata:    metadata,
	}, nil
}
