package session

import (
	"context"
	"testing"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/ningen/v3"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
)

func TestAckWaitsForRemoteSuccessAndDoesNotRegress(t *testing.T) {
	m, n := readyManager(t)
	fillCache(m, n, chGeneral, guildOmar, 0, 5)
	drain(m)
	calls := 0
	m.rest.ackChannel = func(context.Context, *ningen.State, discord.ChannelID, discord.MessageID) error {
		calls++
		return httpErr(403)
	}
	before := *n.ReadState.ReadState(chGeneral)
	ack := func(id string) *protocol.Error {
		_, e := m.Handle(context.Background(), req(t, `{"v":1,"id":1,"command":"ack","channel_id":"300000000000000002","message_id":"`+id+`"}`))
		return e
	}
	if e := ack((msgBase + 4).String()); e == nil || e.Code != protocol.CodeForbidden {
		t.Fatalf("remote failure accepted: %v, REST calls=%d", e, calls)
	}
	after := n.ReadState.ReadState(chGeneral)
	if after.LastMessageID != before.LastMessageID || after.MentionCount != before.MentionCount {
		t.Fatalf("failed ack changed local state: before=%+v after=%+v", before, *after)
	}
	if calls != 1 {
		t.Fatalf("remote calls=%d", calls)
	}
	m.rest.ackChannel = func(context.Context, *ningen.State, discord.ChannelID, discord.MessageID) error { calls++; return nil }
	if e := ack((msgBase + 4).String()); e != nil {
		t.Fatal(e)
	}
	if got := n.ReadState.ReadState(chGeneral).LastMessageID; got != msgBase+4 {
		t.Fatalf("successful remote ack not reflected: %s", got)
	}
	if e := ack((msgBase + 1).String()); e != nil {
		t.Fatal(e)
	}
	if got := n.ReadState.ReadState(chGeneral).LastMessageID; got != msgBase+4 {
		t.Fatalf("older ack moved read boundary backwards: %s", got)
	}
	if calls != 2 {
		t.Fatalf("old request sent remote ack: calls=%d", calls)
	}
	m.rest.ackChannel = func(context.Context, *ningen.State, discord.ChannelID, discord.MessageID) error {
		dispatch(n, &gateway.MessageAckEvent{ChannelID: chGeneral, MessageID: msgBase + 10})
		return nil
	}
	if e := ack((msgBase + 4).String()); e != nil {
		t.Fatal(e)
	}
	if got := n.ReadState.ReadState(chGeneral).LastMessageID; got != msgBase+10 {
		t.Fatalf("in-flight newer ack regressed: %s", got)
	}
	m.mu.Lock()
	m.lifecycle = protocol.LifecycleConnecting
	m.mu.Unlock()
	if e := ack((msgBase + 4).String()); e == nil || e.Code != protocol.CodeGatewayUnavailable {
		t.Fatalf("offline write accepted: %v", e)
	}
}
