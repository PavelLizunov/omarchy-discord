package socket

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
)

type disconnectBackend struct {
	started chan struct{}
	ended   chan struct{}
}

func (b *disconnectBackend) Snapshot() []any { return nil }
func (b *disconnectBackend) Handle(ctx context.Context, _ *protocol.Request) (any, *protocol.Error) {
	close(b.started)
	<-ctx.Done()
	close(b.ended)
	return nil, protocol.Errorf(protocol.CodeInternalError, "cancelled")
}

func TestDisconnectCancelsInflightRequest(t *testing.T) {
	b := &disconnectBackend{started: make(chan struct{}), ended: make(chan struct{})}
	srv, _ := startServer(t, b)
	c, _ := dial(t, srv)
	fmt.Fprintln(c, `{"v":1,"id":1,"command":"wait"}`)
	select {
	case <-b.started:
	case <-time.After(time.Second):
		t.Fatal("request never started")
	}
	c.Close()
	select {
	case <-b.ended:
	case <-time.After(time.Second):
		t.Fatal("request context survived client disconnect")
	}
}

func TestPingBurstHasUniqueCorrelatedResponses(t *testing.T) {
	srv, _ := startServer(t, &fakeBackend{})
	c, sc := dial(t, srv)
	next(t, sc)
	const count = 256
	for i := 1; i <= count; i++ {
		fmt.Fprintf(c, "{\"v\":1,\"id\":%d,\"command\":\"ping\"}\n", i)
	}
	seen := make(map[int]bool)
	for i := 0; i < count; i++ {
		m := next(t, sc)
		id := int(m["id"].(float64))
		if id < 1 || id > count || seen[id] || m["ok"] != true {
			t.Fatalf("invalid correlated response: %v", m)
		}
		seen[id] = true
	}
}

func TestDisconnectedLaneDoesNotDispatchQueuedCommand(t *testing.T) {
	b := &disconnectBackend{started: make(chan struct{}), ended: make(chan struct{})}
	a, peer := net.Pipe()
	defer peer.Close()
	srv := New("", b)
	c := &conn{nc: a, srv: srv, out: make(chan []byte, 1), done: make(chan struct{}), lanes: make(map[string]*ticket)}
	ticket := &ticket{id: "channel", prev: &ticket{done: make(chan struct{})}, done: make(chan struct{})}
	c.close()
	done := make(chan struct{})
	go func() {
		c.dispatch(context.Background(), &protocol.Request{ID: 1, Command: "wait"}, ticket)
		close(done)
	}()
	select {
	case <-b.started:
		t.Fatal("disconnected queued command reached backend")
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("queued request did not terminate")
	}
}
