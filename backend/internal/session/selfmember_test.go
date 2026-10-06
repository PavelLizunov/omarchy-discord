package session

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/utils/httputil/httpdriver"
	"github.com/diamondburned/ningen/v3/states/member"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
)

type countingDriver struct{ calls atomic.Int64 }

func (d *countingDriver) NewRequest(context.Context, string, string) (httpdriver.Request, error) {
	d.calls.Add(1)
	return nil, errors.New("REST disabled in test")
}
func (d *countingDriver) Do(httpdriver.Request) (httpdriver.Response, error) {
	return nil, errors.New("REST disabled in test")
}

func TestSelfMemberSeededFromReady(t *testing.T) {
	n, ready := newUnopenedState(t)
	dispatch(n, ready)
	off := n.Offline()
	me, _ := off.Cabinet.Me()
	if _, err := off.Cabinet.Member(guildOmar, me.ID); err == nil {
		t.Fatal("fixture must not cache the self member before seeding")
	}
	if got := QuickSwitch(off, "", 0); len(got) == 0 || got[0].GuildName != nil {
		t.Fatalf("unseeded sanity: want DM-only entries, got %+v", got)
	}
	for _, e := range QuickSwitch(off, "", 0) {
		if e.GuildName != nil {
			t.Fatalf("unseeded: guild channel %q leaked through", e.Channel.Name)
		}
	}

	seedSelfMembers(n, ready)
	for _, g := range []discord.GuildID{guildOmar, guildQuiet} {
		mem, err := off.Cabinet.Member(g, me.ID)
		if err != nil {
			t.Fatalf("guild %d: self member not seeded", g)
		}
		if mem.User.Username != "tester" {
			t.Fatalf("seeded member must carry the full user: %+v", mem.User)
		}
	}
	var guild int
	for _, e := range QuickSwitch(off, "", 0) {
		if e.GuildName != nil {
			guild++
		}
		if e.Channel.Name == "secret" {
			t.Fatal("denied channel must stay hidden")
		}
	}
	if guild < 4 {
		t.Fatalf("seeded: want the visible guild channels, got %d", guild)
	}
	seedSelfMembers(n, ready)
	cpy := *ready
	cpy.RawEventBody = []byte(`{}`)
	seedSelfMembers(n, &cpy)
}

func TestStructureCommandsAreCacheOnly(t *testing.T) {
	m, n := readyManager(t)
	driver := &countingDriver{}
	n.Client.Client.Client = driver
	for _, line := range []string{
		`{"v":1,"id":1,"command":"list_guilds"}`,
		`{"v":1,"id":2,"command":"list_channels","guild_id":"200000000000000001"}`,
		`{"v":1,"id":3,"command":"quick_switch","query":"gen"}`,
	} {
		res, e := m.Handle(context.Background(), req(t, line))
		if e != nil {
			t.Fatalf("%s: %v", line, e)
		}
		switch r := res.(type) {
		case protocol.ListGuildsResult:
			if len(r.Guilds) != 2 || r.Guilds[1].Unread != protocol.UnreadMentioned {
				t.Fatalf("list_guilds: %+v", r.Guilds)
			}
		case protocol.ListChannelsResult:
			if len(r.Channels) < 4 {
				t.Fatalf("list_channels: %+v", r.Channels)
			}
		case protocol.QuickSwitchResult:
			if len(r.Entries) != 1 || r.Entries[0].Channel.Name != "general" {
				t.Fatalf("quick_switch: %+v", r.Entries)
			}
		}
	}
	if c := driver.calls.Load(); c != 0 {
		t.Fatalf("structure commands made %d REST calls", c)
	}
}

func TestMemberListID(t *testing.T) {
	n := loadOfflineState(t)
	ow := func(id discord.Snowflake, allow bool) discord.Overwrite {
		if allow {
			return discord.Overwrite{ID: id, Type: discord.OverwriteRole, Allow: discord.PermissionViewChannel}
		}
		return discord.Overwrite{ID: id, Type: discord.OverwriteRole, Deny: discord.PermissionViewChannel}
	}
	ch := func(g discord.GuildID, ows ...discord.Overwrite) *discord.Channel {
		return &discord.Channel{ID: chGeneral, GuildID: g, Overwrites: ows}
	}
	n.Cabinet.RoleSet(guildQuiet, &discord.Role{ID: discord.RoleID(guildQuiet), Name: "@everyone"}, true)

	sortedHash := func(allows, denies []discord.Snowflake) string {
		var ows []discord.Overwrite
		for _, a := range allows {
			ows = append(ows, ow(a, true))
		}
		for _, d := range denies {
			ows = append(ows, ow(d, false))
		}
		return member.ComputeListID(ows)
	}
	cases := []struct {
		name string
		ch   *discord.Channel
		want string
	}{
		{"no overwrites, public", ch(guildOmar), "everyone"},
		{"allows only, public", ch(guildOmar, ow(30, true), ow(10, true), ow(20, true)), "everyone"},
		{"no overwrites, private guild", ch(guildQuiet), "everyone"},
		{"allows only, private guild", ch(guildQuiet, ow(30, true), ow(10, true), ow(20, true)), sortedHash([]discord.Snowflake{10, 20, 30}, nil)},
		{"deny, public", ch(guildOmar, ow(30, true), ow(guildOmar, false), ow(10, true)), sortedHash([]discord.Snowflake{10, 30}, []discord.Snowflake{guildOmar})},
		{"denies sorted", ch(guildOmar, ow(50, false), ow(40, false)), sortedHash(nil, []discord.Snowflake{40, 50})},
		{"string order", ch(guildQuiet, ow(99999999999999999, true), ow(100000000000000000, true)), sortedHash([]discord.Snowflake{100000000000000000, 99999999999999999}, nil)},
		{"non-view overwrites ignored", ch(guildOmar, discord.Overwrite{ID: 7, Allow: discord.PermissionSendMessages}), "everyone"},
	}
	for _, c := range cases {
		if got := memberListID(n, c.ch); got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
	if got := memberListID(n, ch(guildQuiet, ow(30, true), ow(10, true))); got == member.ComputeListID(ch(guildQuiet, ow(30, true), ow(10, true)).Overwrites) {
		t.Error("unsorted input must not reproduce ningen's payload-order hash")
	}
}
