package remoteauth

import (
	"context"
	"crypto/rsa"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type terminalRecorder struct {
	recorder
	closed <-chan struct{}
}

func (r *terminalRecorder) Scanned(u User) {
	r.recorder.Scanned(u)
	select {
	case <-r.closed:
	case <-time.After(time.Second):
	}
}

func TestAcceptanceDeclinedAfterQueuedPeerClose(t *testing.T) {
	testTerminalAfterPeerClose(t, "cancel", websocket.CloseNormalClosure, ErrDeclined)
}

func TestApprovedAfterQueuedPeerClose(t *testing.T) {
	testTerminalAfterPeerClose(t, "pending_login", websocket.CloseNormalClosure, nil)
}

func TestExpiredAfterQueuedPeerClose(t *testing.T) {
	testTerminalAfterPeerClose(t, "", 4003, ErrExpired)
}

func testTerminalAfterPeerClose(t *testing.T, op string, closeCode int, want error) {
	t.Helper()
	for i := 0; i < 10; i++ {
		t.Run(string(rune('A'+i)), func(t *testing.T) {
			peerClosed := make(chan struct{})
			g := newFakeGateway(t, map[string]any{"heartbeat_interval": 1, "timeout_ms": 5000}, func(c *websocket.Conn, pub *rsa.PublicKey) {
				_ = c.WriteJSON(map[string]string{"op": "pending_ticket", "encrypted_user_payload": encryptFor(t, pub, "1:0001:abc:alice")})
				if op != "" {
					_ = c.WriteJSON(map[string]string{"op": op, "ticket": "inert-ticket"})
				}
				_ = c.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(closeCode, ""), time.Now().Add(time.Second))
				_, _, _ = c.ReadMessage()
			})
			exchange := noExchange(t)
			if want == nil {
				exchange = func(context.Context, string, string) (string, error) {
					g.mu.Lock()
					defer g.mu.Unlock()
					return encryptFor(t, g.pub, "inert-token"), nil
				}
			}
			result, err := Run(context.Background(), &terminalRecorder{closed: peerClosed}, Options{
				GatewayURL: g.url(), Exchange: exchange,
				Dial: func(ctx context.Context, url string, h http.Header) (*websocket.Conn, error) {
					c, _, err := websocket.DefaultDialer.DialContext(ctx, url, h)
					if err != nil {
						return nil, err
					}
					original := c.CloseHandler()
					c.SetCloseHandler(func(code int, text string) error {
						err := original(code, text)
						close(peerClosed)
						return err
					})
					return c, nil
				},
			})
			if !errors.Is(err, want) {
				t.Fatalf("terminal event must survive peer close; got %v, want %v", err, want)
			}
			if want == nil && result.Token != "inert-token" {
				t.Fatalf("approved token not exchanged: %+v", result)
			}
		})
	}
}
