package voice

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"
)

func TestGatewaySendDoesNotHoldStatusWhileWaitingForConnection(t *testing.T) {
	g := &gatewayImpl{status: StatusReady}
	g.connMu.Lock()
	done := make(chan error, 1)
	go func() { done <- g.Send(context.Background(), OpcodeHeartbeat, GatewayMessageDataHeartbeat{}) }()
	// A writer may wait for the socket, but status/close must remain usable.
	time.Sleep(10 * time.Millisecond)
	available := false
	limit := time.Now().Add(time.Second)
	for time.Now().Before(limit) {
		if g.statusMu.TryLock() {
			g.statusMu.Unlock()
			available = true
			break
		}
		runtime.Gosched()
	}
	g.connMu.Unlock()
	select {
	case err := <-done:
		if !errors.Is(err, ErrGatewayNotConnected) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("send did not retire")
	}
	if !available {
		t.Fatal("send holds status lock while waiting for connection lock")
	}
}
