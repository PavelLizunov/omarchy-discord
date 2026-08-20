package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/ningen/v3"
	"github.com/diamondburned/ningen/v3/states/read"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/socket"
)

// fakeClient stands in for the socket connection's open-channel set.
type fakeClient struct {
	mu   sync.Mutex
	open map[string]bool
}

func newFakeClient() *fakeClient { return &fakeClient{open: map[string]bool{}} }
func (f *fakeClient) OpenChannel(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.open[id] = true
}
func (f *fakeClient) CloseChannel(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	ok := f.open[id]
	delete(f.open, id)
	return ok
}
func (f *fakeClient) HasOpen(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.open[id]
}

const (
	msgBase = discord.MessageID(600000000000000000)
)

func ts(i int) discord.Timestamp {
	return discord.Timestamp(time.Date(2026, 8, 20, 14, 0, i, 117_000_000, time.UTC))
}

// guildMsg builds a default message from ada in #general.
func guildMsg(i int, content string) discord.Message {
	return discord.Message{
		ID: msgBase + discord.MessageID(i), ChannelID: chGeneral, GuildID: guildOmar, Type: discord.DefaultMessage,
		Author: discord.User{ID: adaID, Username: "ada", DisplayName: "Ada", Avatar: "aaaa"}, Content: content, Timestamp: ts(i),
	}
}

// readyManager returns a manager attached to the fixture session in the ready
// state with the snapshot events drained.
func readyManager(t *testing.T) (*Manager, *ningen.State) {
	t.Helper()
	m := New(&fakeKeyring{})
	n, ready := newUnopenedState(t)
	m.mu.Lock()
	m.n, m.token = n, "tok"
	m.installHandlers(n)
	m.setLifecycleLocked(protocol.LifecycleConnecting, "")
	m.mu.Unlock()
	nextEvent(t, m) // connecting
	dispatch(n, ready)
	nextEvent(t, m) // ready
	nextEvent(t, m) // guilds_synced
	return m, n
}

func noEvent(t *testing.T, m *Manager) {
	t.Helper()
	select {
	case ev := <-m.Events():
		t.Fatalf("unexpected event %#v", ev)
	case <-time.After(50 * time.Millisecond):
	}
}

// routed returns the next routed event, skipping read_state/state events
// (ningen's read.UpdateEvent fires on its own goroutine, so their order
// relative to message events is undefined).
func routed(t *testing.T, m *Manager) socket.Routed {
	t.Helper()
	for {
		ev := nextEvent(t, m)
		switch ev := ev.(type) {
		case socket.Routed:
			return ev
		case protocol.ReadStateChangedEvent, protocol.StateChangedEvent:
			continue
		default:
			t.Fatalf("want routed event, got %#v", ev)
		}
	}
}

func TestWireMessageMapping(t *testing.T) {
	n := loadOfflineState(t)
	// Put the guild member with a nick into the cache so nick > display name.
	n.Cabinet.MemberSet(guildOmar, &discord.Member{User: discord.User{ID: adaID, Username: "ada", DisplayName: "Ada"}, Nick: "ada-nick"}, false)

	original := guildMsg(1, "first  line\nsecond")
	n.Cabinet.MessageSet(&original, false)

	edited := ts(9)
	msg := guildMsg(2, "hey <@100000000000000001>")
	msg.Type = discord.InlinedReplyMessage
	msg.EditedTimestamp = edited
	msg.Nonce = "abc123"
	msg.Mentions = []discord.GuildUser{{User: discord.User{ID: selfID, Username: "tester"}}}
	msg.Reference = &discord.MessageReference{MessageID: original.ID, ChannelID: chGeneral, GuildID: guildOmar}
	msg.Attachments = []discord.Attachment{{ID: 700000000000000001, Filename: "SPOILER_shot.png", ContentType: "image/png", Size: 1234, URL: "https://cdn.discordapp.com/attachments/1/2/SPOILER_shot.png", Proxy: "https://media.discordapp.net/attachments/1/2/SPOILER_shot.png", Width: 800, Height: 600}}
	msg.Embeds = []discord.Embed{{Type: discord.LinkEmbed, Title: "T", Description: "D", URL: "https://example.com", Color: 0x00ff00, Thumbnail: &discord.EmbedThumbnail{URL: "https://example.com/t.png"}}}
	msg.Reactions = []discord.Reaction{
		{Count: 2, Me: true, Emoji: discord.Emoji{Name: "👍"}},
		{Count: 1, Emoji: discord.Emoji{ID: 800000000000000001, Name: "omarchy"}},
	}

	w := WireMessage(n, &msg)
	b, _ := json.Marshal(w)
	var raw map[string]any
	json.Unmarshal(b, &raw)
	if raw["id"] != "600000000000000002" || raw["channel_id"] != "300000000000000002" || raw["guild_id"] != "200000000000000001" {
		t.Fatalf("ids must be strings: %s", b)
	}
	if w.Author.DisplayName != "ada-nick" || w.Author.Username != "ada" || w.Author.AvatarURL == "" || w.Author.Bot {
		t.Errorf("author: %+v", w.Author)
	}
	if w.Author.AvatarURL[len(w.Author.AvatarURL)-8:] != "?size=64" {
		t.Errorf("avatar size hint: %s", w.Author.AvatarURL)
	}
	if w.Timestamp != "2026-08-20T14:00:02.117Z" || w.EditedTimestamp == nil || *w.EditedTimestamp != "2026-08-20T14:00:09.117Z" {
		t.Errorf("timestamps: %s %v", w.Timestamp, w.EditedTimestamp)
	}
	if _, err := time.Parse(time.RFC3339Nano, w.Timestamp); err != nil {
		t.Error(err)
	}
	if w.Nonce != "abc123" || w.System || !w.MentionsSelf {
		t.Errorf("flags: %+v", w)
	}
	if w.ReplyTo == nil || w.ReplyTo.MessageID != "600000000000000001" || w.ReplyTo.AuthorDisplayName != "ada-nick" || w.ReplyTo.Preview != "first line second" {
		t.Errorf("reply_to: %+v", w.ReplyTo)
	}
	if len(w.Attachments) != 1 || !w.Attachments[0].Spoiler || w.Attachments[0].ID != "700000000000000001" || w.Attachments[0].Width != 800 || w.Attachments[0].ContentType != "image/png" || w.Attachments[0].ProxyURL == "" {
		t.Errorf("attachments: %+v", w.Attachments)
	}
	if len(w.Embeds) != 1 || w.Embeds[0].Type != "link" || w.Embeds[0].Color != 0x00ff00 || w.Embeds[0].ThumbnailURL == "" || w.Embeds[0].ImageURL != "" {
		t.Errorf("embeds: %+v", w.Embeds)
	}
	if len(w.Reactions) != 2 || w.Reactions[0].Emoji != "👍" || !w.Reactions[0].Me || w.Reactions[0].Count != 2 || w.Reactions[1].Emoji != "omarchy:800000000000000001" || w.Reactions[1].Me {
		t.Errorf("reactions: %+v", w.Reactions)
	}

	// Unknown reply target: id kept, name/preview empty.
	msg.Reference.MessageID = 600000000000000099
	msg.ReferencedMessage = nil
	if r := WireMessage(n, &msg).ReplyTo; r == nil || r.MessageID != "600000000000000099" || r.Preview != "" || r.AuthorDisplayName != "" {
		t.Errorf("unknown reply: %+v", r)
	}
	// Inline referenced message wins over the cache.
	inline := guildMsg(3, "")
	inline.Attachments = []discord.Attachment{{Filename: "a.txt"}, {Filename: "b.txt"}}
	msg.ReferencedMessage = &inline
	if r := WireMessage(n, &msg).ReplyTo; r.Preview != "a.txt, b.txt" {
		t.Errorf("inline reply preview: %+v", r)
	}

	// Plain message: empty arrays, not null; no mention.
	plain := guildMsg(4, "hello")
	b, _ = json.Marshal(WireMessage(n, &plain))
	for _, key := range []string{`"attachments":[]`, `"embeds":[]`, `"reactions":[]`, `"reply_to":null`, `"edited_timestamp":null`, `"mentions_self":false`, `"nonce":""`} {
		if !contains(b, key) {
			t.Errorf("missing %s in %s", key, b)
		}
	}
	// DM: guild_id null, display name is the global one.
	dm := discord.Message{ID: msgBase + 5, ChannelID: dmAda, Author: discord.User{ID: adaID, Username: "ada", DisplayName: "Ada"}, Content: "hi", Timestamp: ts(5)}
	if w := WireMessage(n, &dm); w.GuildID != nil || w.Author.DisplayName != "Ada" {
		t.Errorf("dm: %+v", w)
	}
}

func contains(b []byte, s string) bool {
	return len(b) > 0 && json.Valid(b) && string(b) != "" && indexOf(string(b), s) >= 0
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}

func TestSystemMessages(t *testing.T) {
	n := loadOfflineState(t)
	ada := discord.User{ID: adaID, Username: "ada", DisplayName: "Ada"}
	lin := discord.User{ID: linID, Username: "lin"}
	cases := []struct {
		typ  discord.MessageType
		mut  func(*discord.Message)
		want string
	}{
		{discord.GuildMemberJoinMessage, nil, "Ada joined the server."},
		{discord.ChannelPinnedMessage, nil, "Ada pinned a message."},
		{discord.NitroBoostMessage, nil, "Ada boosted the server!"},
		{discord.NitroTier2Message, nil, "Ada boosted the server! The server reached Tier 2."},
		{discord.CallMessage, nil, "Ada started a call."},
		{discord.ThreadCreatedMessage, func(m *discord.Message) { m.Content = "bugs" }, "Ada started a thread: bugs."},
		{discord.ChannelNameChangeMessage, func(m *discord.Message) { m.Content = "new-name" }, "Ada changed the channel name to new-name."},
		{discord.RecipientAddMessage, func(m *discord.Message) { m.Mentions = []discord.GuildUser{{User: lin}} }, "Ada added lin to the group."},
		{discord.RecipientRemoveMessage, func(m *discord.Message) { m.Mentions = []discord.GuildUser{{User: ada}} }, "Ada left the group."},
		{discord.MessageType(99), nil, "Ada sent a system message (type 99)."},
	}
	for _, c := range cases {
		msg := discord.Message{ID: msgBase, ChannelID: chGeneral, GuildID: guildOmar, Type: c.typ, Author: ada, Timestamp: ts(1), Content: "raw"}
		if c.mut != nil {
			c.mut(&msg)
		}
		w := WireMessage(n, &msg)
		if !w.System || w.Content != c.want {
			t.Errorf("type %d: system=%v content=%q want %q", c.typ, w.System, w.Content, c.want)
		}
	}
	for _, typ := range []discord.MessageType{discord.DefaultMessage, discord.InlinedReplyMessage, discord.ChatInputCommandMessage} {
		msg := discord.Message{ID: msgBase, ChannelID: chGeneral, Type: typ, Author: ada, Content: "raw", Timestamp: ts(1)}
		if w := WireMessage(n, &msg); w.System || w.Content != "raw" {
			t.Errorf("type %d treated as system: %+v", typ, w)
		}
	}
}

// fillCache dispatches count messages into a channel through the gateway
// path so they land in the cabinet exactly as live traffic would.
func fillCache(m *Manager, n *ningen.State, chID discord.ChannelID, guildID discord.GuildID, from, count int) {
	for i := from; i < from+count; i++ {
		msg := guildMsg(i, fmt.Sprintf("m%d", i))
		msg.ChannelID, msg.GuildID = chID, guildID
		dispatch(n, &gateway.MessageCreateEvent{Message: msg})
	}
}

// drainRouted discards the routed events produced by fillCache plus any
// read_state_changed / state_changed they trigger.
func drain(m *Manager) {
	for {
		select {
		case <-m.Events():
		case <-time.After(30 * time.Millisecond):
			return
		}
	}
}

func TestOpenChannelHistoryAndClose(t *testing.T) {
	m, n := readyManager(t)
	var restTail, restBefore int
	m.fetchTail = func(ctx context.Context, n *ningen.State, chID discord.ChannelID, limit uint) ([]discord.Message, error) {
		restTail++
		return n.Offline().Messages(chID, limit)
	}
	m.fetchBefore = func(ctx context.Context, n *ningen.State, chID discord.ChannelID, before discord.MessageID, limit uint) ([]discord.Message, error) {
		restBefore++
		// Pretend the server holds 10 older messages below the oldest cached one.
		var out []discord.Message
		for i := 1; i <= 10 && len(out) < int(limit); i++ {
			id := before - discord.MessageID(i)
			if id < msgBase {
				break
			}
			mm := guildMsg(0, "old")
			mm.ID = id
			out = append(out, mm)
		}
		return out, nil
	}
	fillCache(m, n, chGeneral, guildOmar, 20, 60) // ids base+20 … base+79
	drain(m)

	client := newFakeClient()
	ctx := socket.WithClient(context.Background(), client)
	call := func(line string) (any, *protocol.Error) { return m.Handle(ctx, req(t, line)) }
	open := `{"v":1,"id":1,"command":"open_channel","channel_id":"300000000000000002"}`

	res, e := call(open)
	if e != nil {
		t.Fatal(e)
	}
	r := res.(protocol.OpenChannelResult)
	if len(r.Messages) != 50 || !r.HasMore || r.Channel.ID != "300000000000000002" || r.Channel.Name != "general" {
		t.Fatalf("open: %d msgs has_more=%v channel=%+v", len(r.Messages), r.HasMore, r.Channel)
	}
	if r.Messages[0].ID >= r.Messages[49].ID || r.Messages[49].ID != (msgBase+79).String() {
		t.Fatalf("messages must ascend to the newest: %s … %s", r.Messages[0].ID, r.Messages[49].ID)
	}
	if !client.HasOpen("300000000000000002") || restTail != 1 {
		t.Fatalf("open bookkeeping: open=%v tail=%d", client.HasOpen("300000000000000002"), restTail)
	}
	// Idempotent reopen.
	if _, e := call(open); e != nil {
		t.Fatal(e)
	}

	// history page 1: the cache holds 10 older messages, fewer than a page, so REST is used.
	res, e = call(`{"v":1,"id":2,"command":"history","channel_id":"300000000000000002","before_id":"` + r.Messages[0].ID + `","limit":10}`)
	if e != nil {
		t.Fatal(e)
	}
	h := res.(protocol.HistoryResult)
	if len(h.Messages) != 10 || !h.HasMore || restBefore != 0 {
		t.Fatalf("history from cache: %d has_more=%v rest=%d", len(h.Messages), h.HasMore, restBefore)
	}
	if h.Messages[9].ID != (msgBase+29).String() || h.Messages[0].ID != (msgBase+20).String() {
		t.Fatalf("history page bounds: %s … %s", h.Messages[0].ID, h.Messages[9].ID)
	}
	// page 2: nothing older is cached → REST, which returns 10 < 50 → start of history.
	res, e = call(`{"v":1,"id":3,"command":"history","channel_id":"300000000000000002","before_id":"` + h.Messages[0].ID + `"}`)
	if e != nil {
		t.Fatal(e)
	}
	h = res.(protocol.HistoryResult)
	if len(h.Messages) != 10 || h.HasMore || restBefore != 1 {
		t.Fatalf("history from rest: %d has_more=%v rest=%d", len(h.Messages), h.HasMore, restBefore)
	}
	if h.Messages[0].GuildID == nil {
		t.Fatalf("guild_id must be filled on REST pages")
	}
	// REST pages must not enter the cache.
	if cached, _ := n.Cabinet.Messages(chGeneral); len(cached) != 60 {
		t.Fatalf("cache polluted: %d", len(cached))
	}
	// limit clamping: 0 → 50, 500 → 100 (served by REST stub which caps at 10 anyway).
	if _, e := call(`{"v":1,"id":4,"command":"history","channel_id":"300000000000000002","before_id":"1","limit":500}`); e != nil {
		t.Fatal(e)
	}

	// Error paths.
	for _, c := range []struct{ line, code string }{
		{`{"v":1,"id":5,"command":"history","channel_id":"300000000000000003","before_id":"1"}`, protocol.CodeChannelNotOpen},
		{`{"v":1,"id":5,"command":"history","channel_id":"300000000000000002"}`, protocol.CodeInvalidArgument},
		{`{"v":1,"id":6,"command":"open_channel","channel_id":"nope"}`, protocol.CodeInvalidArgument},
		{`{"v":1,"id":6,"command":"open_channel","channel_id":"300000000000000099"}`, protocol.CodeUnknownChannel},
		{`{"v":1,"id":6,"command":"open_channel","channel_id":"300000000000000005"}`, protocol.CodeForbidden},
		{`{"v":1,"id":7,"command":"close_channel","channel_id":"300000000000000003"}`, protocol.CodeChannelNotOpen},
	} {
		if _, e := call(c.line); e == nil || e.Code != c.code {
			t.Errorf("%s: got %v want %s", c.line, e, c.code)
		}
	}
	if _, e := call(`{"v":1,"id":8,"command":"close_channel","channel_id":"300000000000000002"}`); e != nil {
		t.Fatal(e)
	}
	if client.HasOpen("300000000000000002") {
		t.Fatal("still open after close")
	}
	if _, e := call(`{"v":1,"id":9,"command":"history","channel_id":"300000000000000002","before_id":"1"}`); e == nil || e.Code != protocol.CodeChannelNotOpen {
		t.Fatalf("history after close: %v", e)
	}
	// Without a socket client (no connection context) close is channel_not_open.
	if _, e := m.Handle(context.Background(), req(t, `{"v":1,"id":9,"command":"close_channel","channel_id":"300000000000000002"}`)); e == nil || e.Code != protocol.CodeChannelNotOpen {
		t.Fatalf("close without client: %v", e)
	}
}

func TestOpenChannelVirginDMRefused(t *testing.T) {
	m, _ := readyManager(t)
	m.fetchTail = func(context.Context, *ningen.State, discord.ChannelID, uint) ([]discord.Message, error) {
		return nil, nil
	}
	ctx := socket.WithClient(context.Background(), newFakeClient())
	_, e := m.Handle(ctx, req(t, `{"v":1,"id":1,"command":"open_channel","channel_id":"400000000000000001"}`))
	if e == nil || e.Code != protocol.CodeEmptyDMRefused {
		t.Fatalf("virgin dm: %v", e)
	}
	// A group DM with no history is fine (not the spam heuristic's target).
	res, e := m.Handle(ctx, req(t, `{"v":1,"id":2,"command":"open_channel","channel_id":"400000000000000002"}`))
	if e != nil || len(res.(protocol.OpenChannelResult).Messages) != 0 || res.(protocol.OpenChannelResult).HasMore {
		t.Fatalf("group dm: %v %+v", e, res)
	}
	if res.(protocol.OpenChannelResult).Channel.Type != "group_dm" {
		t.Fatalf("channel: %+v", res)
	}
	m.fetchTail = func(context.Context, *ningen.State, discord.ChannelID, uint) ([]discord.Message, error) {
		return nil, errors.New("dial tcp: network unreachable")
	}
	if _, e := m.Handle(ctx, req(t, `{"v":1,"id":3,"command":"open_channel","channel_id":"300000000000000002"}`)); e == nil || e.Code != protocol.CodeDiscordError {
		t.Fatalf("rest failure: %v", e)
	}
}

func TestMessageEventsRouted(t *testing.T) {
	m, n := readyManager(t)

	// Message in a muted guild: routed to the channel only, no notify.
	quiet := guildMsg(1, "hello")
	quiet.ChannelID, quiet.GuildID = chQuiet, guildQuiet
	dispatch(n, &gateway.MessageCreateEvent{Message: quiet})
	r := routed(t, m)
	ev, ok := r.Event.(protocol.MessageCreateEvent)
	if !ok || r.ChannelID != "300000000000000008" || r.All || ev.Notify || ev.ChannelName != "chat" || ev.Message.Content != "hello" || ev.GuildID == nil {
		t.Fatalf("muted guild message: %+v %+v", r, ev)
	}
	drain(m) // read_state_changed for the new message

	// Message in a guild set to "all messages": notifies without a mention.
	msg := guildMsg(1, "hello")
	dispatch(n, &gateway.MessageCreateEvent{Message: msg})
	r = routed(t, m)
	ev = r.Event.(protocol.MessageCreateEvent)
	if !r.All || !ev.Notify || ev.Message.MentionsSelf || ev.ChannelName != "general" {
		t.Fatalf("all-messages guild: %+v %+v", r, ev)
	}
	drain(m)

	// Mentioning message: notify → All.
	msg = guildMsg(2, "<@100000000000000001> ping")
	msg.Mentions = []discord.GuildUser{{User: discord.User{ID: selfID}}}
	dispatch(n, &gateway.MessageCreateEvent{Message: msg})
	r = routed(t, m)
	ev = r.Event.(protocol.MessageCreateEvent)
	if !r.All || !ev.Notify || !ev.Message.MentionsSelf {
		t.Fatalf("mention: %+v %+v", r, ev)
	}
	drain(m)

	// DM from ada: notifies with the DM's name.
	dm := discord.Message{ID: msgBase + 3, ChannelID: dmAda, Author: discord.User{ID: adaID, Username: "ada", DisplayName: "Ada"}, Content: "yo", Timestamp: ts(3)}
	dispatch(n, &gateway.MessageCreateEvent{Message: dm})
	r = routed(t, m)
	ev = r.Event.(protocol.MessageCreateEvent)
	if !r.All || !ev.Notify || ev.ChannelName != "Ada" || ev.GuildID != nil || ev.Message.MentionsSelf {
		t.Fatalf("dm: %+v %+v", r, ev)
	}
	drain(m)

	// Own message: never notifies, nonce echoed.
	own := guildMsg(4, "mine")
	own.Author = discord.User{ID: selfID, Username: "tester"}
	own.Nonce = "n-1"
	dispatch(n, &gateway.MessageCreateEvent{Message: own})
	r = routed(t, m)
	ev = r.Event.(protocol.MessageCreateEvent)
	if r.All || ev.Notify || ev.Message.Nonce != "n-1" {
		t.Fatalf("own: %+v %+v", r, ev)
	}
	drain(m)

	// Reaction add → message_update with the reaction folded in from the cache.
	dispatch(n, &gateway.MessageReactionAddEvent{UserID: selfID, ChannelID: chGeneral, MessageID: msgBase + 1, Emoji: discord.Emoji{Name: "🔥"}, GuildID: guildOmar})
	r = routed(t, m)
	up, ok := r.Event.(protocol.MessageUpdateEvent)
	if !ok || r.All || len(up.Message.Reactions) != 1 || up.Message.Reactions[0].Emoji != "🔥" || !up.Message.Reactions[0].Me {
		t.Fatalf("reaction add: %+v %+v", r, up)
	}
	dispatch(n, &gateway.MessageReactionRemoveAllEvent{ChannelID: chGeneral, MessageID: msgBase + 1, GuildID: guildOmar})
	up = routed(t, m).Event.(protocol.MessageUpdateEvent)
	if len(up.Message.Reactions) != 0 {
		t.Fatalf("reaction remove all: %+v", up)
	}
	// Reaction on an uncached message is dropped.
	dispatch(n, &gateway.MessageReactionAddEvent{UserID: adaID, ChannelID: chGeneral, MessageID: msgBase + 99, Emoji: discord.Emoji{Name: "x"}})
	noEvent(t, m)

	// Edit → message_update from the merged cache copy (author survives a
	// partial update).
	dispatch(n, &gateway.MessageUpdateEvent{Message: discord.Message{ID: msgBase + 1, ChannelID: chGeneral, GuildID: guildOmar, Content: "hello!", EditedTimestamp: ts(8)}})
	up = routed(t, m).Event.(protocol.MessageUpdateEvent)
	if up.Message.Content != "hello!" || up.Message.Author.Username != "ada" || up.Message.EditedTimestamp == nil {
		t.Fatalf("edit: %+v", up.Message)
	}
	// Partial update of an uncached message carries nothing renderable.
	dispatch(n, &gateway.MessageUpdateEvent{Message: discord.Message{ID: msgBase + 98, ChannelID: chGeneral, Embeds: []discord.Embed{{Title: "t"}}}})
	noEvent(t, m)

	// Delete and bulk delete.
	dispatch(n, &gateway.MessageDeleteEvent{ID: msgBase + 1, ChannelID: chGeneral, GuildID: guildOmar})
	del := routed(t, m).Event.(protocol.MessageDeleteEvent)
	if del.MessageID != (msgBase+1).String() || del.ChannelID != "300000000000000002" || del.GuildID == nil {
		t.Fatalf("delete: %+v", del)
	}
	dispatch(n, &gateway.MessageDeleteBulkEvent{IDs: []discord.MessageID{msgBase + 2, msgBase + 4}, ChannelID: chGeneral, GuildID: guildOmar})
	for _, want := range []discord.MessageID{msgBase + 2, msgBase + 4} {
		if del := routed(t, m).Event.(protocol.MessageDeleteEvent); del.MessageID != want.String() {
			t.Fatalf("bulk delete: %+v want %s", del, want)
		}
	}

	// Typing in a DM resolves the recipient name.
	dispatch(n, &gateway.TypingStartEvent{ChannelID: dmAda, UserID: adaID, Timestamp: discord.UnixTimestamp(1755698602)})
	r = routed(t, m)
	typ := r.Event.(protocol.TypingStartEvent)
	if r.ChannelID != "400000000000000001" || typ.DisplayName != "Ada" || typ.UserID != "100000000000000002" || typ.Timestamp != "2025-08-20T14:03:22.000Z" {
		t.Fatalf("typing: %+v", typ)
	}
	// Typing in a guild with the member on the event.
	dispatch(n, &gateway.TypingStartEvent{ChannelID: chGeneral, GuildID: guildOmar, UserID: linID, Timestamp: 1, Member: &discord.Member{User: discord.User{ID: linID, Username: "lin"}, Nick: "L"}})
	if typ := routed(t, m).Event.(protocol.TypingStartEvent); typ.DisplayName != "L" {
		t.Fatalf("guild typing: %+v", typ)
	}
}

// nextUnrouted returns the next non-routed event: ningen fires read.UpdateEvent
// on its own goroutine, so its order relative to message_create is undefined.
func nextUnrouted(t *testing.T, m *Manager) any {
	t.Helper()
	for {
		ev := nextEvent(t, m)
		if _, ok := ev.(socket.Routed); !ok {
			return ev
		}
	}
}

func TestReadStateChangedFunnel(t *testing.T) {
	m, n := readyManager(t)
	// A new message in #general (read, 0 mentions) → unread, total unchanged (3).
	dispatch(n, &gateway.MessageCreateEvent{Message: guildMsg(1, "x")})
	ev := nextUnrouted(t, m)
	rs, ok := ev.(protocol.ReadStateChangedEvent)
	if !ok || rs.ChannelID != "300000000000000002" || !rs.Unread || rs.MentionCount != 0 || rs.TotalMentionCount != 3 || rs.GuildID == nil {
		t.Fatalf("read_state_changed: %#v", ev)
	}
	noEvent(t, m) // total unchanged → no state_changed

	// A mention bumps the total and emits state_changed after the read event.
	msg := guildMsg(2, "<@100000000000000001>")
	msg.Mentions = []discord.GuildUser{{User: discord.User{ID: selfID}}}
	dispatch(n, &gateway.MessageCreateEvent{Message: msg})
	rs = nextUnrouted(t, m).(protocol.ReadStateChangedEvent)
	if rs.MentionCount != 1 || rs.TotalMentionCount != 4 {
		t.Fatalf("mention read state: %+v", rs)
	}
	if st := nextUnrouted(t, m).(protocol.StateChangedEvent).State; st.TotalMentionCount != 4 {
		t.Fatalf("state after mention: %+v", st)
	}

	// ack: error paths, then a real MarkRead (the message is cached and not
	// ours → ningen would POST the ack; the REST call fails offline, which is
	// fine) and its read_state_changed.
	call := func(line string) *protocol.Error {
		_, e := m.Handle(context.Background(), req(t, line))
		return e
	}
	for _, c := range []struct{ line, code string }{
		{`{"v":1,"id":1,"command":"ack","channel_id":"x","message_id":"1"}`, protocol.CodeInvalidArgument},
		{`{"v":1,"id":1,"command":"ack","channel_id":"300000000000000002"}`, protocol.CodeInvalidArgument},
		{`{"v":1,"id":1,"command":"ack","channel_id":"300000000000000099","message_id":"600000000000000002"}`, protocol.CodeUnknownChannel},
		{`{"v":1,"id":1,"command":"ack","channel_id":"300000000000000002","message_id":"600000000000000077"}`, protocol.CodeUnknownMessage},
	} {
		if e := call(c.line); e == nil || e.Code != c.code {
			t.Errorf("%s: got %v want %s", c.line, e, c.code)
		}
	}
	if e := call(`{"v":1,"id":2,"command":"ack","channel_id":"300000000000000002","message_id":"600000000000000002"}`); e != nil {
		t.Fatal(e)
	}
	rs = nextEvent(t, m).(protocol.ReadStateChangedEvent)
	if rs.Unread || rs.MentionCount != 0 || rs.LastReadMessageID == nil || *rs.LastReadMessageID != "600000000000000002" || rs.TotalMentionCount != 3 {
		t.Fatalf("ack read state: %+v", rs)
	}
	if st := nextEvent(t, m).(protocol.StateChangedEvent).State; st.TotalMentionCount != 3 {
		t.Fatalf("state after ack: %+v", st)
	}

	// Before ready (fresh session) ack is not_logged_in / gateway_unavailable.
	m2 := New(&fakeKeyring{})
	if _, e := m2.Handle(context.Background(), req(t, `{"v":1,"id":1,"command":"ack","channel_id":"300000000000000002","message_id":"600000000000000002"}`)); e == nil || e.Code != protocol.CodeNotLoggedIn {
		t.Fatalf("ack logged out: %v", e)
	}
	// A synthetic read.UpdateEvent (another device acked) is funneled 1:1.
	dispatch(n, &read.UpdateEvent{ReadState: gateway.ReadState{ChannelID: chDev, LastMessageID: 500000000000000020, MentionCount: 0}, GuildID: guildOmar, Unread: false})
	rs = nextEvent(t, m).(protocol.ReadStateChangedEvent)
	if rs.ChannelID != "300000000000000003" || rs.Unread || *rs.LastReadMessageID != "500000000000000020" {
		t.Fatalf("remote ack: %+v", rs)
	}
}

// TestOpenChannelRegistersBeforeFetch: the open is registered before the tail
// fetch so a message arriving mid-fetch is routed to the connection; a failed
// open rolls the registration back, but never closes an already-open channel.
func TestOpenChannelRegistersBeforeFetch(t *testing.T) {
	m, n := readyManager(t)
	fillCache(m, n, chGeneral, guildOmar, 0, 5)
	drain(m)
	client := newFakeClient()
	ctx := socket.WithClient(context.Background(), client)
	const id = "300000000000000002"

	var openDuringFetch bool
	m.fetchTail = func(ctx context.Context, n *ningen.State, chID discord.ChannelID, limit uint) ([]discord.Message, error) {
		openDuringFetch = client.HasOpen(id)
		// A message lands while the REST round trip is in flight.
		dispatch(n, &gateway.MessageCreateEvent{Message: guildMsg(10, "mid-fetch")})
		return n.Cabinet.Messages(chID)
	}
	res, e := m.Handle(ctx, req(t, `{"v":1,"id":1,"command":"open_channel","channel_id":"`+id+`"}`))
	if e != nil {
		t.Fatal(e)
	}
	if !openDuringFetch {
		t.Fatal("channel must be registered open before the tail fetch")
	}
	r := routed(t, m)
	if r.ChannelID != id || r.Event.(protocol.MessageCreateEvent).Message.Content != "mid-fetch" {
		t.Fatalf("mid-fetch message not routed: %+v", r)
	}
	// The same message is also in the tail (cache was filled by the event); the client dedupes by id.
	msgs := res.(protocol.OpenChannelResult).Messages
	if msgs[len(msgs)-1].ID != (msgBase + 10).String() {
		t.Fatalf("tail should end with the mid-fetch message, got %s", msgs[len(msgs)-1].ID)
	}
	drain(m)

	// A failed reopen of an open channel leaves it open.
	m.fetchTail = func(context.Context, *ningen.State, discord.ChannelID, uint) ([]discord.Message, error) {
		return nil, errors.New("dial tcp: network unreachable")
	}
	if _, e := m.Handle(ctx, req(t, `{"v":1,"id":2,"command":"open_channel","channel_id":"`+id+`"}`)); e == nil || e.Code != protocol.CodeDiscordError {
		t.Fatalf("rest failure: %v", e)
	}
	if !client.HasOpen(id) {
		t.Fatal("failed reopen must not close an open channel")
	}
	// A failed first open is rolled back: REST error and virgin-DM refusal.
	if _, e := m.Handle(ctx, req(t, `{"v":1,"id":3,"command":"open_channel","channel_id":"300000000000000003"}`)); e == nil || e.Code != protocol.CodeDiscordError {
		t.Fatalf("rest failure: %v", e)
	}
	if client.HasOpen("300000000000000003") {
		t.Fatal("failed open left the channel open")
	}
	m.fetchTail = func(context.Context, *ningen.State, discord.ChannelID, uint) ([]discord.Message, error) {
		return nil, nil
	}
	if _, e := m.Handle(ctx, req(t, `{"v":1,"id":4,"command":"open_channel","channel_id":"400000000000000001"}`)); e == nil || e.Code != protocol.CodeEmptyDMRefused {
		t.Fatalf("virgin dm: %v", e)
	}
	if client.HasOpen("400000000000000001") {
		t.Fatal("refused DM left open")
	}
}

// TestOpenChannelTailCapped: the cache-aware fetch may hand back more than
// openTail (arikawa returns a "tiny" channel's whole store); the tail is
// capped to the newest openTail and has_more reflects the cap.
func TestOpenChannelTailCapped(t *testing.T) {
	m, n := readyManager(t)
	fillCache(m, n, chGeneral, guildOmar, 0, 70)
	drain(m)
	m.fetchTail = func(ctx context.Context, n *ningen.State, chID discord.ChannelID, limit uint) ([]discord.Message, error) {
		return n.Cabinet.Messages(chID) // all 70, newest first
	}
	ctx := socket.WithClient(context.Background(), newFakeClient())
	res, e := m.Handle(ctx, req(t, `{"v":1,"id":1,"command":"open_channel","channel_id":"300000000000000002"}`))
	if e != nil {
		t.Fatal(e)
	}
	r := res.(protocol.OpenChannelResult)
	if len(r.Messages) != openTail || !r.HasMore {
		t.Fatalf("got %d messages has_more=%v", len(r.Messages), r.HasMore)
	}
	if r.Messages[0].ID != (msgBase+20).String() || r.Messages[openTail-1].ID != (msgBase+69).String() {
		t.Fatalf("must keep the newest 50: %s … %s", r.Messages[0].ID, r.Messages[openTail-1].ID)
	}
	// A short tail reports has_more=false.
	m.fetchTail = func(ctx context.Context, n *ningen.State, chID discord.ChannelID, limit uint) ([]discord.Message, error) {
		all, _ := n.Cabinet.Messages(chID)
		return all[:3], nil
	}
	res, e = m.Handle(ctx, req(t, `{"v":1,"id":2,"command":"open_channel","channel_id":"300000000000000002"}`))
	if e != nil {
		t.Fatal(e)
	}
	if r := res.(protocol.OpenChannelResult); len(r.Messages) != 3 || r.HasMore {
		t.Fatalf("short tail: %d has_more=%v", len(r.Messages), r.HasMore)
	}
}
