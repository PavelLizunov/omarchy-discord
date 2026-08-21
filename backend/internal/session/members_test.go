package session

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/ningen/v3"

	"github.com/mattcalayo/omarchy-discord/backend/internal/socket"
)

const (
	chTeam = discord.ChannelID(300000000000000011)
	thTeam = discord.ChannelID(310000000000000011)
	roleQA = discord.RoleID(250000000000000009)
)

// cid renders one of the untyped fixture snowflake constants.
func cid(id discord.ChannelID) string { return id.String() }

func subscribe(t *testing.T, m *Manager, ctx context.Context, id int, chID discord.ChannelID) {
	t.Helper()
	line := `{"v":1,"id":` + strconv.Itoa(id) + `,"command":"subscribe_members","channel_id":"` + chID.String() + `"}`
	if _, e := m.Handle(ctx, req(t, line)); e != nil {
		t.Fatalf("subscribe %s: %v", chID, e)
	}
}

func unsubscribe(t *testing.T, m *Manager, ctx context.Context, id int, chID discord.ChannelID) {
	t.Helper()
	line := `{"v":1,"id":` + strconv.Itoa(id) + `,"command":"unsubscribe_members","channel_id":"` + chID.String() + `"}`
	if _, e := m.Handle(ctx, req(t, line)); e != nil {
		t.Fatalf("unsubscribe %s: %v", chID, e)
	}
}

// Two channels of one guild share the "everyone" list. Subscribing the second
// one after the first list arrived used to reach ningen's RequestMemberList,
// whose chunk arithmetic panics ("makeslice: cap out of range") for a list with
// fewer than 100 visible members — killing the daemon on an ordinary channel
// switch with the member pane open.
func TestSubscribeSharedMemberListDoesNotPanic(t *testing.T) {
	m, n := readyManager(t)
	m.memberDebounce = time.Millisecond
	ctx := socket.WithClient(context.Background(), newFakeClient())

	subscribe(t, m, ctx, 1, chGeneral)
	// A small list: 3 rows, well under the 100 that ningen's arithmetic assumes.
	dispatch(n, listUpdate(gateway.GuildMemberListOp{Op: "SYNC", Range: [2]int{0, 99}, Items: []gateway.GuildMemberListOpItem{
		groupItem("online", 2), listItem(ada, "", discord.OnlineStatus), listItem(bob, "", discord.IdleStatus),
		groupItem("offline", 1), listItem(lin, "", discord.OfflineStatus),
	}}))
	if _, ev := memberList(t, m); ev.ChannelID != cid(chGeneral) || len(ev.Members) != 3 {
		t.Fatalf("first list: %+v", ev)
	}

	// #dev shares the list id: the subscribe must survive and re-emit at once.
	subscribe(t, m, ctx, 2, chDev)
	r, ev := memberList(t, m)
	if ev.ChannelID != cid(chDev) || len(ev.Members) != 3 || len(r.Members) != 1 || r.Members[0] != cid(chDev) {
		t.Fatalf("shared list re-emit: %+v %+v", r, ev)
	}
	if got := m.memberRanges(n.Offline(), guildOmar); len(got) != 2 || len(got[chGeneral]) != 1 || len(got[chDev]) != 1 {
		t.Fatalf("both channels must be in the Op 14: %+v", got)
	}
	// A later op updates both channels from one gateway event.
	dispatch(n, listUpdate(gateway.GuildMemberListOp{Op: "UPDATE", Index: 1, Item: listItem(ada, "", discord.DoNotDisturbStatus)}))
	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		_, ev := memberList(t, m)
		seen[ev.ChannelID] = true
	}
	if !seen[cid(chGeneral)] || !seen[cid(chDev)] {
		t.Fatalf("both channels must re-emit: %v", seen)
	}
}

// addTeamThread injects a channel with a deny overwrite (so its member list is
// not "everyone") and a thread under it.
func addTeamThread(t *testing.T, m *Manager, n *ningen.State) {
	t.Helper()
	dispatch(n, &gateway.ChannelCreateEvent{Channel: discord.Channel{
		ID: chTeam, GuildID: guildOmar, Type: discord.GuildText, Name: "team", ParentID: catGeneral,
		Overwrites: []discord.Overwrite{{ID: discord.Snowflake(roleQA), Type: discord.OverwriteRole, Deny: discord.PermissionViewChannel}},
	}})
	channelUpdate(t, m)
	dispatch(n, &gateway.ThreadCreateEvent{Channel: discord.Channel{
		ID: thTeam, GuildID: guildOmar, Type: discord.GuildPrivateThread, Name: "team-thread", ParentID: chTeam,
		ThreadMetadata: &discord.ThreadMetadata{},
	}})
	channelUpdate(t, m)
}

// A thread carries no overwrites, so computing its member list from its own
// (empty) set served the guild's public "everyone" list — including for threads
// under a restricted parent. The parent's list is the right one.
func TestThreadMemberListFollowsParent(t *testing.T) {
	m, n := readyManager(t)
	m.memberDebounce = time.Millisecond
	addTeamThread(t, m, n)
	ctx := socket.WithClient(context.Background(), newFakeClient())

	off := n.Offline()
	parent, err := off.Cabinet.Channel(chTeam)
	if err != nil {
		t.Fatal(err)
	}
	teamList := memberListID(off, parent)
	if teamList == "everyone" {
		t.Fatal("fixture parent must not use the everyone list")
	}

	subscribe(t, m, ctx, 1, thTeam)
	// The Op 14 asks for the parent, not the thread.
	if got := m.memberRanges(off, guildOmar); len(got) != 1 || len(got[chTeam]) != 1 {
		t.Fatalf("thread must subscribe its parent: %+v", got)
	}
	// The guild's public list is not this thread's list.
	dispatch(n, listUpdate(gateway.GuildMemberListOp{Op: "SYNC", Range: [2]int{0, 99}, Items: []gateway.GuildMemberListOpItem{
		groupItem("online", 1), listItem(ada, "", discord.OnlineStatus)}}))
	noEvent(t, m)
	// The parent's list is.
	ev := listUpdate(gateway.GuildMemberListOp{Op: "SYNC", Range: [2]int{0, 99}, Items: []gateway.GuildMemberListOpItem{
		groupItem("online", 1), listItem(bob, "", discord.OnlineStatus)}})
	ev.ID = teamList
	dispatch(n, ev)
	_, got := memberList(t, m)
	if got.ChannelID != cid(thTeam) || len(got.Members) != 1 || got.Members[0].User.Username != "bob" {
		t.Fatalf("thread list: %+v", got)
	}
}

// Subscriptions are refcounted across connections: per-channel state lives
// exactly as long as some client displays the channel, and a dropped
// connection releases its share.
func TestMemberSubscriptionRefcounting(t *testing.T) {
	m, n := readyManager(t)
	m.memberDebounce = time.Millisecond
	c1, c2 := newFakeClient(), newFakeClient()
	ctx1 := socket.WithClient(context.Background(), c1)
	ctx2 := socket.WithClient(context.Background(), c2)

	subscribe(t, m, ctx1, 1, chGeneral)
	subscribe(t, m, ctx2, 2, chGeneral)
	dispatch(n, listUpdate(gateway.GuildMemberListOp{Op: "SYNC", Range: [2]int{0, 99}, Items: []gateway.GuildMemberListOpItem{
		groupItem("online", 1), listItem(ada, "", discord.OnlineStatus)}}))
	memberList(t, m)

	unsubscribe(t, m, ctx1, 3, chGeneral)
	if len(m.members.subscribed()) != 1 || len(m.trackedUsers()) != 1 {
		t.Fatal("the second client still displays the channel")
	}
	// The remaining client still gets updates.
	dispatch(n, listUpdate(gateway.GuildMemberListOp{Op: "UPDATE", Index: 1, Item: listItem(ada, "", discord.IdleStatus)}))
	memberList(t, m)

	// The connection of the last subscriber drops: everything goes with it.
	m.ClientClosed(c2)
	if len(m.members.subscribed()) != 0 || len(m.trackedUsers()) != 0 {
		t.Fatalf("state left behind: subs=%v users=%v", m.members.subscribed(), m.trackedUsers())
	}
	if got := m.memberRanges(n.Offline(), guildOmar); len(got) != 0 {
		t.Fatalf("channel still in the Op 14: %+v", got)
	}
	dispatch(n, listUpdate(gateway.GuildMemberListOp{Op: "UPDATE", Index: 1, Item: listItem(ada, "", discord.OnlineStatus)}))
	noEvent(t, m)
	// A presence for a user of the dropped list routes nowhere.
	dispatch(n, &gateway.PresenceUpdateEvent{Presence: discord.Presence{User: discord.User{ID: bobID}, GuildID: guildOmar, Status: discord.IdleStatus}})
	noEvent(t, m)
}

// A re-IDENTIFY wipes ningen's member lists. The subscriptions survive it: the
// backend re-requests them and the pane fills again instead of showing
// pre-reconnect data forever.
func TestMemberListsReplayedAfterReady(t *testing.T) {
	m, n := readyManager(t)
	m.memberDebounce = time.Millisecond
	ctx := socket.WithClient(context.Background(), newFakeClient())

	subscribe(t, m, ctx, 1, chGeneral)
	subscribe(t, m, ctx, 2, dmGroup)
	memberList(t, m) // the DM list is synthesized at once
	dispatch(n, listUpdate(gateway.GuildMemberListOp{Op: "SYNC", Range: [2]int{0, 99}, Items: []gateway.GuildMemberListOpItem{
		groupItem("online", 1), listItem(ada, "", discord.OnlineStatus)}}))
	memberList(t, m)

	_, ready := newUnopenedState(t)
	dispatch(n, ready)
	drain(m)

	// The guild list ningen threw away is not left behind to route presences
	// with (the DM list is synthesized locally and is re-emitted at once).
	if _, stale := m.trackedUsers()[chGeneral]; stale {
		t.Fatalf("stale user set after READY: %v", m.trackedUsers())
	}
	// ...but the subscriptions and the gateway request survived.
	if len(m.members.subscribed()) != 2 {
		t.Fatalf("subscriptions lost: %v", m.members.subscribed())
	}
	if got := m.memberRanges(n.Offline(), guildOmar); len(got) != 1 || len(got[chGeneral]) != 1 {
		t.Fatalf("member list not re-requested: %+v", got)
	}
	dispatch(n, listUpdate(gateway.GuildMemberListOp{Op: "SYNC", Range: [2]int{0, 99}, Items: []gateway.GuildMemberListOpItem{
		groupItem("online", 1), listItem(bob, "", discord.OnlineStatus)}}))
	_, ev := memberList(t, m)
	if ev.ChannelID != cid(chGeneral) || len(ev.Members) != 1 || ev.Members[0].User.Username != "bob" {
		t.Fatalf("post-reconnect list: %+v", ev)
	}
}

// A panicking gateway handler degrades one event instead of the process.
func TestSyncHandlerPanicIsContained(t *testing.T) {
	m, n := readyManager(t)
	addSyncHandler(n, "test_panic", func(*gateway.TypingStartEvent) { panic("boom") })
	dispatch(n, &gateway.TypingStartEvent{ChannelID: chGeneral, GuildID: guildOmar, UserID: adaID, Timestamp: discord.UnixTimestamp(time.Now().Unix())})
	// The session is still usable afterwards.
	if _, e := m.Handle(context.Background(), req(t, `{"v":1,"id":1,"command":"list_guilds"}`)); e != nil {
		t.Fatalf("session died with the handler: %v", e)
	}
}

// trackedUsers exposes the per-channel user sets for assertions.
func (m *Manager) trackedUsers() map[discord.ChannelID]int {
	m.members.mu.Lock()
	defer m.members.mu.Unlock()
	out := map[discord.ChannelID]int{}
	for chID, users := range m.members.users {
		out[chID] = len(users)
	}
	return out
}
