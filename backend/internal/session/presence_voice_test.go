package session

import (
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"testing"
)

func TestVoicePresenceIsIndependentOfMembership(t *testing.T) {
	m, n := readyManager(t)
	dispatch(n, voiceStateEvent(chVoice, ada))
	nextVoiceMembers(t, m)
	if got := VoiceMembers(n.Offline(), guildOmar).Channels[0].Users[0].Status; got != "unknown" {
		t.Fatalf("missing presence invented: %s", got)
	}
	for _, status := range []discord.Status{discord.OnlineStatus, discord.IdleStatus, discord.DoNotDisturbStatus, discord.OfflineStatus} {
		dispatch(n, &gateway.PresenceUpdateEvent{Presence: discord.Presence{GuildID: guildOmar, User: ada, Status: status}})
		ev := nextVoiceMembers(t, m)
		if got := ev.Channels[0].Users[0].Status; got != string(status) {
			t.Fatalf("presence %s got %s", status, got)
		}
	}
	dispatch(n, &gateway.PresenceUpdateEvent{Presence: discord.Presence{GuildID: guildOmar, User: ada, Status: discord.OnlineStatus}})
	nextVoiceMembers(t, m)
	dispatch(n, voiceStateEvent(0, ada))
	nextVoiceMembers(t, m)
	if got := userPresence(n.Offline(), guildOmar, ada.ID); got != "online" {
		t.Fatalf("leaving voice changed Discord presence: %s", got)
	}
}
