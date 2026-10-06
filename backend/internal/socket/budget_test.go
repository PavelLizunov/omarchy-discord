package socket

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
)

type busyBackend struct {
	started chan struct{}
}

func (b *busyBackend) Snapshot() []any { return nil }
func (b *busyBackend) Handle(ctx context.Context, _ *protocol.Request) (any, *protocol.Error) {
	b.started <- struct{}{}
	<-ctx.Done()
	return nil, protocol.Errorf(protocol.CodeInternalError, "cancelled")
}

func TestInflightBudgetKeepsPingResponsive(t *testing.T) {
	b := &busyBackend{started: make(chan struct{}, 65)}
	srv, _ := startServer(t, b)
	c, sc := dial(t, srv)
	for i := 1; i <= 64; i++ {
		fmt.Fprintf(c, "{\"v\":1,\"id\":%d,\"command\":\"wait\"}\n", i)
	}
	for i := 0; i < 64; i++ {
		select {
		case <-b.started:
		case <-time.After(time.Second):
			t.Fatal("request did not start")
		}
	}
	fmt.Fprintln(c, `{"v":1,"id":65,"command":"wait"}`)
	select {
	case <-b.started:
		t.Fatal("inflight request budget exceeded")
	case <-time.After(100 * time.Millisecond):
	}
	r := next(t, sc)
	if r["id"] != float64(65) || r["error"].(map[string]any)["code"] != protocol.CodeRateLimited {
		t.Fatalf("expected overload response, got %v", r)
	}
	fmt.Fprintln(c, `{"v":1,"id":66,"command":"ping"}`)
	if r := next(t, sc); r["id"] != float64(66) || r["ok"] != true {
		t.Fatalf("ping blocked by slow requests: %v", r)
	}
}
