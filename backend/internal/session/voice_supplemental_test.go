package session

import (
	"context"
	"testing"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
)

func TestVoiceMembersPublishedAfterReadySupplemental(t *testing.T) {
	m, n := readyManager(t)
	ev := &gateway.ReadySupplementalEvent{Guilds: []gateway.GuildCreateEvent{{Guild: discord.Guild{ID: guildOmar}, VoiceStates: []discord.VoiceState{{GuildID: guildOmar, ChannelID: chVoice, UserID: adaID, Member: &discord.Member{User: ada}}}}}}
	dispatch(n, ev)
	cached := VoiceMembers(n.Offline(), guildOmar)
	if len(cached.Channels) != 1 || len(cached.Channels[0].Users) != 1 {
		t.Fatalf("supplemental cache not seeded: %+v", cached)
	}
	// Must reach an already-connected socket without a later voice/presence event.
	published := nextVoiceMembers(t, m)
	if published.GuildID != discord.GuildID(guildOmar).String() || len(published.Channels) != 1 || published.Channels[0].Users[0].ID != discord.UserID(adaID).String() {
		t.Fatalf("supplemental roster not published: %+v", published)
	}
	noEvent(t, m)
	result, perr := m.Handle(context.Background(), req(t, `{"v":1,"id":21,"command":"voice_members","guild_id":"200000000000000001"}`))
	if perr != nil {
		t.Fatal(perr)
	}
	if got := result.(protocol.VoiceMembersEvent); len(got.Channels) != 1 {
		t.Fatalf("cached refresh lost roster: %+v", got)
	}
	// The second event must not reconstruct voice, log in or bump generation.
	generation := m.generation
	for _, x := range m.Snapshot() {
		if s, ok := x.(protocol.StateChangedEvent); ok && s.State.Generation != generation {
			t.Fatal("generation changed")
		}
	}
}

func TestVoiceMembersPublishedAfterGuildCreate(t *testing.T) {
	m, n := readyManager(t)
	g, err := n.Cabinet.Guild(guildOmar)
	if err != nil {
		t.Fatal(err)
	}
	dispatch(n, &gateway.GuildCreateEvent{Guild: *g, VoiceStates: []discord.VoiceState{{GuildID: guildOmar, ChannelID: chVoice, UserID: adaID, Member: &discord.Member{User: ada}}}})
	published := nextVoiceMembers(t, m)
	if published.GuildID != discord.GuildID(guildOmar).String() || len(published.Channels) != 1 {
		t.Fatalf("late guild roster not published: %+v", published)
	}
}
