package session

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/ningen/v3"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/socket"
)

const (
	thRelease  = discord.ChannelID(310000000000000001)
	thArchived = discord.ChannelID(310000000000000002)
	thDev      = discord.ChannelID(310000000000000003)
	bobID      = discord.UserID(100000000000000004)
)

func thread(id, parent discord.ChannelID, name string, last discord.MessageID, arch bool) discord.Channel {
	return discord.Channel{
		ID: id, GuildID: guildOmar, Type: discord.GuildPublicThread, Name: name, ParentID: parent,
		LastMessageID: last, MessageCount: 7, MemberCount: 3,
		ThreadMetadata: &discord.ThreadMetadata{Archived: arch},
	}
}

// channelUpdate returns the next channel_update event, skipping unrelated
// async events.
func channelUpdate(t *testing.T, m *Manager) protocol.ChannelUpdateEvent {
	t.Helper()
	for {
		switch ev := nextEvent(t, m).(type) {
		case protocol.ChannelUpdateEvent:
			return ev
		case protocol.ReadStateChangedEvent, protocol.StateChangedEvent:
		default:
			t.Fatalf("want channel_update, got %#v", ev)
		}
	}
}

// addThreads injects the test threads through ThreadCreate events and drains
// the channel_update events they produce.
func addThreads(t *testing.T, m *Manager, n *ningen.State) {
	t.Helper()
	for _, th := range []discord.Channel{
		thread(thRelease, chGeneral, "release-planning", 500000000000000015, false),
		thread(thArchived, chGeneral, "old-stuff", 500000000000000011, true),
		thread(thDev, chDev, "devthread", 500000000000000021, false),
	} {
		dispatch(n, &gateway.ThreadCreateEvent{Channel: th})
		if ev := channelUpdate(t, m); ev.Change != protocol.ChannelChangeCreate || ev.Channel.ID != th.ID.String() || ev.Channel.Type != "thread" {
			t.Fatalf("thread create event: %+v", ev)
		}
	}
}

func TestFuzzyScore(t *testing.T) {
	if fuzzyScore("gen", "general") <= 0 || fuzzyScore("xyz", "general") != 0 || fuzzyScore("", "general") != 0 {
		t.Fatal("basic match/no-match")
	}
	if fuzzyScore("GEN", "General") != fuzzyScore("gen", "general") {
		t.Fatal("case-insensitive")
	}
	if fuzzyScore("dev", "dev") <= fuzzyScore("dev", "devops-chat") {
		t.Fatal("exact should beat longer")
	}
	if fuzzyScore("gen", "general") <= fuzzyScore("gen", "xgxexn") {
		t.Fatal("consecutive should beat gapped")
	}
	if fuzzyScore("rp", "release-planning") <= fuzzyScore("rp", "scraper") {
		t.Fatal("word starts should beat mid-word")
	}
	if fuzzyScore("gen", "general") != fuzzyScore("gen", "general") {
		t.Fatal("deterministic")
	}
}

func TestQuickSwitch(t *testing.T) {
	m, n := readyManager(t)
	addThreads(t, m, n)
	fillCache(m, n, chGeneral, guildOmar, 1, 1)
	drain(m)
	call := func(line string) protocol.QuickSwitchResult {
		t.Helper()
		res, e := m.Handle(context.Background(), req(t, line))
		if e != nil {
			t.Fatalf("%s: %v", line, e)
		}
		return res.(protocol.QuickSwitchResult)
	}
	names := func(r protocol.QuickSwitchResult) []string {
		var out []string
		for _, e := range r.Entries {
			out = append(out, e.Channel.Name)
		}
		return out
	}

	// Exact-ish query: only general matches "gen" (the category is excluded).
	r := call(`{"v":1,"id":1,"command":"quick_switch","query":"gen"}`)
	if got := names(r); len(got) != 1 || got[0] != "general" {
		t.Fatalf("gen: %v", got)
	}
	e := r.Entries[0]
	if e.GuildName == nil || *e.GuildName != "Omarchy" || e.LastMessagePreview != "m1" || e.Score <= 0 || e.Channel.Type != "text" {
		t.Fatalf("gen entry: %+v", e)
	}

	// Unread first: "e" matches dev (mentioned), loose (unread), then read
	// channels by score; the archived thread and voice never appear.
	r = call(`{"v":1,"id":2,"command":"quick_switch","query":"e"}`)
	got := names(r)
	if len(got) < 4 || got[0] != "dev" || got[1] != "loose" {
		t.Fatalf("unread-first: %v", got)
	}
	for _, nm := range got {
		if nm == "old-stuff" || nm == "Voice" || nm == "General" || nm == "Empty" || nm == "secret" {
			t.Fatalf("excluded entry %q in %v", nm, got)
		}
	}
	if len(names(call(`{"v":1,"id":3,"command":"quick_switch","query":"voice"}`))) != 0 {
		t.Fatal("voice channels must be excluded")
	}
	// Threads are candidates.
	if got := names(call(`{"v":1,"id":4,"command":"quick_switch","query":"release"}`)); len(got) != 1 || got[0] != "release-planning" {
		t.Fatalf("thread: %v", got)
	}
	// Guild name is a secondary key at half weight.
	r = call(`{"v":1,"id":5,"command":"quick_switch","query":"quiet"}`)
	if got := names(r); len(got) != 1 || got[0] != "chat" || r.Entries[0].GuildName == nil || *r.Entries[0].GuildName != "Quiet" {
		t.Fatalf("guild name: %v", got)
	}
	// DMs match by recipient name and carry a null guild name.
	r = call(`{"v":1,"id":6,"command":"quick_switch","query":"ada"}`)
	if got := names(r); len(got) != 2 || got[0] != "Ada" || got[1] != "Ada, lin" || r.Entries[0].GuildName != nil || r.Entries[0].LastMessagePreview != "" {
		t.Fatalf("dm: %v %+v", got, r.Entries)
	}
	// Empty query: mentioned (Ada's DM, dev), then unread (general — the
	// cached message made it unread — chat, loose), then the rest, each
	// tier by last message desc; score 0 throughout.
	r = call(`{"v":1,"id":7,"command":"quick_switch","query":""}`)
	got = names(r)
	want := []string{"Ada", "dev", "general", "chat", "loose", "Ada, lin", "devthread", "release-planning"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("empty query: %v, want %v", got, want)
	}
	for _, e := range r.Entries {
		if e.Score != 0 {
			t.Fatalf("empty query score: %+v", e)
		}
	}
	// Limit.
	if got := names(call(`{"v":1,"id":8,"command":"quick_switch","query":"","limit":2}`)); fmt.Sprint(got) != fmt.Sprint(want[:2]) {
		t.Fatalf("limit: %v", got)
	}
	// No session → not_logged_in.
	if _, e := New(&fakeKeyring{}).Handle(context.Background(), req(t, `{"v":1,"id":9,"command":"quick_switch","query":"x"}`)); e == nil || e.Code != protocol.CodeNotLoggedIn {
		t.Fatalf("not logged in: %v", e)
	}
}

func TestListThreadsOpenAndEvents(t *testing.T) {
	m, n := readyManager(t)
	addThreads(t, m, n)
	m.fetchTail = func(ctx context.Context, n *ningen.State, chID discord.ChannelID, limit uint) ([]discord.Message, error) {
		return nil, nil
	}
	client := newFakeClient()
	ctx := socket.WithClient(context.Background(), client)

	res, e := m.Handle(ctx, req(t, `{"v":1,"id":1,"command":"list_threads","channel_id":"300000000000000002"}`))
	if e != nil {
		t.Fatal(e)
	}
	ths := res.(protocol.ListThreadsResult).Threads
	if len(ths) != 1 || ths[0].ID != thRelease.String() || ths[0].Type != "thread" || ths[0].MessageCount != 7 || ths[0].MemberCount != 3 || ths[0].ParentID == nil || *ths[0].ParentID != "300000000000000002" {
		t.Fatalf("threads: %+v", ths)
	}
	res, _ = m.Handle(ctx, req(t, `{"v":1,"id":2,"command":"list_threads","channel_id":"300000000000000004"}`))
	if len(res.(protocol.ListThreadsResult).Threads) != 0 {
		t.Fatal("voice channel has no threads")
	}
	if _, e := m.Handle(ctx, req(t, `{"v":1,"id":3,"command":"list_threads","channel_id":"999"}`)); e == nil || e.Code != protocol.CodeUnknownChannel {
		t.Fatalf("unknown: %v", e)
	}
	if _, e := m.Handle(ctx, req(t, `{"v":1,"id":4,"command":"list_threads","channel_id":"nope"}`)); e == nil || e.Code != protocol.CodeInvalidArgument {
		t.Fatalf("invalid: %v", e)
	}

	// open_channel on a cached thread.
	res, e = m.Handle(ctx, req(t, `{"v":1,"id":5,"command":"open_channel","channel_id":"310000000000000001"}`))
	if e != nil || res.(protocol.OpenChannelResult).Channel.Type != "thread" || !client.HasOpen("310000000000000001") {
		t.Fatalf("open thread: %v %+v", e, res)
	}

	// Uncached thread: one REST lookup, then cached.
	var fetches int
	m.fetchChannel = func(ctx context.Context, n *ningen.State, chID discord.ChannelID) (*discord.Channel, error) {
		fetches++
		switch chID {
		case 310000000000000009:
			th := thread(chID, chGeneral, "late", 500000000000000016, false)
			return &th, nil
		case 310000000000000008:
			ch := discord.Channel{ID: chID, GuildID: guildOmar, Type: discord.GuildText, Name: "not-a-thread"}
			return &ch, nil
		}
		return nil, errors.New("404")
	}
	if _, e := m.Handle(ctx, req(t, `{"v":1,"id":6,"command":"open_channel","channel_id":"310000000000000009"}`)); e != nil {
		t.Fatalf("open uncached thread: %v", e)
	}
	if _, err := n.Cabinet.Channel(310000000000000009); err != nil {
		t.Fatal("fetched thread should be cached")
	}
	if _, e := m.Handle(ctx, req(t, `{"v":1,"id":7,"command":"open_channel","channel_id":"310000000000000007"}`)); e == nil || e.Code != protocol.CodeUnknownChannel {
		t.Fatalf("404 thread: %v", e)
	}
	if _, e := m.Handle(ctx, req(t, `{"v":1,"id":8,"command":"open_channel","channel_id":"310000000000000008"}`)); e == nil || e.Code != protocol.CodeUnknownChannel {
		t.Fatalf("non-thread from REST: %v", e)
	}
	if fetches != 3 {
		t.Fatalf("fetches %d", fetches)
	}

	// Thread update / delete events.
	upd := thread(thRelease, chGeneral, "release-planning-v2", 500000000000000017, false)
	dispatch(n, &gateway.ThreadUpdateEvent{Channel: upd})
	if ev := channelUpdate(t, m); ev.Change != protocol.ChannelChangeUpdate || ev.Channel.Name != "release-planning-v2" {
		t.Fatalf("update: %+v", ev)
	}
	dispatch(n, &gateway.ThreadDeleteEvent{ID: thRelease, GuildID: guildOmar, Type: discord.GuildPublicThread, ParentID: chGeneral})
	if ev := channelUpdate(t, m); ev.Change != protocol.ChannelChangeDelete || ev.Channel.ID != thRelease.String() || ev.Channel.Type != "thread" || ev.Channel.GuildID == nil {
		t.Fatalf("delete: %+v", ev)
	}
	res, _ = m.Handle(ctx, req(t, `{"v":1,"id":9,"command":"list_threads","channel_id":"300000000000000002"}`))
	if ths := res.(protocol.ListThreadsResult).Threads; len(ths) != 1 || ths[0].Name != "late" {
		t.Fatalf("after delete (only the REST-fetched thread remains): %+v", ths)
	}
	// Plain channel events too.
	dispatch(n, &gateway.ChannelCreateEvent{Channel: discord.Channel{ID: 300000000000000099, GuildID: guildOmar, Type: discord.GuildText, Name: "new"}})
	if ev := channelUpdate(t, m); ev.Change != protocol.ChannelChangeCreate || ev.Channel.Name != "new" {
		t.Fatalf("channel create: %+v", ev)
	}
}

// Synthetic GUILD_MEMBER_LIST_UPDATE payloads for #general's "everyone" list.
func listItem(u discord.User, nick string, status discord.Status, acts ...discord.Activity) gateway.GuildMemberListOpItem {
	var it gateway.GuildMemberListOpItem
	it.Member = &struct {
		discord.Member
		HoistedRole string           `json:"hoisted_role"`
		Presence    discord.Presence `json:"presence"`
	}{
		Member:   discord.Member{User: u, Nick: nick},
		Presence: discord.Presence{User: discord.User{ID: u.ID}, Status: status, Activities: acts},
	}
	return it
}

func groupItem(id string, count uint64) gateway.GuildMemberListOpItem {
	return gateway.GuildMemberListOpItem{Group: &gateway.GuildMemberListGroup{ID: id, Count: count}}
}

func listUpdate(ops ...gateway.GuildMemberListOp) *gateway.GuildMemberListUpdateEvent {
	return &gateway.GuildMemberListUpdateEvent{
		ID: "everyone", GuildID: guildOmar, MemberCount: 3, OnlineCount: 2,
		Groups: []gateway.GuildMemberListGroup{{ID: "online", Count: 2}, {ID: "offline", Count: 1}},
		Ops:    ops,
	}
}

// memberList returns the next member_list_update routed event.
func memberList(t *testing.T, m *Manager) (socket.Routed, protocol.MemberListUpdateEvent) {
	t.Helper()
	for {
		r := routed(t, m)
		if ev, ok := r.Event.(protocol.MemberListUpdateEvent); ok {
			return r, ev
		}
	}
}

var (
	ada = discord.User{ID: adaID, Username: "ada", DisplayName: "Ada", Avatar: "aaaa"}
	lin = discord.User{ID: linID, Username: "lin", Bot: true}
	bob = discord.User{ID: bobID, Username: "bob"}
)

func TestMemberListOps(t *testing.T) {
	m, n := readyManager(t)
	m.memberDebounce = time.Millisecond
	client := newFakeClient()
	ctx := socket.WithClient(context.Background(), client)

	// Subscribe before any list exists: ok, subscribed, nothing emitted yet.
	if _, e := m.Handle(ctx, req(t, `{"v":1,"id":1,"command":"subscribe_members","channel_id":"300000000000000002"}`)); e != nil {
		t.Fatal(e)
	}
	if !client.HasMemberSub("300000000000000002") || n.MemberState.GetMemberListChunk(guildOmar, chGeneral) != 0 {
		t.Fatal("subscription not registered / chunk not requested")
	}
	noEvent(t, m)
	// Secret channel: no view permission.
	if _, e := m.Handle(ctx, req(t, `{"v":1,"id":2,"command":"subscribe_members","channel_id":"300000000000000005"}`)); e == nil || e.Code != protocol.CodeForbidden {
		t.Fatalf("secret: %v", e)
	}
	if _, e := m.Handle(ctx, req(t, `{"v":1,"id":3,"command":"subscribe_members","channel_id":"999"}`)); e == nil || e.Code != protocol.CodeUnknownChannel {
		t.Fatalf("unknown: %v", e)
	}

	// SYNC: full first chunk.
	self := discord.User{ID: selfID, Username: "tester", DisplayName: "Tester"}
	dispatch(n, listUpdate(gateway.GuildMemberListOp{Op: "SYNC", Range: [2]int{0, 99}, Items: []gateway.GuildMemberListOpItem{
		groupItem("online", 2),
		listItem(ada, "ada-nick", discord.OnlineStatus, discord.Activity{Type: discord.GameActivity, Name: "Factorio"}),
		listItem(self, "", discord.DoNotDisturbStatus),
		groupItem("offline", 1),
		listItem(lin, "", discord.OfflineStatus),
	}}))
	r, ev := memberList(t, m)
	if r.All || r.ChannelID != "" || len(r.Members) != 1 || r.Members[0] != "300000000000000002" {
		t.Fatalf("routing: %+v", r)
	}
	if ev.ChannelID != "300000000000000002" || ev.GuildID == nil || *ev.GuildID != "200000000000000001" {
		t.Fatalf("ids: %+v", ev)
	}
	if len(ev.Groups) != 2 || ev.Groups[0].ID != "online" || ev.Groups[0].Name != "Online" || ev.Groups[0].Count != 2 || ev.Groups[1].Name != "Offline" {
		t.Fatalf("groups: %+v", ev.Groups)
	}
	if len(ev.Members) != 3 {
		t.Fatalf("members: %+v", ev.Members)
	}
	a := ev.Members[0]
	if a.User.ID != "100000000000000002" || a.User.DisplayName != "ada-nick" || a.User.Username != "ada" || a.GroupID != "online" || a.Status != "online" || a.Activity != "Playing Factorio" || a.User.AvatarURL == "" {
		t.Fatalf("ada: %+v", a)
	}
	if s := ev.Members[1]; s.User.DisplayName != "Tester" || s.Status != "dnd" || s.Activity != "" {
		t.Fatalf("self: %+v", s)
	}
	if l := ev.Members[2]; l.GroupID != "offline" || l.Status != "offline" || !l.User.Bot {
		t.Fatalf("lin: %+v", l)
	}

	// INSERT bob at index 3 (after self, before the offline group).
	dispatch(n, listUpdate(gateway.GuildMemberListOp{Op: "INSERT", Index: 3, Item: listItem(bob, "", discord.IdleStatus)}))
	_, ev = memberList(t, m)
	if len(ev.Members) != 4 || ev.Members[2].User.Username != "bob" || ev.Members[2].GroupID != "online" || ev.Members[2].Status != "idle" {
		t.Fatalf("insert: %+v", ev.Members)
	}
	// UPDATE ada → idle with a custom status.
	dispatch(n, listUpdate(gateway.GuildMemberListOp{Op: "UPDATE", Index: 1, Item: listItem(ada, "ada-nick", discord.IdleStatus,
		discord.Activity{Type: discord.CustomActivity, State: "sleepy", Emoji: &discord.Emoji{Name: "🌙"}})}))
	_, ev = memberList(t, m)
	if ev.Members[0].Status != "idle" || ev.Members[0].Activity != "🌙 sleepy" {
		t.Fatalf("update: %+v", ev.Members[0])
	}
	// DELETE bob.
	dispatch(n, listUpdate(gateway.GuildMemberListOp{Op: "DELETE", Index: 3}))
	_, ev = memberList(t, m)
	if len(ev.Members) != 3 || ev.Members[2].User.Username != "lin" {
		t.Fatalf("delete: %+v", ev.Members)
	}
	// Debounce: two ops in a burst produce one event.
	dispatch(n, listUpdate(gateway.GuildMemberListOp{Op: "UPDATE", Index: 1, Item: listItem(ada, "", discord.OnlineStatus)}))
	dispatch(n, listUpdate(gateway.GuildMemberListOp{Op: "UPDATE", Index: 1, Item: listItem(ada, "", discord.DoNotDisturbStatus)}))
	_, ev = memberList(t, m)
	if ev.Members[0].Status != "dnd" || ev.Members[0].User.DisplayName != "Ada" {
		t.Fatalf("burst: %+v", ev.Members[0])
	}
	noEvent(t, m)
	// INVALIDATE empties the range; groups survive.
	dispatch(n, listUpdate(gateway.GuildMemberListOp{Op: "INVALIDATE", Range: [2]int{0, 99}}))
	_, ev = memberList(t, m)
	if len(ev.Members) != 0 || len(ev.Groups) != 2 {
		t.Fatalf("invalidate: %+v", ev)
	}

	// A list for another list id (secret channel) is never emitted for
	// #general, and #secret has no subscriber request.
	other := listUpdate(gateway.GuildMemberListOp{Op: "SYNC", Range: [2]int{0, 99}, Items: []gateway.GuildMemberListOpItem{groupItem("online", 1), listItem(ada, "", discord.OnlineStatus)}})
	other.ID = "deadbeef"
	dispatch(n, other)
	noEvent(t, m)

	// Re-subscribing (another client) re-emits ningen's kept list at once.
	dispatch(n, listUpdate(gateway.GuildMemberListOp{Op: "SYNC", Range: [2]int{0, 99}, Items: []gateway.GuildMemberListOpItem{groupItem("online", 1), listItem(ada, "", discord.OnlineStatus)}}))
	memberList(t, m)
	c2 := newFakeClient()
	if _, e := m.Handle(socket.WithClient(context.Background(), c2), req(t, `{"v":1,"id":4,"command":"subscribe_members","channel_id":"300000000000000002"}`)); e != nil {
		t.Fatal(e)
	}
	if _, ev = memberList(t, m); len(ev.Members) != 1 {
		t.Fatalf("resubscribe: %+v", ev)
	}
	// Unsubscribe is idempotent.
	for i := 0; i < 2; i++ {
		if _, e := m.Handle(ctx, req(t, `{"v":1,"id":5,"command":"unsubscribe_members","channel_id":"300000000000000002"}`)); e != nil {
			t.Fatal(e)
		}
	}
	if client.HasMemberSub("300000000000000002") {
		t.Fatal("still subscribed")
	}
}

func TestDMMemberListAndPresenceRouting(t *testing.T) {
	m, n := readyManager(t)
	m.memberDebounce = time.Millisecond
	client := newFakeClient()
	ctx := socket.WithClient(context.Background(), client)

	// Ada is online globally (friend presence, guild 0). She is a DM
	// recipient, so the event is routed to her DMs (open-set keys) — the
	// socket layer drops it when nobody has them open.
	dispatch(n, &gateway.PresenceUpdateEvent{Presence: discord.Presence{User: discord.User{ID: adaID}, Status: discord.IdleStatus,
		Activities: []discord.Activity{{Type: discord.ListeningActivity, Name: "Spotify"}}}})
	if r := routed(t, m); len(r.Open) != 2 || len(r.Members) != 0 || r.Event.(protocol.PresenceUpdateEvent).Activity != "Listening to Spotify" {
		t.Fatalf("friend presence: %+v", r)
	}
	// A user in no DM and no list produces nothing.
	dispatch(n, &gateway.PresenceUpdateEvent{Presence: discord.Presence{User: discord.User{ID: bobID}, Status: discord.IdleStatus}})
	noEvent(t, m)

	// Group DM: synthesized from recipients + presence store, no gateway.
	if _, e := m.Handle(ctx, req(t, `{"v":1,"id":1,"command":"subscribe_members","channel_id":"400000000000000002"}`)); e != nil {
		t.Fatal(e)
	}
	r, ev := memberList(t, m)
	if r.Members[0] != "400000000000000002" || ev.GuildID != nil || len(ev.Members) != 2 {
		t.Fatalf("dm list: %+v %+v", r, ev)
	}
	if len(ev.Groups) != 2 || ev.Groups[0].ID != "online" || ev.Groups[0].Count != 1 || ev.Groups[1].ID != "offline" || ev.Groups[1].Count != 1 {
		t.Fatalf("dm groups: %+v", ev.Groups)
	}
	if a := ev.Members[0]; a.User.Username != "ada" || a.GroupID != "online" || a.Status != "idle" || a.Activity != "Listening to Spotify" {
		t.Fatalf("ada: %+v", a)
	}
	if l := ev.Members[1]; l.User.Username != "lin" || l.GroupID != "offline" || l.Status != "offline" {
		t.Fatalf("lin: %+v", l)
	}
	if n.MemberState.GetMemberListChunk(0, dmGroup) != -1 {
		t.Fatal("DM subscribe must not request a member list")
	}

	// Presence for ada now routes to her DMs (open-set keys) and the group
	// list (member-sub key); nothing for an unknown user.
	dispatch(n, &gateway.PresenceUpdateEvent{Presence: discord.Presence{User: discord.User{ID: adaID}, GuildID: guildOmar, Status: discord.DoNotDisturbStatus}})
	r = routed(t, m)
	pev, ok := r.Event.(protocol.PresenceUpdateEvent)
	if !ok || pev.UserID != "100000000000000002" || pev.Status != "dnd" || pev.Activity != "" {
		t.Fatalf("presence: %+v", r.Event)
	}
	if fmt.Sprint(r.Open) != fmt.Sprint([]string{"400000000000000002", "400000000000000001"}) && fmt.Sprint(r.Open) != fmt.Sprint([]string{"400000000000000001", "400000000000000002"}) {
		t.Fatalf("presence dm keys: %v", r.Open)
	}
	if fmt.Sprint(r.Members) != fmt.Sprint([]string{"400000000000000002"}) || r.All || r.ChannelID != "" {
		t.Fatalf("presence member keys: %+v", r)
	}
	dispatch(n, &gateway.PresenceUpdateEvent{Presence: discord.Presence{User: discord.User{ID: 999}, GuildID: guildOmar, Status: discord.OnlineStatus}})
	noEvent(t, m)

	// Guild list membership extends routing: subscribe #general, sync bob
	// in; bob's presence then routes to #general only.
	if _, e := m.Handle(ctx, req(t, `{"v":1,"id":2,"command":"subscribe_members","channel_id":"300000000000000002"}`)); e != nil {
		t.Fatal(e)
	}
	dispatch(n, listUpdate(gateway.GuildMemberListOp{Op: "SYNC", Range: [2]int{0, 99}, Items: []gateway.GuildMemberListOpItem{groupItem("online", 1), listItem(bob, "", discord.OnlineStatus)}}))
	memberList(t, m)
	dispatch(n, &gateway.PresenceUpdateEvent{Presence: discord.Presence{User: discord.User{ID: bobID}, GuildID: guildOmar, Status: discord.IdleStatus}})
	r = routed(t, m)
	if _, ok := r.Event.(protocol.PresenceUpdateEvent); !ok || len(r.Open) != 0 || fmt.Sprint(r.Members) != fmt.Sprint([]string{"300000000000000002"}) {
		t.Fatalf("bob presence: %+v", r)
	}
}

func TestListEmoji(t *testing.T) {
	m, n := readyManager(t)
	role := discord.RoleID(250000000000000001)
	dispatch(n, &gateway.GuildEmojisUpdateEvent{GuildID: guildOmar, Emojis: []discord.Emoji{
		{ID: 800000000000000001, Name: "omarchy", Available: true},
		{ID: 800000000000000002, Name: "partyblob", Animated: true, Available: true},
		{ID: 800000000000000003, Name: "gone", Available: false},
		{ID: 800000000000000004, Name: "vip", Available: true, RoleIDs: []discord.RoleID{role}},
	}})
	res, e := m.Handle(context.Background(), req(t, `{"v":1,"id":1,"command":"list_emoji"}`))
	if e != nil {
		t.Fatal(e)
	}
	gs := res.(protocol.ListEmojiResult).Guilds
	if len(gs) != 1 || gs[0].GuildID != "200000000000000001" || gs[0].GuildName != "Omarchy" {
		t.Fatalf("guilds: %+v", gs)
	}
	em := gs[0].Emoji
	if len(em) != 2 || em[0].Name != "omarchy" || em[0].Animated || em[0].URL != "https://cdn.discordapp.com/emojis/800000000000000001.png" ||
		em[1].Name != "partyblob" || !em[1].Animated || em[1].URL != "https://cdn.discordapp.com/emojis/800000000000000002.gif" {
		t.Fatalf("emoji: %+v", em)
	}
	// Gaining the role makes the restricted emoji usable.
	n.Cabinet.MemberSet(guildOmar, &discord.Member{User: discord.User{ID: selfID, Username: "tester"}, RoleIDs: []discord.RoleID{role}}, true)
	res, _ = m.Handle(context.Background(), req(t, `{"v":1,"id":2,"command":"list_emoji"}`))
	if em := res.(protocol.ListEmojiResult).Guilds[0].Emoji; len(em) != 3 || em[2].Name != "vip" {
		t.Fatalf("role emoji: %+v", em)
	}
	if _, e := New(&fakeKeyring{}).Handle(context.Background(), req(t, `{"v":1,"id":3,"command":"list_emoji"}`)); e == nil || e.Code != protocol.CodeNotLoggedIn {
		t.Fatalf("not logged in: %v", e)
	}
}
