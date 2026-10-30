package voice

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func stalledGateway(t *testing.T) *gatewayImpl {
	t.Helper()
	accepted := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err == nil {
			accepted <- c
		}
	}))
	t.Cleanup(server.Close)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	peer := <-accepted
	t.Cleanup(func() { peer.Close(); conn.Close() })
	if tcp, ok := conn.UnderlyingConn().(*net.TCPConn); ok {
		tcp.SetWriteBuffer(1024)
	}
	return &gatewayImpl{conn: conn, status: StatusReady, config: gatewayConfig{Logger: slog.Default()}}
}

func TestGatewayWriteHonorsDeadlineAndCanClose(t *testing.T) {
	g := stalledGateway(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := g.send(ctx, websocket.BinaryMessage, make([]byte, 8*1024*1024)); err == nil {
		t.Fatal("stalled peer accepted unbounded write")
	}
	if time.Since(start) > time.Second {
		t.Fatal("write ignored context deadline")
	}
	done := make(chan struct{})
	go func() { g.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("close waited for stalled peer")
	}
	if g.Status() != StatusDisconnected {
		t.Fatal("gateway remained ready")
	}
}
