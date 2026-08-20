package socket

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
)

// fakeBackend is a stand-in session: fixed snapshot, a couple of commands.
type fakeBackend struct {
	mu    sync.Mutex
	ready bool
}

func (f *fakeBackend) Snapshot() []any {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := protocol.State{ProtocolVersion: 1, BackendVersion: protocol.BackendVersion, Lifecycle: protocol.LifecycleLoggedOut, Generation: 1}
	evs := []any{protocol.NewStateChanged(st)}
	if f.ready {
		st.Lifecycle = protocol.LifecycleReady
		evs = []any{protocol.NewStateChanged(st), protocol.NewGuildsSynced(1, []protocol.Guild{{ID: "1", Name: "g", Unread: "read"}}, nil)}
	}
	return evs
}

func (f *fakeBackend) Handle(_ context.Context, req *protocol.Request) (any, *protocol.Error) {
	switch req.Command {
	case "get_state":
		return protocol.State{ProtocolVersion: 1, BackendVersion: protocol.BackendVersion, Lifecycle: protocol.LifecycleLoggedOut, Generation: 1}, nil
	case "slow":
		time.Sleep(100 * time.Millisecond)
		return protocol.EmptyResult{}, nil
	}
	return nil, protocol.Errorf(protocol.CodeUnknownCommand, "unknown command %q", req.Command)
}

type msg map[string]any

func startServer(t *testing.T, b Backend) (*Server, context.CancelFunc) {
	t.Helper()
	// Unix socket paths are length-limited; keep it short.
	dir, err := os.MkdirTemp("", "ods")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "sub", "b.sock")
	// Pre-create a stale file to prove it gets unlinked.
	os.MkdirAll(filepath.Dir(path), 0o700)
	os.WriteFile(path, nil, 0o600)
	srv := New(path, b)
	if err := srv.Listen(); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode %v", st.Mode().Perm())
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { srv.Serve(ctx); close(done) }()
	t.Cleanup(func() {
		cancel()
		<-done
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("socket file not removed on shutdown")
		}
	})
	return srv, cancel
}

func dial(t *testing.T, srv *Server) (net.Conn, *bufio.Scanner) {
	t.Helper()
	c, err := net.Dial("unix", srv.Path())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	c.SetDeadline(time.Now().Add(5 * time.Second))
	return c, bufio.NewScanner(c)
}

func next(t *testing.T, sc *bufio.Scanner) msg {
	t.Helper()
	if !sc.Scan() {
		t.Fatalf("connection closed: %v", sc.Err())
	}
	var m msg
	if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
		t.Fatalf("bad json %q: %v", sc.Text(), err)
	}
	return m
}

func TestSnapshotThenCorrelatedResponses(t *testing.T) {
	srv, _ := startServer(t, &fakeBackend{})
	c, sc := dial(t, srv)

	// Send requests immediately; the snapshot must still arrive first.
	fmt.Fprint(c, `{"v":1,"id":1,"command":"hello"}`+"\n"+`{"v":1,"id":2,"command":"ping"}`+"\n"+`{"v":1,"id":3,"command":"get_state"}`+"\n")

	first := next(t, sc)
	if first["type"] != "event" || first["event"] != "state_changed" {
		t.Fatalf("first line is not the snapshot: %v", first)
	}
	if first["state"].(map[string]any)["lifecycle"] != "logged_out" {
		t.Fatalf("snapshot state: %v", first)
	}

	got := map[float64]msg{}
	for i := 0; i < 3; i++ {
		m := next(t, sc)
		if m["type"] != "response" {
			t.Fatalf("expected response, got %v", m)
		}
		got[m["id"].(float64)] = m
	}
	if r := got[1]["result"].(map[string]any); r["engine"] != "arikawa" || r["protocol_version"] != float64(1) || r["backend_version"] != protocol.BackendVersion {
		t.Fatalf("hello: %v", got[1])
	}
	if r := got[2]["result"].(map[string]any); r["pong"] != true {
		t.Fatalf("ping: %v", got[2])
	}
	if r := got[3]["result"].(map[string]any); r["lifecycle"] != "logged_out" {
		t.Fatalf("get_state: %v", got[3])
	}
	for id, m := range got {
		if m["ok"] != true || m["v"] != float64(1) {
			t.Fatalf("id %v: %v", id, m)
		}
	}
}

func TestReadySnapshotIncludesGuildsSynced(t *testing.T) {
	srv, _ := startServer(t, &fakeBackend{ready: true})
	_, sc := dial(t, srv)
	if m := next(t, sc); m["event"] != "state_changed" {
		t.Fatalf("%v", m)
	}
	m := next(t, sc)
	if m["event"] != "guilds_synced" {
		t.Fatalf("%v", m)
	}
	if gs := m["guilds"].([]any); len(gs) != 1 {
		t.Fatalf("%v", m)
	}
	if _, ok := m["dms"].([]any); !ok {
		t.Fatalf("dms must be an array: %v", m)
	}
}

func TestMalformedAndUnknownAndVersion(t *testing.T) {
	srv, _ := startServer(t, &fakeBackend{})
	c, sc := dial(t, srv)
	next(t, sc) // snapshot

	fmt.Fprint(c, "{this is not json\n")
	m := next(t, sc)
	if m["id"] != float64(0) || m["ok"] != false || m["error"].(map[string]any)["code"] != "invalid_request" {
		t.Fatalf("malformed: %v", m)
	}

	fmt.Fprint(c, `{"v":1,"id":4}`+"\n")
	m = next(t, sc)
	if m["id"] != float64(4) || m["error"].(map[string]any)["code"] != "invalid_request" {
		t.Fatalf("missing command must echo id: %v", m)
	}

	fmt.Fprint(c, `{"v":1,"id":5,"command":"send","content":"x"}`+"\n")
	m = next(t, sc)
	if m["id"] != float64(5) || m["error"].(map[string]any)["code"] != "unknown_command" {
		t.Fatalf("unknown: %v", m)
	}

	fmt.Fprint(c, `{"v":2,"id":6,"command":"ping"}`+"\n")
	m = next(t, sc)
	if m["id"] != float64(6) || m["error"].(map[string]any)["code"] != "unsupported_version" {
		t.Fatalf("version: %v", m)
	}
	if _, has := m["result"]; has {
		t.Fatalf("error response must not carry result: %v", m)
	}
}

func TestOutOfOrderResponsesAndBroadcastFanOut(t *testing.T) {
	srv, _ := startServer(t, &fakeBackend{})
	c1, sc1 := dial(t, srv)
	_, sc2 := dial(t, srv)
	next(t, sc1)
	next(t, sc2)

	// A slow command must not block ping.
	fmt.Fprint(c1, `{"v":1,"id":10,"command":"slow"}`+"\n"+`{"v":1,"id":11,"command":"ping"}`+"\n")
	if m := next(t, sc1); m["id"] != float64(11) {
		t.Fatalf("ping should overtake slow: %v", m)
	}
	if m := next(t, sc1); m["id"] != float64(10) {
		t.Fatalf("slow: %v", m)
	}

	srv.Broadcast(protocol.NewStateChanged(protocol.State{Lifecycle: protocol.LifecycleConnecting, Generation: 2}))
	for _, sc := range []*bufio.Scanner{sc1, sc2} {
		m := next(t, sc)
		if m["event"] != "state_changed" || m["state"].(map[string]any)["generation"] != float64(2) {
			t.Fatalf("broadcast: %v", m)
		}
	}
}

// The snapshot's lines are never interleaved with broadcast events, even under
// concurrent connects and broadcasts.
func TestSnapshotNotInterleavedByBroadcasts(t *testing.T) {
	srv, _ := startServer(t, &fakeBackend{ready: true})
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
				srv.Broadcast(protocol.NewStateChanged(protocol.State{Lifecycle: protocol.LifecycleReady, Generation: int64(100 + i)}))
			}
		}
	}()
	for i := 0; i < 20; i++ {
		_, sc := dial(t, srv)
		if m := next(t, sc); m["event"] != "state_changed" || m["state"].(map[string]any)["generation"] != float64(1) {
			t.Fatalf("conn %d: first line %v", i, m)
		}
		if m := next(t, sc); m["event"] != "guilds_synced" || m["generation"] != float64(1) {
			t.Fatalf("conn %d: second line %v", i, m)
		}
	}
	close(stop)
	wg.Wait()
}
