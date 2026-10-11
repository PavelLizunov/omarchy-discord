package session

import (
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"testing"
)

func TestDMRecipientsContainPresenceAndCustomStatus(t *testing.T) {
	m, n := readyManager(t)
	dispatch(n, &gateway.PresenceUpdateEvent{Presence: discord.Presence{User: discord.User{ID: adaID}, Status: discord.IdleStatus, Activities: []discord.Activity{{Type: discord.GameActivity, Name: "Chess"}, {Type: discord.CustomActivity, State: "Reading"}}}})
	r := routed(t, m)
	if !r.All {
		t.Fatal("closed DM list cannot receive presence")
	}
	ch, err := n.Cabinet.Channel(dmAda)
	if err != nil {
		t.Fatal(err)
	}
	u := wireChannel(n.Offline(), *ch).Recipients[0]
	if u.Status != "idle" || u.Activity != "Reading" {
		t.Fatalf("recipient presence %+v", u)
	}
	ch, err = n.Cabinet.Channel(dmGroup)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range wireChannel(n.Offline(), *ch).Recipients {
		if u.ID == discord.UserID(linID).String() && u.Status != "unknown" {
			t.Fatalf("missing presence %+v", u)
		}
	}
}
