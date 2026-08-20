package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/utils/httputil"
	"github.com/diamondburned/ningen/v3"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/socket"
)

// fakeREST records write calls and returns scripted errors.
type fakeREST struct {
	sends    []api.SendMessageData
	edits    []string
	deletes  int
	reacts   []discord.APIEmoji
	unreacts []discord.APIEmoji
	typings  int
	statuses []discord.Status
	err      error
}

func (f *fakeREST) ops() restOps {
	return restOps{
		send: func(_ context.Context, _ *ningen.State, _ discord.ChannelID, d api.SendMessageData) (*discord.Message, error) {
			f.sends = append(f.sends, d)
			if f.err != nil {
				return nil, f.err
			}
			return &discord.Message{ID: 777, Nonce: d.Nonce}, nil
		},
		edit: func(_ context.Context, _ *ningen.State, _ discord.ChannelID, _ discord.MessageID, c string) error {
			f.edits = append(f.edits, c)
			return f.err
		},
		delete: func(context.Context, *ningen.State, discord.ChannelID, discord.MessageID) error {
			f.deletes++
			return f.err
		},
		react: func(_ context.Context, _ *ningen.State, _ discord.ChannelID, _ discord.MessageID, e discord.APIEmoji) error {
			f.reacts = append(f.reacts, e)
			return f.err
		},
		unreact: func(_ context.Context, _ *ningen.State, _ discord.ChannelID, _ discord.MessageID, e discord.APIEmoji) error {
			f.unreacts = append(f.unreacts, e)
			return f.err
		},
		typing: func(context.Context, *ningen.State, discord.ChannelID) error {
			f.typings++
			return f.err
		},
		setStatus: func(_ *ningen.State, s discord.Status) error {
			f.statuses = append(f.statuses, s)
			return f.err
		},
	}
}

func httpErr(status int) error {
	return fmt.Errorf("wrapped: %w", &httputil.HTTPError{Status: status, Message: "nope"})
}

// writeHarness is a ready manager with a fake REST layer and #general open.
func writeHarness(t *testing.T) (*Manager, *fakeREST, func(line string) (any, *protocol.Error)) {
	t.Helper()
	m, _ := readyManager(t)
	f := &fakeREST{}
	m.rest = f.ops()
	client := newFakeClient()
	client.OpenChannel("300000000000000002")
	ctx := socket.WithClient(context.Background(), client)
	return m, f, func(line string) (any, *protocol.Error) { return m.Handle(ctx, req(t, line)) }
}

const general = `"channel_id":"300000000000000002"`

func TestSendSetsNonceAndReply(t *testing.T) {
	_, f, call := writeHarness(t)
	res, e := call(`{"v":1,"id":1,"command":"send",` + general + `,"content":"hi","reply_to":"600000000000000001"}`)
	if e != nil {
		t.Fatal(e)
	}
	r := res.(protocol.SendResult)
	if r.MessageID != "777" || len(r.Nonce) != 16 || strings.Trim(r.Nonce, "0123456789abcdef") != "" {
		t.Fatalf("result %+v", r)
	}
	d := f.sends[0]
	if d.Nonce != r.Nonce || d.Content != "hi" || d.Reference == nil || d.Reference.MessageID != 600000000000000001 {
		t.Fatalf("send data %+v", d)
	}
	if d.AllowedMentions == nil || d.AllowedMentions.RepliedUser == nil || !*d.AllowedMentions.RepliedUser {
		t.Fatalf("reply mention default should be true: %+v", d.AllowedMentions)
	}

	res2, _ := call(`{"v":1,"id":2,"command":"send",` + general + `,"content":"hi","reply_to":"600000000000000001","reply_mention":false}`)
	if res2.(protocol.SendResult).Nonce == r.Nonce {
		t.Fatal("nonce reused")
	}
	if am := f.sends[1].AllowedMentions; am == nil || am.RepliedUser == nil || *am.RepliedUser {
		t.Fatalf("reply_mention false not honoured: %+v", am)
	}
	if _, e := call(`{"v":1,"id":3,"command":"send",` + general + `,"content":"plain"}`); e != nil {
		t.Fatal(e)
	}
	if d := f.sends[2]; d.Reference != nil || d.AllowedMentions != nil {
		t.Fatalf("plain send carries reply fields: %+v", d)
	}
}

func TestSendValidation(t *testing.T) {
	_, f, call := writeHarness(t)
	cases := map[string]string{
		`{"v":1,"id":1,"command":"send",` + general + `,"content":""}`:                                  protocol.CodeInvalidArgument,
		`{"v":1,"id":1,"command":"send",` + general + `,"content":"   "}`:                               protocol.CodeInvalidArgument,
		`{"v":1,"id":1,"command":"send",` + general + `,"content":"` + strings.Repeat("x", 2001) + `"}`: protocol.CodeInvalidArgument,
		`{"v":1,"id":1,"command":"send",` + general + `,"content":"x","reply_to":"nope"}`:               protocol.CodeInvalidArgument,
		`{"v":1,"id":1,"command":"send","channel_id":"300000000000000003","content":"x"}`:               protocol.CodeChannelNotOpen,
		`{"v":1,"id":1,"command":"send","channel_id":"abc","content":"x"}`:                              protocol.CodeInvalidArgument,
	}
	for line, want := range cases {
		if _, e := call(line); e == nil || e.Code != want {
			t.Errorf("%s: want %s got %v", line[:60], want, e)
		}
	}
	if len(f.sends) != 0 {
		t.Fatalf("REST called on invalid input: %+v", f.sends)
	}
	// Exactly 2000 runes (multi-byte) is fine.
	if _, e := call(`{"v":1,"id":1,"command":"send",` + general + `,"content":"` + strings.Repeat("é", 2000) + `"}`); e != nil {
		t.Fatal(e)
	}
}

func TestWriteErrorMapping(t *testing.T) {
	m, f, call := writeHarness(t)
	kr := m.kr.(*fakeKeyring)
	for _, c := range []struct {
		err  error
		want string
	}{
		{httpErr(403), protocol.CodeForbidden},
		{httpErr(429), protocol.CodeRateLimited},
		{httpErr(500), protocol.CodeDiscordError},
		{errors.New("network down"), protocol.CodeDiscordError},
	} {
		f.err = c.err
		_, e := call(`{"v":1,"id":1,"command":"send",` + general + `,"content":"x"}`)
		if e == nil || e.Code != c.want {
			t.Errorf("%v: want %s got %v", c.err, c.want, e)
		}
	}
	// 404 on a message-scoped command is unknown_message.
	f.err = httpErr(404)
	if _, e := call(`{"v":1,"id":1,"command":"delete",` + general + `,"message_id":"600000000000000001"}`); e == nil || e.Code != protocol.CodeUnknownMessage {
		t.Fatalf("404 delete: %v", e)
	}
	if kr.clears != 0 {
		t.Fatal("keyring cleared prematurely")
	}

	// 401 → reauth_needed, keyring cleared, command fails not_logged_in.
	f.err = httpErr(401)
	_, e := call(`{"v":1,"id":1,"command":"send",` + general + `,"content":"x"}`)
	if e == nil || e.Code != protocol.CodeNotLoggedIn {
		t.Fatalf("401: %v", e)
	}
	st := nextEvent(t, m).(protocol.StateChangedEvent).State
	if st.Lifecycle != protocol.LifecycleReauthNeeded || st.User != nil || !strings.Contains(st.Error, "invalidated") {
		t.Fatalf("after 401: %+v", st)
	}
	if kr.clears != 1 {
		t.Fatalf("keyring clears %d", kr.clears)
	}
	m.mu.Lock()
	tok, n := m.token, m.n
	m.mu.Unlock()
	if tok != "" || n == nil {
		t.Fatalf("token %q n %v", tok, n != nil)
	}
	// Structure is still served read-only; writes are not_logged_in.
	if _, e := call(`{"v":1,"id":1,"command":"list_guilds"}`); e != nil {
		t.Fatalf("list_guilds after reauth: %v", e)
	}
	f.err = nil
	if _, e := call(`{"v":1,"id":1,"command":"send",` + general + `,"content":"x"}`); e == nil || e.Code != protocol.CodeNotLoggedIn {
		t.Fatalf("send after reauth: %v", e)
	}
	if _, e := call(`{"v":1,"id":1,"command":"set_presence","status":"idle"}`); e == nil || e.Code != protocol.CodeNotLoggedIn {
		t.Fatalf("presence after reauth: %v", e)
	}
}

func TestEditOwnOnly(t *testing.T) {
	m, f, call := writeHarness(t)
	_, n := m.n, m.n
	theirs := guildMsg(1, "theirs")
	mine := guildMsg(2, "mine")
	mine.Author = discord.User{ID: selfID, Username: "tester"}
	n.Cabinet.MessageSet(&theirs, false)
	n.Cabinet.MessageSet(&mine, false)

	if _, e := call(`{"v":1,"id":1,"command":"edit",` + general + `,"message_id":"600000000000000001","content":"x"}`); e == nil || e.Code != protocol.CodeForbidden {
		t.Fatalf("edit theirs: %v", e)
	}
	if _, e := call(`{"v":1,"id":1,"command":"edit",` + general + `,"message_id":"600000000000000002","content":"x"}`); e != nil {
		t.Fatalf("edit mine: %v", e)
	}
	if _, e := call(`{"v":1,"id":1,"command":"edit",` + general + `,"message_id":"600000000000000002","content":""}`); e == nil || e.Code != protocol.CodeInvalidArgument {
		t.Fatalf("edit empty: %v", e)
	}
	// Uncached: REST decides.
	if _, e := call(`{"v":1,"id":1,"command":"edit",` + general + `,"message_id":"600000000000000099","content":"x"}`); e != nil {
		t.Fatalf("edit uncached: %v", e)
	}
	if len(f.edits) != 2 {
		t.Fatalf("edits %v", f.edits)
	}
	if _, e := call(`{"v":1,"id":1,"command":"delete",` + general + `,"message_id":"600000000000000001"}`); e != nil || f.deletes != 1 {
		t.Fatalf("delete: %v %d", e, f.deletes)
	}
}

func TestReactSanitizesEmoji(t *testing.T) {
	_, f, call := writeHarness(t)
	for emoji, want := range map[string]discord.APIEmoji{
		"👍":                           "👍",
		"❤️":                          "❤", // U+FE0F stripped
		"omarchy:1000000000000000099": "omarchy:1000000000000000099",
	} {
		if _, e := call(`{"v":1,"id":1,"command":"react",` + general + `,"message_id":"600000000000000001","emoji":"` + emoji + `"}`); e != nil {
			t.Fatalf("%q: %v", emoji, e)
		}
		if got := f.reacts[len(f.reacts)-1]; got != want {
			t.Fatalf("%q: sent %q want %q", emoji, got, want)
		}
	}
	if _, e := call(`{"v":1,"id":1,"command":"unreact",` + general + `,"message_id":"600000000000000001","emoji":"👍"}`); e != nil || len(f.unreacts) != 1 {
		t.Fatalf("unreact: %v", e)
	}
	for _, bad := range []string{"", "️", "name:abc", ":123"} {
		if _, e := call(`{"v":1,"id":1,"command":"react",` + general + `,"message_id":"600000000000000001","emoji":"` + bad + `"}`); e == nil || e.Code != protocol.CodeInvalidArgument {
			t.Fatalf("%q: %v", bad, e)
		}
	}
}

func TestTypingThrottle(t *testing.T) {
	m, f, call := writeHarness(t)
	now := time.Unix(1_000_000, 0)
	m.now = func() time.Time { return now }
	line := `{"v":1,"id":1,"command":"typing",` + general + `}`
	for i := 0; i < 3; i++ {
		if _, e := call(line); e != nil {
			t.Fatal(e)
		}
	}
	if f.typings != 1 {
		t.Fatalf("typings %d", f.typings)
	}
	now = now.Add(9 * time.Second)
	call(line)
	now = now.Add(time.Second)
	call(line)
	if f.typings != 2 {
		t.Fatalf("typings after 10s %d", f.typings)
	}
	// Another channel has its own budget.
	client := newFakeClient()
	client.OpenChannel("300000000000000003")
	if _, e := m.Handle(socket.WithClient(context.Background(), client), req(t, `{"v":1,"id":1,"command":"typing","channel_id":"300000000000000003"}`)); e != nil || f.typings != 3 {
		t.Fatalf("other channel: %v %d", e, f.typings)
	}
	if _, e := call(`{"v":1,"id":1,"command":"typing","channel_id":"300000000000000008"}`); e == nil || e.Code != protocol.CodeChannelNotOpen {
		t.Fatalf("closed: %v", e)
	}
}

func TestSetPresence(t *testing.T) {
	m, f, call := writeHarness(t)
	if _, e := call(`{"v":1,"id":1,"command":"set_presence","status":"busy"}`); e == nil || e.Code != protocol.CodeInvalidArgument {
		t.Fatalf("bad status: %v", e)
	}
	if _, e := call(`{"v":1,"id":1,"command":"set_presence","status":"invisible"}`); e != nil {
		t.Fatal(e)
	}
	if f.statuses[0] != discord.InvisibleStatus {
		t.Fatalf("sent %v", f.statuses)
	}
	st := nextEvent(t, m).(protocol.StateChangedEvent).State
	if st.Presence != "invisible" {
		t.Fatalf("presence %q", st.Presence)
	}
	// Same status again: no state change.
	call(`{"v":1,"id":1,"command":"set_presence","status":"invisible"}`)
	noEvent(t, m)
	f.err = httpErr(429)
	if _, e := call(`{"v":1,"id":1,"command":"set_presence","status":"dnd"}`); e == nil || e.Code != protocol.CodeRateLimited {
		t.Fatalf("429: %v", e)
	}
}

func TestWritesNeedLiveGateway(t *testing.T) {
	m := New(&fakeKeyring{})
	client := newFakeClient()
	client.OpenChannel("300000000000000002")
	ctx := socket.WithClient(context.Background(), client)
	if _, e := m.Handle(ctx, req(t, `{"v":1,"id":1,"command":"send",`+general+`,"content":"x"}`)); e == nil || e.Code != protocol.CodeNotLoggedIn {
		t.Fatalf("no session: %v", e)
	}
	m, n := readyManager(t)
	m.rest = (&fakeREST{}).ops()
	m.mu.Lock()
	m.setLifecycleLocked(protocol.LifecycleConnecting, "")
	m.mu.Unlock()
	_ = n
	if _, e := m.Handle(ctx, req(t, `{"v":1,"id":1,"command":"send",`+general+`,"content":"x"}`)); e == nil || e.Code != protocol.CodeGatewayUnavailable {
		t.Fatalf("disconnected: %v", e)
	}
}
