// Package mediauri serializes MediaItem and MusicSource values to and from
// heos://media URI strings, interoperable with URIs produced by the Python
// pyheos library. The URIs are for external persistence (e.g. storing a
// favorite in a config file); they are not part of the CLI protocol.
package mediauri

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/ezeql/goheos"
)

// BaseURI is the prefix of every media URI.
const BaseURI = "heos://media"

// IsMediaURI reports whether the URI is a HEOS media URI.
func IsMediaURI(uri string) bool {
	return strings.HasPrefix(uri, BaseURI)
}

// Result is a parsed media URI: exactly one of MediaItem or MusicSource is
// non-nil. Extra holds the raw JSON of the extra payload, when present.
type Result struct {
	MediaItem   *goheos.MediaItem
	MusicSource *goheos.MusicSource
	Extra       json.RawMessage
}

// pyBool renders a boolean the way Python's str() does, keeping URIs
// byte-compatible with pyheos.
func pyBool(v bool) string {
	if v {
		return "True"
	}
	return "False"
}

// encoder builds a query string in insertion order with Python
// urlencode-compatible escaping (space becomes '+').
type encoder struct {
	pairs []string
}

func (e *encoder) add(key, value string) {
	e.pairs = append(e.pairs, key+"="+url.QueryEscape(value))
}

func (e *encoder) addNonEmpty(key, value string) {
	if value != "" {
		e.add(key, value)
	}
}

func (e *encoder) encode() string { return strings.Join(e.pairs, "&") }

func marshalExtra(e *encoder, extra any) error {
	if extra == nil {
		return nil
	}
	raw, err := json.Marshal(extra)
	if err != nil {
		return fmt.Errorf("mediauri: marshal extra: %w", err)
	}
	e.add("extra", string(raw))
	return nil
}

// FromMediaItem serializes a media item, with an optional extra payload
// (marshaled as JSON), to a URI string.
func FromMediaItem(media *goheos.MediaItem, extra any) (string, error) {
	e := &encoder{}
	e.add("name", media.Name)
	e.add("image_url", media.ImageURL)
	e.add("playable", pyBool(media.Playable))
	e.add("browsable", pyBool(media.Browsable))
	e.addNonEmpty("container_id", media.ContainerID)
	e.addNonEmpty("media_id", media.MediaID)
	e.addNonEmpty("artist", media.Artist)
	e.addNonEmpty("album", media.Album)
	e.addNonEmpty("album_id", media.AlbumID)
	if err := marshalExtra(e, extra); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%d/%s?%s", BaseURI, media.SourceID, media.Type, e.encode()), nil
}

// FromMusicSource serializes a music source, with an optional extra payload
// (marshaled as JSON), to a URI string.
func FromMusicSource(source goheos.MusicSource, extra any) (string, error) {
	e := &encoder{}
	e.add("name", source.Name)
	e.add("image_url", source.ImageURL)
	e.add("available", pyBool(source.Available))
	e.addNonEmpty("service_username", source.ServiceUsername)
	if err := marshalExtra(e, extra); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%d/%s?%s", BaseURI, source.SourceID, source.Type, e.encode()), nil
}

// Parse builds a MediaItem or MusicSource back from a media URI.
func Parse(uri string) (*Result, error) {
	if !IsMediaURI(uri) {
		return nil, errors.New("mediauri: uri is not a HEOS media URI")
	}
	parsed, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("mediauri: %w", err)
	}
	// heos://media/<source_id>/<type> parses with host "media" and path
	// "/<source_id>/<type>".
	path := strings.SplitN(strings.TrimPrefix(parsed.Path, "/"), "/", 3)
	if len(path) < 2 {
		return nil, errors.New("mediauri: uri is missing source id or type")
	}
	sourceID, err := strconv.Atoi(path[0])
	if err != nil {
		return nil, fmt.Errorf("mediauri: invalid source id: %w", err)
	}
	mediaType := goheos.MediaType(path[1])
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return nil, fmt.Errorf("mediauri: %w", err)
	}

	result := &Result{}
	if query.Has("extra") {
		result.Extra = json.RawMessage(query.Get("extra"))
	}
	if query.Has("playable") {
		result.MediaItem = &goheos.MediaItem{
			SourceID:    sourceID,
			Type:        mediaType,
			Name:        query.Get("name"),
			ImageURL:    query.Get("image_url"),
			Playable:    query.Get("playable") == "True",
			Browsable:   query.Get("browsable") == "True",
			ContainerID: query.Get("container_id"),
			MediaID:     query.Get("media_id"),
			Artist:      query.Get("artist"),
			Album:       query.Get("album"),
			AlbumID:     query.Get("album_id"),
		}
		return result, nil
	}
	result.MusicSource = &goheos.MusicSource{
		SourceID:        sourceID,
		Type:            mediaType,
		Name:            query.Get("name"),
		ImageURL:        query.Get("image_url"),
		Available:       query.Get("available") == "True",
		ServiceUsername: query.Get("service_username"),
	}
	return result, nil
}
