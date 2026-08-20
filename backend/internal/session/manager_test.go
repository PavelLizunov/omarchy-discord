package session

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/diamondburned/arikawa/v3/utils/ws"
	"github.com/diamondburned/ningen/v3"

	"github.com/mattcalayo/omarchy-discord/backend/internal/keyring"
	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
)

type fakeKeyring struct {
	token    string
	clears   int
	stores   []string
	storeErr error
}

func (f *fakeKeyring) Lookup(context.Context) (string, error) {
	if f.token == "" {
		return "", keyring.ErrNotFound
	}
	return f.token, nil
}
func (f *fakeKeyring) Store(_ context.Context, t string) error {
	if f.storeErr != nil {
		return f.storeErr
	}
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
	// Structure carries a generation newer than the state that preceded it.
	if gs.Generation <= st.Generation {
		t.Fatalf("guilds_synced generation %d not after state generation %d", gs.Generation, st.Generation)
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
	// A connect-time snapshot is stamped with the current generation, so a
	// client can discard the (older) guilds_synced still in flight from the
	// event channel.
	snapState, snapGS := snap[0].(protocol.StateChangedEvent).State, snap[1].(protocol.GuildsSyncedEvent)
	if snapGS.Generation != snapState.Generation || snapGS.Generation < gs.Generation {
		t.Fatalf("snapshot generations: state %d guilds %d (event %d)", snapState.Generation, snapGS.Generation, gs.Generation)
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

// stubLoops replaces the network connect loop: each loop just waits for its
// context to be cancelled and records that it was torn down.
type stubLoops struct {
	mu        sync.Mutex
	cancelled map[*ningen.State]bool
}

func (s *stubLoops) run(ctx context.Context, n *ningen.State, done chan struct{}) {
	defer close(done)
	<-ctx.Done()
	s.mu.Lock()
	s.cancelled[n] = true
	s.mu.Unlock()
}

// Two concurrent session replacements must leave exactly one live session, with
// the other one closed — never two loops running or a live session orphaned.
func TestConcurrentLoginKeepsOneSession(t *testing.T) {
	m := New(&fakeKeyring{})
	loops := &stubLoops{cancelled: map[*ningen.State]bool{}}
	m.runLoop = loops.run
	n1, _ := newUnopenedState(t)
	n2, _ := newUnopenedState(t)

	var wg sync.WaitGroup
	for _, n := range []*ningen.State{n1, n2} {
		wg.Add(1)
		go func(n *ningen.State) {
			defer wg.Done()
			m.replaceSession(n, "tok")
		}(n)
	}
	wg.Wait()

	m.mu.Lock()
	live := m.n
	m.mu.Unlock()
	if live != n1 && live != n2 {
		t.Fatalf("no live session: %p", live)
	}
	loser := n1
	if live == n1 {
		loser = n2
	}
	loops.mu.Lock()
	defer loops.mu.Unlock()
	if !loops.cancelled[loser] {
		t.Fatalf("losing session was not torn down")
	}
	if loops.cancelled[live] {
		t.Fatalf("surviving session was torn down")
	}
	t.Cleanup(m.Stop)
}

// Replacing a live session resets the per-account state before the new
// session reports ready, and always emits a state_changed.
func TestReplacementResetsUserState(t *testing.T) {
	m := New(&fakeKeyring{})
	loops := &stubLoops{cancelled: map[*ningen.State]bool{}}
	m.runLoop = loops.run
	n1, ready := newUnopenedState(t)
	m.replaceSession(n1, "tok")
	nextEvent(t, m) // connecting
	dispatch(n1, ready)
	if st := nextEvent(t, m).(protocol.StateChangedEvent).State; st.User == nil || st.TotalMentionCount != 3 {
		t.Fatalf("ready state: %+v", st)
	}
	nextEvent(t, m) // guilds_synced

	n2, _ := newUnopenedState(t)
	m.replaceSession(n2, "tok2")
	st := nextEvent(t, m).(protocol.StateChangedEvent).State
	if st.Lifecycle != protocol.LifecycleConnecting || st.User != nil || st.Presence != "" || st.TotalMentionCount != 0 || st.UnreadDMChannelID != nil {
		t.Fatalf("state after replacement: %+v", st)
	}
	t.Cleanup(m.Stop)
}

// A keyring store failure still yields a live session; the result says so.
func TestLoginResultReportsKeyringFailure(t *testing.T) {
	kr := &fakeKeyring{storeErr: errors.New("secret-tool: no collection")}
	m := New(kr)
	loops := &stubLoops{cancelled: map[*ningen.State]bool{}}
	m.runLoop = loops.run
	n, _ := newUnopenedState(t)
	res := m.finishLogin(context.Background(), n, "tok", protocol.User{ID: "1"})
	if res.KeyringStored || res.User.ID != "1" {
		t.Fatalf("%+v", res)
	}
	if st := nextEvent(t, m).(protocol.StateChangedEvent).State; st.Lifecycle != protocol.LifecycleConnecting {
		t.Fatalf("%+v", st)
	}
	kr.storeErr = nil
	if res := m.finishLogin(context.Background(), n, "tok", protocol.User{ID: "1"}); !res.KeyringStored || kr.token != "tok" {
		t.Fatalf("%+v", res)
	}
	t.Cleanup(m.Stop)
}
