package session

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/state"
	"github.com/diamondburned/ningen/v3"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/remoteauth"
)

// scriptedQR is a fake remote-auth flow driven by the test.
type scriptedQR struct {
	steps chan func(ev remoteauth.Events) (done bool, res remoteauth.Result, err error)
}

func newScriptedQR() *scriptedQR {
	return &scriptedQR{steps: make(chan func(remoteauth.Events) (bool, remoteauth.Result, error), 8)}
}

func (s *scriptedQR) run(ctx context.Context, ev remoteauth.Events) (remoteauth.Result, error) {
	for {
		select {
		case <-ctx.Done():
			return remoteauth.Result{}, ctx.Err()
		case step := <-s.steps:
			if done, res, err := step(ev); done {
				return res, err
			}
		}
	}
}

func (s *scriptedQR) code() {
	s.steps <- func(ev remoteauth.Events) (bool, remoteauth.Result, error) {
		ev.QRCode("https://discord.com/ra/fp1", "fp1", 2*time.Minute)
		return false, remoteauth.Result{}, nil
	}
}

func (s *scriptedQR) finish(res remoteauth.Result, err error) {
	s.steps <- func(remoteauth.Events) (bool, remoteauth.Result, error) { return true, res, err }
}

// qrManager is a logged-out manager with the QR seams faked and no network.
func qrManager(t *testing.T) (*Manager, *scriptedQR, *fakeKeyring) {
	t.Helper()
	kr := &fakeKeyring{}
	m := New(kr)
	s := newScriptedQR()
	m.runQR = s.run
	m.qrWait = 200 * time.Millisecond
	m.qrPath = filepath.Join(t.TempDir(), "qr.png")
	m.newState = func(string) *ningen.State {
		return ningen.FromState(state.NewWithIdentifier(gateway.DefaultIdentifier("qr-test-token")))
	}
	m.runLoop = func(ctx context.Context, _ *ningen.State, done chan struct{}) { <-ctx.Done(); close(done) }
	m.Start(context.Background())
	nextEvent(t, m) // logged_out
	return m, s, kr
}

func lifecycleEvent(t *testing.T, m *Manager) string {
	t.Helper()
	return nextEvent(t, m).(protocol.StateChangedEvent).State.Lifecycle
}

func TestQRLoginHappyPath(t *testing.T) {
	m, s, kr := qrManager(t)
	s.code()
	if _, e := m.Handle(context.Background(), req(t, `{"v":1,"id":50,"command":"start_qr_login"}`)); e != nil {
		t.Fatal(e)
	}
	if lc := lifecycleEvent(t, m); lc != protocol.LifecycleQRPending {
		t.Fatalf("lifecycle %s", lc)
	}
	code := nextEvent(t, m).(protocol.QRCodeEvent)
	if code.URL != "https://discord.com/ra/fp1" || code.Fingerprint != "fp1" || code.ExpiresInMS != 120000 || code.ImagePath != m.qrPath {
		t.Fatalf("%+v", code)
	}
	png, err := os.ReadFile(m.qrPath)
	if err != nil || !bytes.HasPrefix(png, []byte("\x89PNG")) {
		t.Fatalf("qr image: %v", err)
	}
	if st, _ := os.Stat(m.qrPath); st.Mode().Perm() != 0o600 {
		t.Fatalf("qr image mode %v", st.Mode())
	}
	// A second start while running, and a token login, are refused.
	if _, e := m.Handle(context.Background(), req(t, `{"v":1,"id":51,"command":"start_qr_login"}`)); e == nil || e.Code != protocol.CodeQRUnavailable {
		t.Fatalf("second start: %v", e)
	}
	if _, e := m.Handle(context.Background(), req(t, `{"v":1,"id":52,"command":"login","token":"x"}`)); e == nil || e.Code != protocol.CodeQRUnavailable {
		t.Fatalf("login during qr: %v", e)
	}

	s.steps <- func(ev remoteauth.Events) (bool, remoteauth.Result, error) {
		ev.Scanned(remoteauth.User{ID: "100000000000000001", Username: "tester", Discriminator: "0", AvatarHash: "abc"})
		ev.Approved()
		return true, remoteauth.Result{Token: "qr-token", User: remoteauth.User{ID: "100000000000000001", Username: "tester", AvatarHash: "abc"}}, nil
	}
	if ev := nextEvent(t, m).(protocol.QRScannedEvent); ev.User.Username != "tester" || ev.User.AvatarHash != "abc" {
		t.Fatalf("%+v", ev)
	}
	if _, ok := nextEvent(t, m).(protocol.QRApprovedEvent); !ok {
		t.Fatal("want qr_approved")
	}
	if lc := lifecycleEvent(t, m); lc != protocol.LifecycleConnecting {
		t.Fatalf("lifecycle %s", lc)
	}
	if len(kr.stores) != 1 || kr.stores[0] != "qr-token" {
		t.Fatalf("keyring stores %v", kr.stores)
	}
	if _, err := os.Stat(m.qrPath); !os.IsNotExist(err) {
		t.Fatal("qr image not removed")
	}
	m.mu.Lock()
	tok, running := m.token, m.qr != nil
	m.mu.Unlock()
	if tok != "qr-token" || running {
		t.Fatalf("token %q running %v", tok, running)
	}
	m.Stop()
}

func TestQRLoginCancel(t *testing.T) {
	m, s, kr := qrManager(t)
	s.code()
	if _, e := m.Handle(context.Background(), req(t, `{"v":1,"id":50,"command":"start_qr_login"}`)); e != nil {
		t.Fatal(e)
	}
	lifecycleEvent(t, m)
	nextEvent(t, m) // qr_code
	if _, e := m.Handle(context.Background(), req(t, `{"v":1,"id":51,"command":"cancel_qr_login"}`)); e != nil {
		t.Fatal(e)
	}
	if ev := nextEvent(t, m).(protocol.QRCancelledEvent); ev.Reason != protocol.QRReasonCancelled || ev.Error != "" {
		t.Fatalf("%+v", ev)
	}
	if lc := lifecycleEvent(t, m); lc != protocol.LifecycleLoggedOut {
		t.Fatalf("lifecycle %s", lc)
	}
	if _, e := m.Handle(context.Background(), req(t, `{"v":1,"id":52,"command":"cancel_qr_login"}`)); e == nil || e.Code != protocol.CodeQRUnavailable {
		t.Fatalf("cancel idle: %v", e)
	}
	if len(kr.stores) != 0 {
		t.Fatal("token stored on cancel")
	}
	if _, err := os.Stat(m.qrPath); !os.IsNotExist(err) {
		t.Fatal("qr image not removed")
	}
}

func TestQRLoginExpiryDeclinedAndError(t *testing.T) {
	m, s, _ := qrManager(t)
	for _, c := range []struct {
		err    error
		reason string
	}{
		{remoteauth.ErrExpired, protocol.QRReasonExpired},
		{remoteauth.ErrDeclined, protocol.QRReasonDeclined},
		{errors.New("ticket exchange failed: token=abc"), protocol.QRReasonError},
	} {
		s.code()
		if _, e := m.Handle(context.Background(), req(t, `{"v":1,"id":50,"command":"start_qr_login"}`)); e != nil {
			t.Fatal(e)
		}
		lifecycleEvent(t, m)
		nextEvent(t, m) // qr_code
		s.finish(remoteauth.Result{}, c.err)
		ev := nextEvent(t, m).(protocol.QRCancelledEvent)
		if ev.Reason != c.reason {
			t.Fatalf("%v: %+v", c.err, ev)
		}
		if c.reason == protocol.QRReasonError && (ev.Error == "" || bytes.Contains([]byte(ev.Error), []byte("abc"))) {
			t.Fatalf("error text %q", ev.Error)
		}
		if c.reason != protocol.QRReasonError && ev.Error != "" {
			t.Fatalf("error text %q", ev.Error)
		}
		if lc := lifecycleEvent(t, m); lc != protocol.LifecycleLoggedOut {
			t.Fatalf("lifecycle %s", lc)
		}
	}
}

func TestQRLoginUnavailable(t *testing.T) {
	m, s, _ := qrManager(t)
	// Dial failure before any code: qr_unavailable, no qr_cancelled.
	s.finish(remoteauth.Result{}, errors.New("dial gateway: refused"))
	if _, e := m.Handle(context.Background(), req(t, `{"v":1,"id":50,"command":"start_qr_login"}`)); e == nil || e.Code != protocol.CodeQRUnavailable {
		t.Fatalf("dial failure: %v", e)
	}
	if lc := lifecycleEvent(t, m); lc != protocol.LifecycleQRPending {
		t.Fatalf("lifecycle %s", lc)
	}
	if lc := lifecycleEvent(t, m); lc != protocol.LifecycleLoggedOut {
		t.Fatalf("lifecycle %s", lc)
	}
	noEvent(t, m)

	// No code within the wait window: qr_unavailable and the flow is torn down.
	start := time.Now()
	if _, e := m.Handle(context.Background(), req(t, `{"v":1,"id":51,"command":"start_qr_login"}`)); e == nil || e.Code != protocol.CodeQRUnavailable {
		t.Fatalf("timeout: %v", e)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("did not time out within the wait window")
	}
	lifecycleEvent(t, m) // qr_pending
	if lc := lifecycleEvent(t, m); lc != protocol.LifecycleLoggedOut {
		t.Fatalf("lifecycle %s", lc)
	}
	if m.qrRunning() {
		t.Fatal("flow still running after timeout")
	}

	// Logged in: refused outright.
	lm, _ := readyManager(t)
	if _, e := lm.Handle(context.Background(), req(t, `{"v":1,"id":52,"command":"start_qr_login"}`)); e == nil || e.Code != protocol.CodeQRUnavailable {
		t.Fatalf("logged in: %v", e)
	}
	noEvent(t, lm)
}
