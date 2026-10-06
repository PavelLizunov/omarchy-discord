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

func TestSubscribeSharedMemberListDoesNotPanic(t *testing.T) {
	m, n := readyManager(t)
	m.memberDebounce = time.Millisecond
	ctx := socket.WithClient(context.Background(), newFakeClient())

	subscribe(t, m, ctx, 1, chGeneral)
	dispatch(n, listUpdate(gateway.GuildMemberListOp{Op: "SYNC", Range: [2]int{0, 99}, Items: []gateway.GuildMemberListOpItem{
		groupItem("online", 2), listItem(ada, "", discord.OnlineStatus), listItem(bob, "", discord.IdleStatus),
		groupItem("offline", 1), listItem(lin, "", discord.OfflineStatus),
	}}))
	if _, ev := memberList(t, m); ev.ChannelID != cid(chGeneral) || len(ev.Members) != 3 {
		t.Fatalf("first list: %+v", ev)
	}

	subscribe(t, m, ctx, 2, chDev)
	r, ev := memberList(t, m)
	if ev.ChannelID != cid(chDev) || len(ev.Members) != 3 || len(r.Members) != 1 || r.Members[0] != cid(chDev) {
		t.Fatalf("shared list re-emit: %+v %+v", r, ev)
	}
	if got := m.memberRanges(n.Offline(), guildOmar); len(got) != 2 || len(got[chGeneral]) != 1 || len(got[chDev]) != 1 {
		t.Fatalf("both channels must be in the Op 14: %+v", got)
	}
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
	if got := m.memberRanges(off, guildOmar); len(got) != 1 || len(got[chTeam]) != 1 {
		t.Fatalf("thread must subscribe its parent: %+v", got)
	}
	dispatch(n, listUpdate(gateway.GuildMemberListOp{Op: "SYNC", Range: [2]int{0, 99}, Items: []gateway.GuildMemberListOpItem{
		groupItem("online", 1), listItem(ada, "", discord.OnlineStatus)}}))
	noEvent(t, m)
	ev := listUpdate(gateway.GuildMemberListOp{Op: "SYNC", Range: [2]int{0, 99}, Items: []gateway.GuildMemberListOpItem{
		groupItem("online", 1), listItem(bob, "", discord.OnlineStatus)}})
	ev.ID = teamList
	dispatch(n, ev)
	_, got := memberList(t, m)
	if got.ChannelID != cid(thTeam) || len(got.Members) != 1 || got.Members[0].User.Username != "bob" {
		t.Fatalf("thread list: %+v", got)
	}
}

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
	dispatch(n, listUpdate(gateway.GuildMemberListOp{Op: "UPDATE", Index: 1, Item: listItem(ada, "", discord.IdleStatus)}))
	memberList(t, m)

	m.ClientClosed(c2)
	if len(m.members.subscribed()) != 0 || len(m.trackedUsers()) != 0 {
		t.Fatalf("state left behind: subs=%v users=%v", m.members.subscribed(), m.trackedUsers())
	}
	if got := m.memberRanges(n.Offline(), guildOmar); len(got) != 0 {
		t.Fatalf("channel still in the Op 14: %+v", got)
	}
	dispatch(n, listUpdate(gateway.GuildMemberListOp{Op: "UPDATE", Index: 1, Item: listItem(ada, "", discord.OnlineStatus)}))
	noEvent(t, m)
	dispatch(n, &gateway.PresenceUpdateEvent{Presence: discord.Presence{User: discord.User{ID: bobID}, GuildID: guildOmar, Status: discord.IdleStatus}})
	noEvent(t, m)
}

func TestMemberListsReplayedAfterReady(t *testing.T) {
	m, n := readyManager(t)
	m.memberDebounce = time.Millisecond
	ctx := socket.WithClient(context.Background(), newFakeClient())

	subscribe(t, m, ctx, 1, chGeneral)
	subscribe(t, m, ctx, 2, dmGroup)
	memberList(t, m)
	dispatch(n, listUpdate(gateway.GuildMemberListOp{Op: "SYNC", Range: [2]int{0, 99}, Items: []gateway.GuildMemberListOpItem{
		groupItem("online", 1), listItem(ada, "", discord.OnlineStatus)}}))
	memberList(t, m)

	_, ready := newUnopenedState(t)
	dispatch(n, ready)
	drain(m)

	if _, stale := m.trackedUsers()[chGeneral]; stale {
		t.Fatalf("stale user set after READY: %v", m.trackedUsers())
	}
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

func TestSyncHandlerPanicIsContained(t *testing.T) {
	m, n := readyManager(t)
	addSyncHandler(n, "test_panic", func(*gateway.TypingStartEvent) { panic("boom") })
	dispatch(n, &gateway.TypingStartEvent{ChannelID: chGeneral, GuildID: guildOmar, UserID: adaID, Timestamp: discord.UnixTimestamp(time.Now().Unix())})
	if _, e := m.Handle(context.Background(), req(t, `{"v":1,"id":1,"command":"list_guilds"}`)); e != nil {
		t.Fatalf("session died with the handler: %v", e)
	}
}

func (m *Manager) trackedUsers() map[discord.ChannelID]int {
	m.members.mu.Lock()
	defer m.members.mu.Unlock()
	out := map[discord.ChannelID]int{}
	for chID, users := range m.members.users {
		out[chID] = len(users)
	}
	return out
}
