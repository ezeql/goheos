package goheos_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/ezeql/goheos/internal/heostest"
)

func TestGetGroups(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	registerGroupCommands(d)
	c := connectClient(t, d)

	groups, err := c.GetGroups(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	g := groups[1]
	if g.Name() != "Back Patio + Front Porch" || g.ID() != 1 {
		t.Errorf("group base info wrong: %q id=%d", g.Name(), g.ID())
	}
	if g.LeadPlayerID() != 1 {
		t.Errorf("leader = %d, want 1", g.LeadPlayerID())
	}
	if members := g.MemberPlayerIDs(); len(members) != 1 || members[0] != 2 {
		t.Errorf("members = %v", members)
	}
	if g.Volume() != 42 || g.IsMuted() {
		t.Errorf("group state wrong: vol=%d muted=%v", g.Volume(), g.IsMuted())
	}
}

func TestGroupCommands(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	registerGroupCommands(d)
	c := connectClient(t, d)
	groups, err := c.GetGroups(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	g := groups[1]
	ctx := context.Background()

	d.Register("group/set_volume", map[string]string{"gid": "1", "level": "25"}, "group.set_volume")
	if err := g.SetVolume(ctx, 25); err != nil {
		t.Fatal(err)
	}
	d.Register("group/volume_up", map[string]string{"gid": "1", "step": "6"}, "group.volume_up")
	if err := g.VolumeUp(ctx, 6); err != nil {
		t.Fatal(err)
	}
	d.Register("group/volume_down", map[string]string{"gid": "1", "step": "5"}, "group.volume_down")
	if err := g.VolumeDown(ctx, 5); err != nil {
		t.Fatal(err)
	}
	d.Register("group/set_mute", map[string]string{"gid": "1", "state": "on"}, "group.set_mute")
	if err := g.Mute(ctx); err != nil {
		t.Fatal(err)
	}
	d.Register("group/toggle_mute", map[string]string{"gid": "1"}, "group.toggle_mute")
	if err := g.ToggleMute(ctx); err != nil {
		t.Fatal(err)
	}
	d.AssertCommandCalled(t, "group/set_volume", map[string]string{"gid": "1", "level": "25"})
}

func TestSetGroupVariants(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	c := connectClient(t, d)
	ctx := context.Background()

	// The first player id is the leader; ids are sent as CSV.
	d.Register("group/set_group", map[string]string{"pid": "1,2,3"}, "group.set_group_create")
	if err := c.CreateGroup(ctx, 1, []int{2, 3}); err != nil {
		t.Fatal(err)
	}
	d.AssertCommandCalled(t, "group/set_group", map[string]string{"pid": "1,2,3"})

	// Removing a group sends only the group (leader) id.
	d.RegisterReplace("group/set_group", map[string]string{"pid": "1"}, "group.set_group_remove")
	if err := c.RemoveGroup(ctx, 1); err != nil {
		t.Fatal(err)
	}
	d.AssertCommandCalled(t, "group/set_group", map[string]string{"pid": "1"})

	d.RegisterReplace("group/set_group", map[string]string{"pid": "1,4"}, "group.set_group_update")
	if err := c.UpdateGroup(ctx, 1, []int{4}); err != nil {
		t.Fatal(err)
	}
}

func TestGetGroupInfoNotLoaded(t *testing.T) {
	t.Parallel()
	d := heostest.StartDevice(t)
	// The fixture reports gid -263109739; the volume/mute refresh uses it.
	const gid = -263109739
	gidArgs := map[string]string{"gid": strconv.Itoa(gid)}
	d.Register("group/get_group_info", gidArgs, "group.get_group_info")
	d.Register("group/get_volume", gidArgs, "group.get_volume")
	d.Register("group/get_mute", gidArgs, "group.get_mute")
	c := connectClient(t, d)

	g, err := c.GetGroupInfo(context.Background(), gid)
	if err != nil {
		t.Fatal(err)
	}
	if g.ID() != gid || g.LeadPlayerID() != gid {
		t.Errorf("group info wrong: id=%d leader=%d", g.ID(), g.LeadPlayerID())
	}
}
