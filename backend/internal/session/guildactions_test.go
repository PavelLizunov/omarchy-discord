package session

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/ningen/v3"
	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
)

func TestGuildActions(t *testing.T) {
	m, n := readyManager(t)
	online := uint64(0)
	called := 0
	m.rest.guildStats = func(context.Context, *ningen.State, discord.GuildID) (*guildCounts, error) {
		return &guildCounts{Online: &online}, nil
	}
	m.rest.leaveGuild = func(context.Context, *ningen.State, discord.GuildID) error { called++; return nil }
	m.rest.muteGuild = func(context.Context, *ningen.State, discord.GuildID, bool) error { called++; return nil }
	m.rest.ackChannel = func(_ context.Context, _ *ningen.State, ch discord.ChannelID, msg discord.MessageID) error {
		if ch == chSecret {
			t.Fatal("acknowledged inaccessible channel")
		}
		called++
		return nil
	}
	call := func(command, fields string) (any, *protocol.Error) {
		return m.Handle(context.Background(), req(t, `{"v":1,"id":1,"command":"`+command+`","guild_id":"200000000000000001"`+fields+`}`))
	}
	res, e := call("guild_stats", "")
	if e != nil || res.(guildStatsResult).OnlineCount != 0 {
		t.Fatalf("genuine zero: %v %v", res, e)
	}
	m.rest.guildStats = func(context.Context, *ningen.State, discord.GuildID) (*guildCounts, error) {
		return &guildCounts{}, nil
	}
	if _, e = call("guild_stats", ""); e == nil {
		t.Fatal("missing count was accepted as zero")
	}
	if _, e = call("leave_guild", ""); e == nil || called != 0 {
		t.Fatal("unconfirmed leave")
	}
	if _, e = call("leave_guild", `,"confirmed":true`); e != nil || called != 1 {
		t.Fatalf("leave: %v", e)
	}
	_, e = m.Handle(context.Background(), req(t, `{"v":1,"id":1,"command":"leave_guild","guild_id":"200000000000000002","confirmed":true}`))
	if e == nil || called != 1 {
		t.Fatal("owner allowed to leave")
	}
	if _, e = call("set_guild_mute", ""); e == nil {
		t.Fatal("missing muted")
	}
	if _, e = call("set_guild_mute", `,"muted":true`); e != nil {
		t.Fatal(e)
	}
	if !n.MutedState.Guild(guildOmar, false) {
		t.Fatal("mute not reflected")
	}
	if _, e = call("set_guild_mute", `,"muted":false`); e != nil {
		t.Fatal(e)
	}
	if n.MutedState.Guild(guildOmar, false) {
		t.Fatal("unmute not reflected")
	}
	before := called
	if _, e = call("mark_guild_read", ""); e != nil {
		t.Fatal(e)
	}
	if called <= before {
		t.Fatal("no acknowledgements")
	}
	m.rest.ackChannel = func(context.Context, *ningen.State, discord.ChannelID, discord.MessageID) error { return httpErr(403) }
	if _, e = call("mark_guild_read", ""); e == nil {
		t.Fatal("remote failure suppressed")
	}
	m.rest.leaveGuild = func(context.Context, *ningen.State, discord.GuildID) error { return httpErr(403) }
	if _, e = call("leave_guild", `,"confirmed":true`); e == nil {
		t.Fatal("leave failure suppressed")
	}
	m.mu.Lock()
	m.lifecycle = protocol.LifecycleConnecting
	m.mu.Unlock()
	if _, e = call("mark_guild_read", ""); e == nil {
		t.Fatal("offline write")
	}
}

func TestGuildCountDecode(t *testing.T) {
	for _, raw := range []string{`{}`, `{"approximate_presence_count":null}`, `{"approximate_presence_count":0}`} {
		var counts guildCounts
		if e := json.Unmarshal([]byte(raw), &counts); e != nil {
			t.Fatal(e)
		}
		if (counts.Online != nil) != (raw == `{"approximate_presence_count":0}`) {
			t.Fatal("unknown/zero conflated")
		}
	}
}
