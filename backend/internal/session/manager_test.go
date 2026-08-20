package session

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/diamondburned/arikawa/v3/utils/ws"

	"github.com/mattcalayo/omarchy-discord/backend/internal/keyring"
	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
)

type fakeKeyring struct {
	token  string
	clears int
	stores []string
}

func (f *fakeKeyring) Lookup(context.Context) (string, error) {
	if f.token == "" {
		return "", keyring.ErrNotFound
	}
	return f.token, nil
}
func (f *fakeKeyring) Store(_ context.Context, t string) error {
	f.stores = append(f.stores, t)
	f.token = t
	return nil
}
func (f *fakeKeyring) Clear(context.Context) error { f.clears++; f.token = ""; return nil }

func req(t *testing.T, line string) *protocol.Request {
	t.Helper()
	r, e := protocol.DecodeRequest([]byte(line))
	if e != nil {
		t.Fatal(e)
	}
	return r
}

func nextEvent(t *testing.T, m *Manager) any {
	t.Helper()
	select {
	case ev := <-m.Events():
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("no event")
		return nil
	}
}

func TestStartWithoutToken(t *testing.T) {
	m := New(&fakeKeyring{})
	m.Start(context.Background())
	ev := nextEvent(t, m).(protocol.StateChangedEvent)
	if ev.State.Lifecycle != protocol.LifecycleLoggedOut || ev.State.Generation != 2 {
		t.Fatalf("%+v", ev.State)
	}
	snap := m.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("snapshot %+v", snap)
	}
	for _, c := range []string{"list_guilds", "list_dms", `list_channels","guild_id":"1`, "logout"} {
		_, e := m.Handle(context.Background(), req(t, `{"v":1,"id":1,"command":"`+c+`"}`))
		if e == nil || e.Code != protocol.CodeNotLoggedIn {
			t.Errorf("%s: %v", c, e)
		}
	}
	_, e := m.Handle(context.Background(), req(t, `{"v":1,"id":1,"command":"send"}`))
	if e == nil || e.Code != protocol.CodeUnknownCommand {
		t.Errorf("unknown: %v", e)
	}
	_, e = m.Handle(context.Background(), req(t, `{"v":1,"id":1,"command":"login"}`))
	if e == nil || e.Code != protocol.CodeInvalidArgument {
		t.Errorf("login without token: %v", e)
	}
	_, e = m.Handle(context.Background(), req(t, `{"v":1,"id":1,"command":"list_channels","guild_id":"abc"}`))
	if e == nil || e.Code != protocol.CodeInvalidArgument {
		t.Errorf("bad guild id: %v", e)
	}
	res, e := m.Handle(context.Background(), req(t, `{"v":1,"id":1,"command":"get_state"}`))
	if e != nil || res.(protocol.State).Lifecycle != protocol.LifecycleLoggedOut {
		t.Errorf("get_state: %v %+v", e, res)
	}
}

// TestLifecycleFromFixture drives the manager's gateway handlers with the READY
// fixture and synthetic close events instead of a live connection.
func TestLifecycleFromFixture(t *testing.T) {
	kr := &fakeKeyring{}
	m := New(kr)
	n, ready := newUnopenedState(t)

	// Attach without starting the connect loop (no network in tests).
	m.mu.Lock()
	m.n, m.token = n, "tok"
	m.installHandlers(n)
	m.setLifecycleLocked(protocol.LifecycleConnecting, "")
	m.mu.Unlock()
	if ev := nextEvent(t, m).(protocol.StateChangedEvent); ev.State.Lifecycle != protocol.LifecycleConnecting {
		t.Fatalf("%+v", ev.State)
	}
	if _, e := m.Handle(context.Background(), req(t, `{"v":1,"id":1,"command":"list_guilds"}`)); e == nil || e.Code != protocol.CodeGatewayUnavailable {
		t.Fatalf("before ready: %v", e)
	}

	dispatch(n, ready)

	st := nextEvent(t, m).(protocol.StateChangedEvent).State
	if st.Lifecycle != protocol.LifecycleReady || st.User == nil || st.User.Username != "tester" || st.TotalMentionCount != 3 || st.UnreadDMChannelID == nil || st.Presence != "idle" {
		t.Fatalf("ready state: %+v", st)
	}
	gs := nextEvent(t, m).(protocol.GuildsSyncedEvent)
	if len(gs.Guilds) != 2 || len(gs.DMs) != 2 {
		t.Fatalf("guilds_synced: %+v", gs)
	}
	b, _ := json.Marshal(gs)
	var m2 map[string]any
	json.Unmarshal(b, &m2)
	if m2["event"] != "guilds_synced" {
		t.Fatalf("%s", b)
	}

	snap := m.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("snapshot %+v", snap)
	}
	res, e := m.Handle(context.Background(), req(t, `{"v":1,"id":1,"command":"list_channels","guild_id":"200000000000000001"}`))
	if e != nil || len(res.(protocol.ListChannelsResult).Channels) != 5 {
		t.Fatalf("list_channels: %v %+v", e, res)
	}
	_, e = m.Handle(context.Background(), req(t, `{"v":1,"id":1,"command":"list_channels","guild_id":"200000000000000009"}`))
	if e == nil || e.Code != protocol.CodeUnknownGuild {
		t.Fatalf("unknown guild: %v", e)
	}

	// Transient drop → connecting; resume → ready.
	dispatch(n, &ws.CloseEvent{Code: -1})
	if st := nextEvent(t, m).(protocol.StateChangedEvent).State; st.Lifecycle != protocol.LifecycleConnecting {
		t.Fatalf("%+v", st)
	}
	dispatch(n, ready)
	if st := nextEvent(t, m).(protocol.StateChangedEvent).State; st.Lifecycle != protocol.LifecycleReady {
		t.Fatalf("%+v", st)
	}
	nextEvent(t, m) // guilds_synced

	// Fatal close → reauth_needed with user cleared.
	dispatch(n, &ws.CloseEvent{Code: 4004})
	st = nextEvent(t, m).(protocol.StateChangedEvent).State
	if st.Lifecycle != protocol.LifecycleReauthNeeded || st.User != nil || st.Error == "" {
		t.Fatalf("%+v", st)
	}
	// Structure stays readable from cache after reauth_needed.
	if _, e := m.Handle(context.Background(), req(t, `{"v":1,"id":1,"command":"list_guilds"}`)); e != nil {
		t.Fatalf("cache after reauth: %v", e)
	}

	// Logout clears the keyring and goes logged_out.
	if e := m.Logout(context.Background()); e != nil {
		t.Fatal(e)
	}
	if st := nextEvent(t, m).(protocol.StateChangedEvent).State; st.Lifecycle != protocol.LifecycleLoggedOut || st.TotalMentionCount != 0 {
		t.Fatalf("%+v", st)
	}
	if kr.clears != 1 {
		t.Fatalf("keyring clears = %d", kr.clears)
	}
}
