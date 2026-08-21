package remoteauth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image/png"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// fakeGateway is an in-process remote-auth gateway. After the handshake it
// runs scenario with the client's public key.
type fakeGateway struct {
	t          *testing.T
	srv        *httptest.Server
	hello      map[string]any
	scenario   func(c *websocket.Conn, pub *rsa.PublicKey)
	heartbeats atomic.Int32
	hdr        http.Header
	mu         sync.Mutex
	pub        *rsa.PublicKey
}

func newFakeGateway(t *testing.T, hello map[string]any, scenario func(c *websocket.Conn, pub *rsa.PublicKey)) *fakeGateway {
	g := &fakeGateway{t: t, hello: hello, scenario: scenario}
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	g.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		g.hdr = r.Header.Clone()
		g.mu.Unlock()
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		g.handle(c)
	}))
	t.Cleanup(g.srv.Close)
	return g
}

func (g *fakeGateway) url() string { return "ws" + strings.TrimPrefix(g.srv.URL, "http") }

// readOp reads messages until one that isn't a heartbeat, which it counts and acks.
func (g *fakeGateway) readOp(c *websocket.Conn, want string) map[string]string {
	for {
		var m map[string]string
		if err := c.ReadJSON(&m); err != nil {
			return nil
		}
		if m["op"] == "heartbeat" {
			g.heartbeats.Add(1)
			_ = c.WriteJSON(map[string]string{"op": "heartbeat_ack"})
			continue
		}
		if m["op"] != want {
			g.t.Errorf("gateway: want op %q, got %q", want, m["op"])
			return nil
		}
		return m
	}
}

func encryptFor(t *testing.T, pub *rsa.PublicKey, plain string) string {
	t.Helper()
	ct, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, pub, []byte(plain), nil)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(ct)
}

func (g *fakeGateway) handle(c *websocket.Conn) {
	hello := map[string]any{"op": "hello"}
	for k, v := range g.hello {
		hello[k] = v
	}
	if err := c.WriteJSON(hello); err != nil {
		return
	}
	init := g.readOp(c, "init")
	if init == nil {
		return
	}
	spki, err := base64.StdEncoding.DecodeString(init["encoded_public_key"])
	if err != nil {
		g.t.Error(err)
		return
	}
	pubAny, err := x509.ParsePKIXPublicKey(spki)
	if err != nil {
		g.t.Error(err)
		return
	}
	pub := pubAny.(*rsa.PublicKey)
	g.mu.Lock()
	g.pub = pub
	g.mu.Unlock()

	nonce := make([]byte, 32)
	_, _ = rand.Read(nonce)
	ct, _ := rsa.EncryptOAEP(sha256.New(), rand.Reader, pub, nonce, nil)
	_ = c.WriteJSON(map[string]string{"op": "nonce_proof", "encrypted_nonce": base64.StdEncoding.EncodeToString(ct)})
	proof := g.readOp(c, "nonce_proof")
	if proof == nil {
		return
	}
	if proof["nonce"] != base64.RawURLEncoding.EncodeToString(nonce) {
		g.t.Errorf("bad nonce proof")
		_ = c.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(4001, ""), time.Now().Add(time.Second))
		return
	}
	sum := sha256.Sum256(spki)
	_ = c.WriteJSON(map[string]string{"op": "pending_remote_init", "fingerprint": base64.RawURLEncoding.EncodeToString(sum[:])})
	if g.scenario != nil {
		g.scenario(c, pub)
	}
}

// drainFor reads (and acks) heartbeats for d.
func (g *fakeGateway) drainFor(c *websocket.Conn, d time.Duration) {
	_ = c.SetReadDeadline(time.Now().Add(d))
	defer c.SetReadDeadline(time.Time{})
	for {
		var m map[string]string
		if err := c.ReadJSON(&m); err != nil {
			return
		}
		if m["op"] == "heartbeat" {
			g.heartbeats.Add(1)
			_ = c.WriteJSON(map[string]string{"op": "heartbeat_ack"})
		}
	}
}

func closeNormal(c *websocket.Conn) {
	_ = c.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
	// Keep the connection open briefly so the client reads the close frame.
	_, _, _ = c.ReadMessage()
}

type recorder struct {
	mu       sync.Mutex
	url, fp  string
	expires  time.Duration
	scanned  []User
	approved int
}

func (r *recorder) QRCode(url, fp string, exp time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.url, r.fp, r.expires = url, fp, exp
}
func (r *recorder) Scanned(u User) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scanned = append(r.scanned, u)
}
func (r *recorder) Approved() { r.mu.Lock(); defer r.mu.Unlock(); r.approved++ }

var fastHello = map[string]any{"heartbeat_interval": 20, "timeout_ms": 5000}

func TestRunHappyPath(t *testing.T) {
	const ticket, token = "ODUy.ticket.value", "mfa.the-secret-token"
	var g *fakeGateway
	g = newFakeGateway(t, fastHello, func(c *websocket.Conn, pub *rsa.PublicKey) {
		g.drainFor(c, 80*time.Millisecond) // let a few heartbeats through
		_ = c.WriteJSON(map[string]string{"op": "pending_ticket", "encrypted_user_payload": encryptFor(t, pub, "852892297661906993:0:0:dolfies")})
		_ = c.WriteJSON(map[string]string{"op": "pending_login", "ticket": ticket})
		closeNormal(c)
	})
	rec := &recorder{}
	var gotTicket, gotFP string
	res, err := Run(context.Background(), rec, Options{
		GatewayURL: g.url(),
		UserAgent:  "test-ua",
		Exchange: func(ctx context.Context, tk, fp string) (string, error) {
			gotTicket, gotFP = tk, fp
			g.mu.Lock()
			pub := g.pub
			g.mu.Unlock()
			return encryptFor(t, pub, token), nil
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Token != token {
		t.Errorf("token mismatch")
	}
	want := User{ID: "852892297661906993", Discriminator: "0", Username: "dolfies"}
	if res.User != want {
		t.Errorf("user = %+v, want %+v", res.User, want)
	}
	if gotTicket != ticket || gotFP != rec.fp || rec.fp == "" {
		t.Errorf("exchange got ticket=%q fp=%q (event fp=%q)", gotTicket, gotFP, rec.fp)
	}
	if rec.url != "https://discord.com/ra/"+rec.fp {
		t.Errorf("qr url = %q", rec.url)
	}
	if rec.expires != 5*time.Second {
		t.Errorf("expires = %v", rec.expires)
	}
	if len(rec.scanned) != 1 || rec.scanned[0] != want || rec.approved != 1 {
		t.Errorf("events: scanned=%v approved=%d", rec.scanned, rec.approved)
	}
	if g.heartbeats.Load() == 0 {
		t.Errorf("no heartbeats received")
	}
	g.mu.Lock()
	hdr := g.hdr
	g.mu.Unlock()
	if hdr.Get("Origin") != Origin || hdr.Get("User-Agent") != "test-ua" {
		t.Errorf("dial headers = %v", hdr)
	}
}

func TestRunDeclined(t *testing.T) {
	g := newFakeGateway(t, fastHello, func(c *websocket.Conn, pub *rsa.PublicKey) {
		_ = c.WriteJSON(map[string]string{"op": "pending_ticket", "encrypted_user_payload": encryptFor(t, pub, "1:0001:abc:alice")})
		_ = c.WriteJSON(map[string]string{"op": "cancel"})
		closeNormal(c)
	})
	rec := &recorder{}
	_, err := Run(context.Background(), rec, Options{GatewayURL: g.url(), Exchange: noExchange(t)})
	if !errors.Is(err, ErrDeclined) {
		t.Fatalf("err = %v, want ErrDeclined", err)
	}
	if len(rec.scanned) != 1 || rec.scanned[0].AvatarHash != "abc" || rec.approved != 0 {
		t.Errorf("events: %+v", rec)
	}
}

func TestRunExpiredByTimeout(t *testing.T) {
	g := newFakeGateway(t, map[string]any{"heartbeat_interval": 20, "timeout_ms": 150}, func(c *websocket.Conn, _ *rsa.PublicKey) {
		_, _, _ = c.ReadMessage() // sit idle until the client closes
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	})
	start := time.Now()
	_, err := Run(context.Background(), &recorder{}, Options{GatewayURL: g.url(), Exchange: noExchange(t)})
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("err = %v, want ErrExpired", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("took too long: %v", time.Since(start))
	}
}

func TestRunExpiredByServerClose(t *testing.T) {
	g := newFakeGateway(t, fastHello, func(c *websocket.Conn, _ *rsa.PublicKey) {
		_ = c.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(4003, "timeout"), time.Now().Add(time.Second))
		_, _, _ = c.ReadMessage()
	})
	_, err := Run(context.Background(), &recorder{}, Options{GatewayURL: g.url(), Exchange: noExchange(t)})
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("err = %v, want ErrExpired", err)
	}
}

func TestRunContextCancel(t *testing.T) {
	released := make(chan struct{})
	g := newFakeGateway(t, fastHello, func(c *websocket.Conn, _ *rsa.PublicKey) {
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				close(released)
				return
			}
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	rec := &recorder{}
	done := make(chan error, 1)
	go func() {
		_, err := Run(ctx, rec, Options{GatewayURL: g.url(), Exchange: noExchange(t)})
		done <- err
	}()
	// Wait for the QR to be issued, then cancel.
	deadline := time.Now().Add(2 * time.Second)
	for {
		rec.mu.Lock()
		fp := rec.fp
		rec.mu.Unlock()
		if fp != "" || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	select {
	case <-released:
	case <-time.After(2 * time.Second):
		t.Fatal("websocket was not closed after cancel")
	}
}

func TestRunBadFingerprintRejected(t *testing.T) {
	// A gateway that lies about the fingerprint must be rejected.
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.WriteJSON(map[string]any{"op": "hello", "heartbeat_interval": 1000, "timeout_ms": 5000})
		var init map[string]string
		_ = c.ReadJSON(&init)
		spki, _ := base64.StdEncoding.DecodeString(init["encoded_public_key"])
		pub, _ := x509.ParsePKIXPublicKey(spki)
		ct, _ := rsa.EncryptOAEP(sha256.New(), rand.Reader, pub.(*rsa.PublicKey), []byte("n"), nil)
		_ = c.WriteJSON(map[string]string{"op": "nonce_proof", "encrypted_nonce": base64.StdEncoding.EncodeToString(ct)})
		_ = c.ReadJSON(&init)
		_ = c.WriteJSON(map[string]string{"op": "pending_remote_init", "fingerprint": "bogus"})
		_, _, _ = c.ReadMessage()
	}))
	defer srv.Close()
	_, err := Run(context.Background(), &recorder{}, Options{GatewayURL: "ws" + strings.TrimPrefix(srv.URL, "http"), Exchange: noExchange(t)})
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("err = %v, want ErrProtocol", err)
	}
}

func TestExchangeErrorIsRedacted(t *testing.T) {
	g := newFakeGateway(t, fastHello, func(c *websocket.Conn, pub *rsa.PublicKey) {
		_ = c.WriteJSON(map[string]string{"op": "pending_login", "ticket": "SECRET-TICKET"})
		closeNormal(c)
	})
	_, err := Run(context.Background(), &recorder{}, Options{
		GatewayURL: g.url(),
		Exchange: func(context.Context, string, string) (string, error) {
			return "", errors.New("boom SECRET-TICKET")
		},
	})
	if !errors.Is(err, ErrExchange) || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("err = %v", err)
	}
}

func noExchange(t *testing.T) func(context.Context, string, string) (string, error) {
	return func(context.Context, string, string) (string, error) {
		t.Errorf("exchange must not be called")
		return "", errors.New("unexpected")
	}
}

func TestQRPNG(t *testing.T) {
	data, err := QRPNG("https://discord.com/ra/UZ0-kOVzXDZTFVV5_QlpURSO2BQHrtkKWHNpIGoDI0k", 256)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 256 || b.Dy() != 256 {
		t.Errorf("size = %v", b)
	}
	if _, err := QRPNG("x", 0); err == nil {
		t.Error("expected error for size 0")
	}
}

// Guard against accidental JSON field drift.
func TestGatewayMsgFields(t *testing.T) {
	var m gatewayMsg
	if err := json.Unmarshal([]byte(`{"op":"hello","heartbeat_interval":41250,"timeout_ms":142637}`), &m); err != nil {
		t.Fatal(err)
	}
	if m.HeartbeatInterval != 41250 || m.TimeoutMS != 142637 {
		t.Errorf("%+v", m)
	}
}

// readerGoroutines counts live goroutines parked in the gateway reader.
func readerGoroutines() int {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	return strings.Count(string(buf[:n]), "remoteauth.(*flow).run.func1")
}

// TestReaderGoroutineExitsWithoutCancel: when Run ends on its own (expiry
// here) with a message still buffered and a never-cancelled context, the
// reader goroutine must still exit once the socket is closed.
func TestReaderGoroutineExitsWithoutCancel(t *testing.T) {
	g := newFakeGateway(t, map[string]any{"heartbeat_interval": 20, "timeout_ms": 150}, func(c *websocket.Conn, _ *rsa.PublicKey) {
		// Flood unknown ops so the reader always has a message in hand when
		// the expiry fires.
		for {
			if err := c.WriteJSON(map[string]string{"op": "noise"}); err != nil {
				return
			}
		}
	})
	_, err := Run(context.Background(), &recorder{}, Options{GatewayURL: g.url(), Exchange: noExchange(t)})
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("err = %v, want ErrExpired", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for readerGoroutines() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("gateway reader goroutine leaked")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
