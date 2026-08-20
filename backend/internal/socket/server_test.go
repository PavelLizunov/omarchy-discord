package socket

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
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

func (f *fakeBackend) Handle(ctx context.Context, req *protocol.Request) (any, *protocol.Error) {
	switch req.Command {
	case "get_state":
		return protocol.State{ProtocolVersion: 1, BackendVersion: protocol.BackendVersion, Lifecycle: protocol.LifecycleLoggedOut, Generation: 1}, nil
	case "slow":
		time.Sleep(100 * time.Millisecond)
		return protocol.EmptyResult{}, nil
	case "open_channel", "close_channel":
		var p protocol.OpenChannelParams
		if e := req.Params(&p); e != nil {
			return nil, e
		}
		c := ClientFromContext(ctx)
		if c == nil {
			return nil, protocol.Errorf(protocol.CodeInternalError, "no client in context")
		}
		if req.Command == "open_channel" {
			if strings.HasPrefix(p.ChannelID, "slow") {
				// Models a cold tail fetch; the registration itself is late
				// so only the per-channel lane keeps a racing close correct.
				time.Sleep(100 * time.Millisecond)
			}
			c.OpenChannel(p.ChannelID)
			return protocol.OpenChannelResult{Channel: protocol.Channel{ID: p.ChannelID, Recipients: []protocol.User{}}, Messages: []protocol.Message{}}, nil
		}
		if !c.CloseChannel(p.ChannelID) {
			return nil, protocol.Errorf(protocol.CodeChannelNotOpen, "not open")
		}
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

// Routed events reach only the clients that opened the channel; a notify
// (All) routed event reaches everyone; closing the channel stops delivery.
func TestRoutedEventsFollowOpenChannels(t *testing.T) {
	srv, _ := startServer(t, &fakeBackend{})
	a, sa := dial(t, srv)
	b, sb := dial(t, srv)
	next(t, sa) // snapshots
	next(t, sb)

	fmt.Fprintln(a, `{"v":1,"id":1,"command":"open_channel","channel_id":"77"}`)
	if r := next(t, sa); r["ok"] != true {
		t.Fatalf("open: %v", r)
	}

	msg := func(id string) protocol.Message {
		return protocol.Message{ID: id, ChannelID: "77", Attachments: []protocol.Attachment{}, Embeds: []protocol.Embed{}, Reactions: []protocol.Reaction{}}
	}
	srv.Broadcast(Routed{ChannelID: "77", Event: protocol.NewMessageCreate(msg("1"), false, "general")})
	srv.Broadcast(Routed{ChannelID: "78", Event: protocol.NewMessageCreate(msg("2"), false, "other")})
	srv.Broadcast(Routed{ChannelID: "78", All: true, Event: protocol.NewMessageCreate(msg("3"), true, "other")})
	srv.Broadcast(protocol.NewReadStateChanged("77", nil, true, 0, nil, 0))

	// A: message 1 (open), then 3 (notify), then read state; never 2.
	if ev := next(t, sa); ev["event"] != "message_create" || ev["message"].(map[string]any)["id"] != "1" {
		t.Fatalf("a first: %v", ev)
	}
	if ev := next(t, sa); ev["event"] != "message_create" || ev["message"].(map[string]any)["id"] != "3" || ev["notify"] != true {
		t.Fatalf("a second: %v", ev)
	}
	if ev := next(t, sa); ev["event"] != "read_state_changed" {
		t.Fatalf("a third: %v", ev)
	}
	// B: only the notify message and the read state.
	if ev := next(t, sb); ev["event"] != "message_create" || ev["message"].(map[string]any)["id"] != "3" {
		t.Fatalf("b first: %v", ev)
	}
	if ev := next(t, sb); ev["event"] != "read_state_changed" {
		t.Fatalf("b second: %v", ev)
	}
	// Close: A stops receiving; a second close is channel_not_open.
	fmt.Fprintln(a, `{"v":1,"id":2,"command":"close_channel","channel_id":"77"}`)
	if r := next(t, sa); r["ok"] != true {
		t.Fatalf("close: %v", r)
	}
	fmt.Fprintln(a, `{"v":1,"id":3,"command":"close_channel","channel_id":"77"}`)
	if r := next(t, sa); r["ok"] != false || r["error"].(map[string]any)["code"] != protocol.CodeChannelNotOpen {
		t.Fatalf("second close: %v", r)
	}
	srv.Broadcast(Routed{ChannelID: "77", Event: protocol.NewMessageDelete("77", nil, "1")})
	fmt.Fprintln(a, `{"v":1,"id":4,"command":"ping"}`)
	if r := next(t, sa); r["type"] != "response" || r["id"] != float64(4) {
		t.Fatalf("after close, expected only the ping response, got %v", r)
	}
	// B still gets nothing for 77 either.
	fmt.Fprintln(b, `{"v":1,"id":5,"command":"ping"}`)
	if r := next(t, sb); r["id"] != float64(5) {
		t.Fatalf("b: %v", r)
	}
	// Open-channel state is per connection: B opening 77 now gets deletes, A does not.
	fmt.Fprintln(b, `{"v":1,"id":6,"command":"open_channel","channel_id":"77"}`)
	next(t, sb)
	srv.Broadcast(Routed{ChannelID: "77", Event: protocol.NewMessageDelete("77", nil, "9")})
	if ev := next(t, sb); ev["event"] != "message_delete" || ev["message_id"] != "9" {
		t.Fatalf("b delete: %v", ev)
	}
	fmt.Fprintln(a, `{"v":1,"id":7,"command":"ping"}`)
	if r := next(t, sa); r["id"] != float64(7) {
		t.Fatalf("a leaked delete: %v", r)
	}
}

func TestClientFromContextNil(t *testing.T) {
	if ClientFromContext(context.Background()) != nil {
		t.Fatal("expected nil client")
	}
}

// TestOpenCloseSameChannelOrdered: open_channel and close_channel for one
// channel run in arrival order even though each request has its own
// goroutine, so a close sent right after a slow open lands after it and the
// channel ends closed. Other channels are not held up by the slow open.
func TestOpenCloseSameChannelOrdered(t *testing.T) {
	srv, _ := startServer(t, &fakeBackend{})
	a, sa := dial(t, srv)
	next(t, sa)

	fmt.Fprintln(a, `{"v":1,"id":1,"command":"open_channel","channel_id":"slow1"}`)
	fmt.Fprintln(a, `{"v":1,"id":2,"command":"close_channel","channel_id":"slow1"}`)
	fmt.Fprintln(a, `{"v":1,"id":3,"command":"open_channel","channel_id":"77"}`)
	// 77 opens without waiting for slow1.
	if r := next(t, sa); r["id"] != float64(3) || r["ok"] != true {
		t.Fatalf("independent channel blocked: %v", r)
	}
	if r := next(t, sa); r["id"] != float64(1) || r["ok"] != true {
		t.Fatalf("open: %v", r)
	}
	if r := next(t, sa); r["id"] != float64(2) || r["ok"] != true {
		t.Fatalf("close must follow the open and succeed: %v", r)
	}
	srv.Broadcast(Routed{ChannelID: "slow1", Event: protocol.NewMessageDelete("slow1", nil, "1")})
	srv.Broadcast(Routed{ChannelID: "77", Event: protocol.NewMessageDelete("77", nil, "2")})
	if ev := next(t, sa); ev["event"] != "message_delete" || ev["message_id"] != "2" {
		t.Fatalf("slow1 leaked after close, or 77 not open: %v", ev)
	}
	// Lanes are released: a fresh open/close pair works, and the lane map drains.
	fmt.Fprintln(a, `{"v":1,"id":4,"command":"open_channel","channel_id":"slow1"}`)
	fmt.Fprintln(a, `{"v":1,"id":5,"command":"close_channel","channel_id":"slow1"}`)
	if r := next(t, sa); r["id"] != float64(4) || r["ok"] != true {
		t.Fatalf("reopen: %v", r)
	}
	if r := next(t, sa); r["id"] != float64(5) || r["ok"] != true {
		t.Fatalf("reclose: %v", r)
	}
	srv.mu.Lock()
	for c := range srv.conns {
		c.lanesMu.Lock()
		if len(c.lanes) != 0 {
			t.Errorf("lanes not drained: %d", len(c.lanes))
		}
		c.lanesMu.Unlock()
	}
	srv.mu.Unlock()
}
