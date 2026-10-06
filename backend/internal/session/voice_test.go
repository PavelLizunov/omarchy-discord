package session

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/ningen/v3"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/voice"
)

const chLocked = discord.ChannelID(300000000000000009)

type fakeVoice struct {
	mu     sync.Mutex
	ev     voice.Events
	calls  []string
	state  voice.State
	closed int
	err    error
}

func (f *fakeVoice) record(call string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	return f.err
}

func (f *fakeVoice) Join(_ context.Context, g discord.GuildID, c discord.ChannelID) error {
	return f.record("join " + g.String() + " " + c.String())
}
func (f *fakeVoice) Leave(context.Context) error { return f.record("leave") }
func (f *fakeVoice) SetMute(_ context.Context, muted bool) error {
	return f.record("mute " + boolStr(muted))
}
func (f *fakeVoice) SetDeaf(_ context.Context, deaf bool) error {
	return f.record("deaf " + boolStr(deaf))
}
func (f *fakeVoice) State() voice.State {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state
}
func (f *fakeVoice) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed++
}

func (f *fakeVoice) took() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.calls, "; ")
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func voiceManager(t *testing.T) (*Manager, *ningen.State, *fakeVoice) {
	t.Helper()
	m := New(&fakeKeyring{})
	m.runLoop = (&stubLoops{cancelled: map[*ningen.State]bool{}}).run
	fv := &fakeVoice{}
	m.newVoice = func(_ *ningen.State, ev voice.Events) voiceEngine {
		fv.ev = ev
		return fv
	}
	n, ready := newUnopenedState(t)
	m.replaceSession(n, "tok")
	if st := nextEvent(t, m).(protocol.StateChangedEvent).State; st.Voice.Status != protocol.VoiceIdle || st.Voice.GuildID != nil {
		t.Fatalf("fresh session voice: %+v", st.Voice)
	}
	dispatch(n, ready)
	nextEvent(t, m)
	nextEvent(t, m)
	drainVoiceSeed(t, m)
	t.Cleanup(m.Stop)
	return m, n, fv
}

func drainVoiceSeed(t *testing.T, m *Manager) {
	t.Helper()
	voiceSeed(t, m)
}

func voiceReq(t *testing.T, m *Manager, line string) (any, *protocol.Error) {
	t.Helper()
	return m.Handle(context.Background(), req(t, line))
}

func TestVoiceCommandsDispatchToEngine(t *testing.T) {
	m, n, fv := voiceManager(t)
	join := `{"v":1,"id":70,"command":"voice_join","guild_id":"200000000000000001","channel_id":"300000000000000004"}`
	if res, e := voiceReq(t, m, join); e != nil || res != (protocol.EmptyResult{}) {
		t.Fatalf("voice_join: %v %+v", e, res)
	}
	if _, e := voiceReq(t, m, `{"v":1,"id":71,"command":"voice_set","muted":true}`); e != nil {
		t.Fatalf("voice_set: %v", e)
	}
	if _, e := voiceReq(t, m, `{"v":1,"id":72,"command":"voice_set","deafened":false}`); e != nil {
		t.Fatalf("voice_set deafen: %v", e)
	}
	if _, e := voiceReq(t, m, `{"v":1,"id":73,"command":"voice_set"}`); e != nil {
		t.Fatalf("empty voice_set: %v", e)
	}
	if _, e := voiceReq(t, m, `{"v":1,"id":74,"command":"voice_leave"}`); e != nil {
		t.Fatalf("voice_leave: %v", e)
	}
	want := "join 200000000000000001 300000000000000004; mute true; deaf false; leave"
	if got := fv.took(); got != want {
		t.Fatalf("engine calls:\n got %q\nwant %q", got, want)
	}

	dispatch(n, &gateway.ChannelCreateEvent{Channel: discord.Channel{
		ID: chLocked, GuildID: guildOmar, Type: discord.GuildVoice, Name: "locked", ParentID: catGeneral,
		Overwrites: []discord.Overwrite{{ID: discord.Snowflake(guildOmar), Type: discord.OverwriteRole, Deny: discord.PermissionConnect}},
	}})
	channelUpdate(t, m)

	for _, c := range []struct {
		line string
		code string
	}{
		{`{"v":1,"id":75,"command":"voice_join","guild_id":"nope","channel_id":"300000000000000004"}`, protocol.CodeInvalidArgument},
		{`{"v":1,"id":76,"command":"voice_join","guild_id":"200000000000000001","channel_id":"300000000000000002"}`, protocol.CodeInvalidArgument},
		{`{"v":1,"id":77,"command":"voice_join","guild_id":"200000000000000002","channel_id":"300000000000000004"}`, protocol.CodeUnknownChannel},
		{`{"v":1,"id":78,"command":"voice_join","guild_id":"200000000000000001","channel_id":"300000000000000009"}`, protocol.CodeForbidden},
	} {
		if _, e := voiceReq(t, m, c.line); e == nil || e.Code != c.code {
			t.Errorf("want %s, got %v", c.code, e)
		}
	}
	if got := fv.took(); got != want {
		t.Fatalf("refused requests must not reach the engine: %q", got)
	}
}

func TestVoiceCallbacksPushEvents(t *testing.T) {
	m, n, fv := voiceManager(t)
	connected := voice.State{Status: voice.StatusConnected, GuildID: guildOmar, ChannelID: chVoice}

	m.mu.Lock()
	gen := m.generation
	m.mu.Unlock()
	fv.ev.State(connected)
	st := nextEvent(t, m).(protocol.StateChangedEvent).State
	if st.Voice.Status != protocol.VoiceConnected || st.Voice.GuildID == nil || *st.Voice.ChannelID != discord.ChannelID(chVoice).String() || st.Generation <= gen {
		t.Fatalf("voice state_changed: %+v (gen %d)", st.Voice, st.Generation)
	}
	fv.ev.State(connected)

	muted := connected
	muted.Muted = true
	fv.ev.State(muted)
	if st := nextEvent(t, m).(protocol.StateChangedEvent).State; !st.Voice.Muted {
		t.Fatalf("muted state: %+v", st.Voice)
	}

	fv.ev.Speaking(adaID, true)
	sp := nextEvent(t, m).(protocol.VoiceSpeakingEvent)
	if sp.UserID != discord.UserID(adaID).String() || !sp.Speaking {
		t.Fatalf("speaking: %+v", sp)
	}

	dispatch(n, voiceStateEvent(chVoice, ada))
	nextVoiceMembers(t, m)
	snap := m.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("snapshot %+v", snap)
	}
	if ev, ok := snap[2].(protocol.VoiceMembersEvent); !ok || ev.GuildID != discord.GuildID(guildOmar).String() || len(ev.Channels) != 1 {
		t.Fatalf("snapshot voice members: %+v", snap[2])
	}

	fv.ev.State(voice.State{Status: voice.StatusError, Error: "voice gateway closed (4006)"})
	if st := nextEvent(t, m).(protocol.StateChangedEvent).State; st.Voice.Status != protocol.VoiceError || st.Voice.Error == "" || st.Voice.GuildID != nil {
		t.Fatalf("error state: %+v", st.Voice)
	}
}

func TestVoiceUnavailableWithoutEngine(t *testing.T) {
	m, _ := readyManager(t)
	for _, line := range []string{
		`{"v":1,"id":70,"command":"voice_join","guild_id":"200000000000000001","channel_id":"300000000000000004"}`,
		`{"v":1,"id":71,"command":"voice_leave"}`,
		`{"v":1,"id":72,"command":"voice_set","muted":true}`,
	} {
		if _, e := voiceReq(t, m, line); e == nil || e.Code != protocol.CodeGatewayUnavailable {
			t.Errorf("%s: %v", line, e)
		}
	}
	if st := m.Snapshot()[0].(protocol.StateChangedEvent).State; st.Voice.Status != protocol.VoiceIdle {
		t.Fatalf("voice state without an engine: %+v", st.Voice)
	}
}

func TestVoiceEngineClosedWithSession(t *testing.T) {
	m, _, fv := voiceManager(t)
	if e := m.Logout(context.Background()); e != nil {
		t.Fatal(e)
	}
	fv.mu.Lock()
	defer fv.mu.Unlock()
	if fv.closed != 1 {
		t.Fatalf("engine closed %d times", fv.closed)
	}
}

func voiceStateEvent(chID discord.ChannelID, u discord.User) *gateway.VoiceStateUpdateEvent {
	return &gateway.VoiceStateUpdateEvent{VoiceState: discord.VoiceState{
		GuildID:   guildOmar,
		ChannelID: chID,
		UserID:    u.ID,
		Member:    &discord.Member{User: u},
	}}
}

func nextVoiceMembers(t *testing.T, m *Manager) protocol.VoiceMembersEvent {
	t.Helper()
	for {
		if ev, ok := nextEvent(t, m).(protocol.VoiceMembersEvent); ok {
			return ev
		}
	}
}

func userIDs(ev protocol.VoiceMembersEvent, i int) []string {
	out := []string{}
	for _, u := range ev.Channels[i].Users {
		out = append(out, u.DisplayName)
	}
	return out
}

func TestVoiceMembersFromCabinet(t *testing.T) {
	m, n := readyManager(t)

	if ev := VoiceMembers(n.Offline(), guildOmar); len(ev.Channels) != 0 || ev.GuildID != discord.GuildID(guildOmar).String() {
		t.Fatalf("empty guild: %+v", ev)
	}

	dispatch(n, voiceStateEvent(chVoice, lin))
	ev := nextVoiceMembers(t, m)
	if len(ev.Channels) != 1 || ev.Channels[0].ChannelID != discord.ChannelID(chVoice).String() || len(ev.Channels[0].Users) != 1 {
		t.Fatalf("first occupant: %+v", ev)
	}
	if u := ev.Channels[0].Users[0]; u.ID != discord.UserID(linID).String() || u.Username != "lin" {
		t.Fatalf("occupant: %+v", u)
	}

	dispatch(n, voiceStateEvent(chVoice, ada))
	ev = nextVoiceMembers(t, m)
	if len(ev.Channels) != 1 || len(ev.Channels[0].Users) != 2 {
		t.Fatalf("two occupants: %+v", ev)
	}
	if got := userIDs(ev, 0); got[0] != "Ada" || got[1] != "lin" {
		t.Fatalf("occupant order: %v", got)
	}

	dispatch(n, voiceStateEvent(0, ada))
	if ev := nextVoiceMembers(t, m); len(ev.Channels) != 1 || len(ev.Channels[0].Users) != 1 {
		t.Fatalf("after one left: %+v", ev)
	}
	dispatch(n, voiceStateEvent(0, lin))
	if ev := nextVoiceMembers(t, m); len(ev.Channels) != 0 {
		t.Fatalf("after all left: %+v", ev)
	}
}

func TestVoiceMembersSeededAfterGuildsSynced(t *testing.T) {
	m := New(&fakeKeyring{})
	n, ready := newUnopenedState(t)
	m.mu.Lock()
	m.n, m.token = n, "tok"
	m.installHandlers(n)
	m.setLifecycleLocked(protocol.LifecycleConnecting, "")
	m.mu.Unlock()
	nextEvent(t, m)

	ready.Guilds[0].VoiceStates = []discord.VoiceState{
		{GuildID: guildOmar, ChannelID: chVoice, UserID: adaID, Member: &discord.Member{User: ada}},
	}
	dispatch(n, ready)
	nextEvent(t, m)
	nextEvent(t, m)
	seed := voiceSeed(t, m)
	if omar := seed[discord.GuildID(guildOmar).String()]; len(omar.Channels) != 1 || len(omar.Channels[0].Users) != 1 {
		t.Fatalf("seeded voice members: %+v", omar)
	}
	if quiet, ok := seed[discord.GuildID(guildQuiet).String()]; !ok || quiet.Channels == nil || len(quiet.Channels) != 0 {
		t.Fatalf("quiet guild: %+v (%v)", quiet, ok)
	}
	noEvent(t, m)

	ready.Guilds[0].VoiceStates = nil
	dispatch(n, ready)
	if omar := voiceSeed(t, m)[discord.GuildID(guildOmar).String()]; len(omar.Channels) != 0 {
		t.Fatalf("re-ready must clear the guild: %+v", omar)
	}
}

func voiceSeed(t *testing.T, m *Manager) map[string]protocol.VoiceMembersEvent {
	t.Helper()
	seed := map[string]protocol.VoiceMembersEvent{}
	for range 2 {
		ev := nextVoiceMembers(t, m)
		seed[ev.GuildID] = ev
	}
	return seed
}
