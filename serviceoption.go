package goheos

import (
	"context"
	"fmt"
	"strings"
)

type serviceOptionParams struct {
	sourceID       int
	hasSourceID    bool
	containerID    string
	hasContainerID bool
	mediaID        string
	hasMediaID     bool
	playerID       int
	hasPlayerID    bool
	name           string
	hasName        bool
	criteriaID     int
	hasCriteriaID  bool
	rangeStart     int
	rangeEnd       int
	hasRange       bool
}

// ServiceOptionParam supplies a parameter to SetServiceOption. Each service
// option requires a specific combination of parameters; the rest are
// rejected.
type ServiceOptionParam func(*serviceOptionParams)

// OptionSourceID supplies the source id parameter.
func OptionSourceID(sourceID int) ServiceOptionParam {
	return func(p *serviceOptionParams) { p.sourceID, p.hasSourceID = sourceID, true }
}

// OptionContainerID supplies the container id parameter.
func OptionContainerID(containerID string) ServiceOptionParam {
	return func(p *serviceOptionParams) { p.containerID, p.hasContainerID = containerID, true }
}

// OptionMediaID supplies the media id parameter.
func OptionMediaID(mediaID string) ServiceOptionParam {
	return func(p *serviceOptionParams) { p.mediaID, p.hasMediaID = mediaID, true }
}

// OptionPlayerID supplies the player id parameter.
func OptionPlayerID(playerID int) ServiceOptionParam {
	return func(p *serviceOptionParams) { p.playerID, p.hasPlayerID = playerID, true }
}

// OptionName supplies the name parameter.
func OptionName(name string) ServiceOptionParam {
	return func(p *serviceOptionParams) { p.name, p.hasName = name, true }
}

// OptionCriteriaID supplies the search criteria id parameter.
func OptionCriteriaID(criteriaID int) ServiceOptionParam {
	return func(p *serviceOptionParams) { p.criteriaID, p.hasCriteriaID = criteriaID, true }
}

// OptionRange supplies the range parameter (inclusive, 0-based).
func OptionRange(start, end int) ServiceOptionParam {
	return func(p *serviceOptionParams) {
		p.rangeStart, p.rangeEnd, p.hasRange = start, end, true
	}
}

// SetServiceOption invokes a service option (add/remove from library or
// favorites, thumbs up/down, create station). Each option requires a specific
// set of parameters, validated client-side like pyheos.
func (c *Client) SetServiceOption(ctx context.Context, optionID ServiceOptionID, params ...ServiceOptionParam) error {
	var p serviceOptionParams
	for _, param := range params {
		param(&p)
	}
	cmd := newCommand(cmdBrowseSetServiceOption).withIntParam(attrOptionID, int(optionID))

	type disallowed struct {
		name    string
		present bool
	}
	var disallowedParams []disallowed

	switch optionID {
	case ServiceOptionAddTrackToLibrary,
		ServiceOptionAddStationToLibrary,
		ServiceOptionRemoveTrackFromLibrary,
		ServiceOptionRemoveStationFromLibrary:
		if !p.hasSourceID || !p.hasMediaID {
			return fmt.Errorf("%w: source id and media id parameters are required for service option %d", ErrInvalidArgument, optionID)
		}
		disallowedParams = []disallowed{
			{"container_id", p.hasContainerID},
			{"player_id", p.hasPlayerID},
			{"name", p.hasName},
			{"criteria_id", p.hasCriteriaID},
			{"range", p.hasRange},
		}
		cmd = cmd.withIntParam(attrSourceID, p.sourceID).withParam(attrMediaID, p.mediaID)

	case ServiceOptionAddAlbumToLibrary,
		ServiceOptionRemoveAlbumFromLibrary,
		ServiceOptionRemovePlaylistFromLibrary:
		if !p.hasSourceID || !p.hasContainerID {
			return fmt.Errorf("%w: source id and container id parameters are required for service option %d", ErrInvalidArgument, optionID)
		}
		disallowedParams = []disallowed{
			{"media_id", p.hasMediaID},
			{"player_id", p.hasPlayerID},
			{"name", p.hasName},
			{"criteria_id", p.hasCriteriaID},
			{"range", p.hasRange},
		}
		cmd = cmd.withIntParam(attrSourceID, p.sourceID).withParam(attrContainerID, p.containerID)

	case ServiceOptionAddPlaylistToLibrary:
		if !p.hasSourceID || !p.hasContainerID || !p.hasName {
			return fmt.Errorf("%w: source id, container id, and name parameters are required for service option %d", ErrInvalidArgument, optionID)
		}
		disallowedParams = []disallowed{
			{"media_id", p.hasMediaID},
			{"player_id", p.hasPlayerID},
			{"criteria_id", p.hasCriteriaID},
			{"range", p.hasRange},
		}
		cmd = cmd.withIntParam(attrSourceID, p.sourceID).
			withParam(attrContainerID, p.containerID).
			withParam(attrName, p.name)

	case ServiceOptionThumbsUp, ServiceOptionThumbsDown:
		if !p.hasSourceID || !p.hasPlayerID {
			return fmt.Errorf("%w: source id and player id parameters are required for service option %d", ErrInvalidArgument, optionID)
		}
		disallowedParams = []disallowed{
			{"media_id", p.hasMediaID},
			{"container_id", p.hasContainerID},
			{"name", p.hasName},
			{"criteria_id", p.hasCriteriaID},
			{"range", p.hasRange},
		}
		cmd = cmd.withIntParam(attrSourceID, p.sourceID).withIntParam(attrPlayerID, p.playerID)

	case ServiceOptionCreateNewStationBySearchCriteria:
		if !p.hasSourceID || !p.hasName || !p.hasCriteriaID {
			return fmt.Errorf("%w: source id, name, and criteria id parameters are required for service option %d", ErrInvalidArgument, optionID)
		}
		disallowedParams = []disallowed{
			{"media_id", p.hasMediaID},
			{"container_id", p.hasContainerID},
			{"player_id", p.hasPlayerID},
		}
		cmd = cmd.withIntParam(attrSourceID, p.sourceID).
			withIntParam(attrSearchCriteriaID, p.criteriaID).
			withParam(attrName, p.name)
		if p.hasRange {
			cmd = cmd.withParam(attrRange,
				browseOpts{rangeStart: p.rangeStart, rangeEnd: p.rangeEnd}.rangeParam())
		}

	case ServiceOptionAddToFavorites:
		byMedia := p.hasSourceID && p.hasMediaID && p.hasName
		if p.hasPlayerID == byMedia {
			return fmt.Errorf("%w: either player id OR source id, media id, and name are required for service option %d", ErrInvalidArgument, optionID)
		}
		if p.hasPlayerID {
			if p.hasSourceID || p.hasMediaID || p.hasName {
				return fmt.Errorf("%w: source id, media id, and name parameters are not allowed when using player id for service option %d", ErrInvalidArgument, optionID)
			}
			cmd = cmd.withIntParam(attrPlayerID, p.playerID)
		} else {
			cmd = cmd.withIntParam(attrSourceID, p.sourceID).
				withParam(attrMediaID, p.mediaID).
				withParam(attrName, p.name)
		}
		disallowedParams = []disallowed{
			{"container_id", p.hasContainerID},
			{"criteria_id", p.hasCriteriaID},
			{"range", p.hasRange},
		}

	case ServiceOptionRemoveFromFavorites:
		if !p.hasMediaID {
			return fmt.Errorf("%w: media id parameter is required for service option %d", ErrInvalidArgument, optionID)
		}
		cmd = cmd.withParam(attrMediaID, p.mediaID)
		disallowedParams = []disallowed{
			{"source_id", p.hasSourceID},
			{"player_id", p.hasPlayerID},
			{"container_id", p.hasContainerID},
			{"name", p.hasName},
			{"criteria_id", p.hasCriteriaID},
			{"range", p.hasRange},
		}

	default:
		return fmt.Errorf("%w: unknown service option %d", ErrInvalidArgument, optionID)
	}

	var names []string
	violated := false
	for _, d := range disallowedParams {
		names = append(names, d.name)
		if d.present {
			violated = true
		}
	}
	if violated {
		return fmt.Errorf("%w: %s parameters are not allowed for service option %d",
			ErrInvalidArgument, strings.Join(names, ", "), optionID)
	}

	_, err := c.conn.command(ctx, cmd)
	return err
}
