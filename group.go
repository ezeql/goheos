package goheos

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sync"
)

// Group is a group of players. Its state is kept up to date by the client
// while change events are enabled; access it through the getter methods,
// which are safe for concurrent use.
type Group struct {
	client *Client

	mu              sync.RWMutex
	name            string
	groupID         int
	leadPlayerID    int
	memberPlayerIDs []int
	volume          int
	isMuted         bool
}

func groupFromData(data payloadData, client *Client) (*Group, error) {
	g := &Group{client: client}
	if err := g.updateFromData(data); err != nil {
		return nil, err
	}
	return g, nil
}

func (g *Group) updateFromData(data payloadData) error {
	players, ok := data[attrPlayers].([]any)
	if !ok {
		return errors.New("goheos: group payload has no players list")
	}
	leadPlayerID := 0
	hasLeader := false
	var memberPlayerIDs []int
	for _, entry := range players {
		member, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		memberID, _ := payloadData(member).intVal(attrPlayerID)
		if payloadData(member).str(attrRole) == valueLeader {
			leadPlayerID = memberID
			hasLeader = true
		} else {
			memberPlayerIDs = append(memberPlayerIDs, memberID)
		}
	}
	if !hasLeader {
		return errors.New("goheos: no leader found in group data")
	}
	groupID, _ := data.intVal(attrGroupID)
	g.mu.Lock()
	defer g.mu.Unlock()
	g.name = data.str(attrName)
	g.groupID = groupID
	g.leadPlayerID = leadPlayerID
	g.memberPlayerIDs = memberPlayerIDs
	return nil
}

// ID returns the group identifier, which equals the lead player's id.
func (g *Group) ID() int { g.mu.RLock(); defer g.mu.RUnlock(); return g.groupID }

// Name returns the group name.
func (g *Group) Name() string { g.mu.RLock(); defer g.mu.RUnlock(); return g.name }

// LeadPlayerID returns the id of the group leader.
func (g *Group) LeadPlayerID() int { g.mu.RLock(); defer g.mu.RUnlock(); return g.leadPlayerID }

// MemberPlayerIDs returns the ids of the non-leader members.
func (g *Group) MemberPlayerIDs() []int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return append([]int(nil), g.memberPlayerIDs...)
}

// Volume returns the last known group volume (0-100).
func (g *Group) Volume() int { g.mu.RLock(); defer g.mu.RUnlock(); return g.volume }

// IsMuted returns the last known group mute state.
func (g *Group) IsMuted() bool { g.mu.RLock(); defer g.mu.RUnlock(); return g.isMuted }

// onHeosEvent updates the group from a device event, reporting whether the
// event applied to this group.
func (g *Group) onHeosEvent(m *message) bool {
	if EventCommand(m.command) != EventGroupVolumeChanged {
		return false
	}
	groupID, err := m.intValue(attrGroupID)
	if err != nil || groupID != g.ID() {
		return false
	}
	level, err := m.intValue(attrLevel)
	if err != nil {
		return false
	}
	mute, err := m.value(attrMute)
	if err != nil {
		return false
	}
	g.mu.Lock()
	g.volume = level
	g.isMuted = mute == valueOn
	g.mu.Unlock()
	return true
}

// refresh pulls the group's volume and mute state.
func (g *Group) refresh(ctx context.Context) error {
	volume, err := g.client.GetGroupVolume(ctx, g.ID())
	if err != nil {
		return err
	}
	muted, err := g.client.GetGroupMute(ctx, g.ID())
	if err != nil {
		return err
	}
	g.mu.Lock()
	g.volume = volume
	g.isMuted = muted
	g.mu.Unlock()
	return nil
}

// SetVolume sets the group volume (0-100).
func (g *Group) SetVolume(ctx context.Context, level int) error {
	return g.client.SetGroupVolume(ctx, g.ID(), level)
}

// VolumeUp raises the group volume by step (1-10).
func (g *Group) VolumeUp(ctx context.Context, step int) error {
	return g.client.GroupVolumeUp(ctx, g.ID(), step)
}

// VolumeDown lowers the group volume by step (1-10).
func (g *Group) VolumeDown(ctx context.Context, step int) error {
	return g.client.GroupVolumeDown(ctx, g.ID(), step)
}

// SetMute sets the group mute state.
func (g *Group) SetMute(ctx context.Context, mute bool) error {
	return g.client.SetGroupMute(ctx, g.ID(), mute)
}

// Mute mutes the group.
func (g *Group) Mute(ctx context.Context) error { return g.SetMute(ctx, true) }

// Unmute unmutes the group.
func (g *Group) Unmute(ctx context.Context) error { return g.SetMute(ctx, false) }

// ToggleMute toggles the group mute state.
func (g *Group) ToggleMute(ctx context.Context) error {
	return g.client.GroupToggleMute(ctx, g.ID())
}

// Client group commands.

// Groups returns the loaded groups without contacting the device. The map is
// a copy; the groups are shared, live objects.
func (c *Client) Groups() map[int]*Group {
	c.mu.RLock()
	defer c.mu.RUnlock()
	groups := make(map[int]*Group, len(c.groups))
	maps.Copy(groups, c.groups)
	return groups
}

// GetGroups returns the groups, loading them from the device on first use.
// Use RefreshGroups to force a reload.
func (c *Client) GetGroups(ctx context.Context) (map[int]*Group, error) {
	c.mu.RLock()
	loaded := c.groupsLoaded
	c.mu.RUnlock()
	if !loaded {
		if err := c.RefreshGroups(ctx); err != nil {
			return nil, err
		}
	}
	return c.Groups(), nil
}

// RefreshGroups reloads the groups from the device.
func (c *Client) RefreshGroups(ctx context.Context) error {
	resp, err := c.conn.command(ctx, newCommand(cmdGetGroups))
	if err != nil {
		return err
	}
	payload, err := decodePayloadList(resp.payload)
	if err != nil {
		return err
	}
	groups := make(map[int]*Group, len(payload))
	for _, data := range payload {
		group, err := groupFromData(data, c)
		if err != nil {
			return err
		}
		groups[group.ID()] = group
	}
	for _, group := range groups {
		if err := group.refresh(ctx); err != nil {
			return err
		}
	}
	c.mu.Lock()
	c.groups = groups
	c.groupsLoaded = true
	c.mu.Unlock()
	return nil
}

// GetGroupInfo returns the group with the given id, fetching it from the
// device when not already loaded.
func (c *Client) GetGroupInfo(ctx context.Context, groupID int) (*Group, error) {
	c.mu.RLock()
	group := c.groups[groupID]
	c.mu.RUnlock()
	if group != nil {
		return group, nil
	}
	resp, err := c.conn.command(ctx,
		newCommand(cmdGetGroupInfo).withIntParam(attrGroupID, groupID))
	if err != nil {
		return nil, err
	}
	data, err := decodePayloadObject(resp.payload)
	if err != nil {
		return nil, err
	}
	group, err = groupFromData(data, c)
	if err != nil {
		return nil, err
	}
	if err := group.refresh(ctx); err != nil {
		return nil, err
	}
	return group, nil
}

// SetGroup creates, modifies, or ungroups players. The first player id is the
// group leader; passing a single id ungroups that player's group.
func (c *Client) SetGroup(ctx context.Context, playerIDs []int) error {
	_, err := c.conn.command(ctx,
		newCommand(cmdSetGroup).withParam(attrPlayerID, joinInts(playerIDs)))
	return err
}

// CreateGroup creates a group with the given leader and members.
func (c *Client) CreateGroup(ctx context.Context, leaderPlayerID int, memberPlayerIDs []int) error {
	return c.SetGroup(ctx, append([]int{leaderPlayerID}, memberPlayerIDs...))
}

// RemoveGroup ungroups the given group. The group id equals the lead
// player's id.
func (c *Client) RemoveGroup(ctx context.Context, groupID int) error {
	return c.SetGroup(ctx, []int{groupID})
}

// UpdateGroup replaces the membership of a group.
func (c *Client) UpdateGroup(ctx context.Context, groupID int, memberPlayerIDs []int) error {
	return c.CreateGroup(ctx, groupID, memberPlayerIDs)
}

// GetGroupVolume returns the group volume (0-100).
func (c *Client) GetGroupVolume(ctx context.Context, groupID int) (int, error) {
	resp, err := c.conn.command(ctx,
		newCommand(cmdGetGroupVolume).withIntParam(attrGroupID, groupID))
	if err != nil {
		return 0, err
	}
	return resp.intValue(attrLevel)
}

// SetGroupVolume sets the group volume (0-100).
func (c *Client) SetGroupVolume(ctx context.Context, groupID, level int) error {
	if level < 0 || level > 100 {
		return fmt.Errorf("%w: level must be in the range 0-100", ErrInvalidArgument)
	}
	_, err := c.conn.command(ctx, newCommand(cmdSetGroupVolume).
		withIntParam(attrGroupID, groupID).
		withIntParam(attrLevel, level))
	return err
}

// GroupVolumeUp raises the group volume by step (1-10).
func (c *Client) GroupVolumeUp(ctx context.Context, groupID, step int) error {
	if step < 1 || step > 10 {
		return fmt.Errorf("%w: step must be in the range 1-10", ErrInvalidArgument)
	}
	_, err := c.conn.command(ctx, newCommand(cmdGroupVolumeUp).
		withIntParam(attrGroupID, groupID).
		withIntParam(attrStep, step))
	return err
}

// GroupVolumeDown lowers the group volume by step (1-10).
func (c *Client) GroupVolumeDown(ctx context.Context, groupID, step int) error {
	if step < 1 || step > 10 {
		return fmt.Errorf("%w: step must be in the range 1-10", ErrInvalidArgument)
	}
	_, err := c.conn.command(ctx, newCommand(cmdGroupVolumeDown).
		withIntParam(attrGroupID, groupID).
		withIntParam(attrStep, step))
	return err
}

// GetGroupMute returns the group mute state.
func (c *Client) GetGroupMute(ctx context.Context, groupID int) (bool, error) {
	resp, err := c.conn.command(ctx,
		newCommand(cmdGetGroupMute).withIntParam(attrGroupID, groupID))
	if err != nil {
		return false, err
	}
	state, err := resp.value(attrState)
	if err != nil {
		return false, err
	}
	return state == valueOn, nil
}

// SetGroupMute sets the group mute state.
func (c *Client) SetGroupMute(ctx context.Context, groupID int, mute bool) error {
	_, err := c.conn.command(ctx, newCommand(cmdSetGroupMute).
		withIntParam(attrGroupID, groupID).
		withParam(attrState, onOff(mute)))
	return err
}

// GroupToggleMute toggles the group mute state.
func (c *Client) GroupToggleMute(ctx context.Context, groupID int) error {
	_, err := c.conn.command(ctx,
		newCommand(cmdGroupToggleMute).withIntParam(attrGroupID, groupID))
	return err
}
