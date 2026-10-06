package session

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/state"
	"github.com/diamondburned/ningen/v3"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/remoteauth"
)

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
	nextEvent(t, m)
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
	nextEvent(t, m)
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
		nextEvent(t, m)
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

	start := time.Now()
	if _, e := m.Handle(context.Background(), req(t, `{"v":1,"id":51,"command":"start_qr_login"}`)); e == nil || e.Code != protocol.CodeQRUnavailable {
		t.Fatalf("timeout: %v", e)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("did not time out within the wait window")
	}
	lifecycleEvent(t, m)
	if lc := lifecycleEvent(t, m); lc != protocol.LifecycleLoggedOut {
		t.Fatalf("lifecycle %s", lc)
	}
	if m.qrRunning() {
		t.Fatal("flow still running after timeout")
	}

	lm, _ := readyManager(t)
	if _, e := lm.Handle(context.Background(), req(t, `{"v":1,"id":52,"command":"start_qr_login"}`)); e == nil || e.Code != protocol.CodeQRUnavailable {
		t.Fatalf("logged in: %v", e)
	}
	noEvent(t, lm)
}

type gatedKeyring struct {
	fakeKeyring
	entered chan struct{}
	release chan struct{}
}

func (g *gatedKeyring) Store(ctx context.Context, t string) error {
	close(g.entered)
	<-g.release
	return g.fakeKeyring.Store(ctx, t)
}

func TestQRFinishWindowRefusesOtherAuth(t *testing.T) {
	m, s, _ := qrManager(t)
	kr := &gatedKeyring{entered: make(chan struct{}), release: make(chan struct{})}
	m.kr = kr
	s.code()
	if _, e := m.Handle(context.Background(), req(t, `{"v":1,"id":50,"command":"start_qr_login"}`)); e != nil {
		t.Fatal(e)
	}
	lifecycleEvent(t, m)
	nextEvent(t, m)
	s.finish(remoteauth.Result{Token: "qr-token", User: remoteauth.User{ID: "1", Username: "u"}}, nil)
	<-kr.entered

	for _, c := range []string{"start_qr_login", "cancel_qr_login", `login","token":"x`} {
		done := make(chan *protocol.Error, 1)
		go func() {
			_, e := m.Handle(context.Background(), req(t, `{"v":1,"id":51,"command":"`+c+`"}`))
			done <- e
		}()
		select {
		case e := <-done:
			if e == nil || e.Code != protocol.CodeQRUnavailable {
				t.Fatalf("%s during finish: %v", c, e)
			}
		case <-time.After(time.Second):
			if !strings.HasPrefix(c, "login") {
				t.Fatalf("%s blocked during finish", c)
			}
			close(kr.release)
			if e := <-done; e == nil || e.Code != protocol.CodeQRUnavailable {
				t.Fatalf("%s after finish: %v", c, e)
			}
		}
	}
	select {
	case <-kr.release:
	default:
		close(kr.release)
	}
	if lc := lifecycleEvent(t, m); lc != protocol.LifecycleConnecting {
		t.Fatalf("lifecycle %s", lc)
	}
	noEvent(t, m)
	m.mu.Lock()
	tok, running, lc := m.token, m.qr != nil, m.lifecycle
	m.mu.Unlock()
	if tok != "qr-token" || running || lc != protocol.LifecycleConnecting {
		t.Fatalf("token %q running %v lifecycle %s", tok, running, lc)
	}
	m.Stop()
}

func TestQRCancelAfterForeignStateChangeKeepsIt(t *testing.T) {
	m, s, _ := qrManager(t)
	s.code()
	if _, e := m.Handle(context.Background(), req(t, `{"v":1,"id":50,"command":"start_qr_login"}`)); e != nil {
		t.Fatal(e)
	}
	lifecycleEvent(t, m)
	nextEvent(t, m)
	m.opMu.Lock()
	m.replaceSession(m.newState("other"), "other")
	m.opMu.Unlock()
	if lc := lifecycleEvent(t, m); lc != protocol.LifecycleConnecting {
		t.Fatalf("lifecycle %s", lc)
	}
	if _, e := m.Handle(context.Background(), req(t, `{"v":1,"id":51,"command":"cancel_qr_login"}`)); e != nil {
		t.Fatal(e)
	}
	if _, ok := nextEvent(t, m).(protocol.QRCancelledEvent); !ok {
		t.Fatal("want qr_cancelled")
	}
	noEvent(t, m)
	m.mu.Lock()
	lc := m.lifecycle
	m.mu.Unlock()
	if lc != protocol.LifecycleConnecting {
		t.Fatalf("lifecycle %s after cancel", lc)
	}
	m.Stop()
}

func TestSnapshotReplaysQRCode(t *testing.T) {
	m, s, _ := qrManager(t)
	now := time.Unix(1_000_000, 0)
	m.now = func() time.Time { return now }
	s.code()
	if _, e := m.Handle(context.Background(), req(t, `{"v":1,"id":50,"command":"start_qr_login"}`)); e != nil {
		t.Fatal(e)
	}
	lifecycleEvent(t, m)
	nextEvent(t, m)
	now = now.Add(30 * time.Second)
	snap := m.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("snapshot %+v", snap)
	}
	if st := snap[0].(protocol.StateChangedEvent).State; st.Lifecycle != protocol.LifecycleQRPending {
		t.Fatalf("%+v", st)
	}
	code := snap[1].(protocol.QRCodeEvent)
	if code.URL != "https://discord.com/ra/fp1" || code.Fingerprint != "fp1" || code.ExpiresInMS != 90000 || code.ImagePath != m.qrPath {
		t.Fatalf("%+v", code)
	}
	now = now.Add(5 * time.Minute)
	if code := m.Snapshot()[1].(protocol.QRCodeEvent); code.ExpiresInMS != 0 {
		t.Fatalf("past deadline: %+v", code)
	}
	if _, e := m.Handle(context.Background(), req(t, `{"v":1,"id":51,"command":"cancel_qr_login"}`)); e != nil {
		t.Fatal(e)
	}
	nextEvent(t, m)
	lifecycleEvent(t, m)
	if snap := m.Snapshot(); len(snap) != 1 {
		t.Fatalf("snapshot after cancel %+v", snap)
	}
}

func forwarded(m *Manager) func() []any {
	var mu sync.Mutex
	var got []any
	go m.Forward(func(ev any) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, ev)
	})
	return func() []any {
		mu.Lock()
		defer mu.Unlock()
		return append([]any(nil), got...)
	}
}

func eventNames(evs []any) string {
	var names []string
	for _, ev := range evs {
		switch e := ev.(type) {
		case protocol.StateChangedEvent:
			names = append(names, "state_changed:"+e.State.Lifecycle)
		case protocol.QRCodeEvent:
			names = append(names, "qr_code")
		case protocol.QRCancelledEvent:
			names = append(names, "qr_cancelled")
		default:
			names = append(names, "?")
		}
	}
	return strings.Join(names, ",")
}

func TestQREventsPrecedeResponse(t *testing.T) {
	m, s, _ := qrManager(t)
	sink := forwarded(m)
	s.code()
	if _, e := m.Handle(context.Background(), req(t, `{"v":1,"id":50,"command":"start_qr_login"}`)); e != nil {
		t.Fatal(e)
	}
	if got := eventNames(sink()); got != "state_changed:qr_pending,qr_code" {
		t.Fatalf("events at start response: %s", got)
	}
	if _, e := m.Handle(context.Background(), req(t, `{"v":1,"id":51,"command":"cancel_qr_login"}`)); e != nil {
		t.Fatal(e)
	}
	if got := eventNames(sink()); got != "state_changed:qr_pending,qr_code,qr_cancelled,state_changed:logged_out" {
		t.Fatalf("events at cancel response: %s", got)
	}
	s.finish(remoteauth.Result{}, errors.New("dial gateway: refused"))
	if _, e := m.Handle(context.Background(), req(t, `{"v":1,"id":52,"command":"start_qr_login"}`)); e == nil || e.Code != protocol.CodeQRUnavailable {
		t.Fatalf("dial failure: %v", e)
	}
	if got := eventNames(sink()); !strings.HasSuffix(got, "state_changed:qr_pending,state_changed:logged_out") {
		t.Fatalf("events at failed start response: %s", got)
	}
}

func TestConcurrentLoginAndQRLeaveOneSession(t *testing.T) {
	m, s, _ := qrManager(t)
	installed := 0
	var installMu sync.Mutex
	m.runLoop = func(ctx context.Context, _ *ningen.State, done chan struct{}) {
		installMu.Lock()
		installed++
		installMu.Unlock()
		<-ctx.Done()
		close(done)
	}
	s.code()
	if _, e := m.Handle(context.Background(), req(t, `{"v":1,"id":50,"command":"start_qr_login"}`)); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		s.finish(remoteauth.Result{Token: "qr-token", User: remoteauth.User{ID: "1", Username: "u"}}, nil)
	}()
	var loginErr *protocol.Error
	go func() {
		defer wg.Done()
		_, loginErr = m.Handle(context.Background(), req(t, `{"v":1,"id":51,"command":"login","token":"x"}`))
	}()
	wg.Wait()
	if loginErr == nil || (loginErr.Code != protocol.CodeQRUnavailable && loginErr.Code != protocol.CodeLoginFailed) {
		t.Fatalf("login: %v", loginErr)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		m.mu.Lock()
		running, tok := m.qr != nil, m.token
		m.mu.Unlock()
		installMu.Lock()
		n := installed
		installMu.Unlock()
		if !running && tok == "qr-token" && n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("running %v token %q", running, tok)
		}
		time.Sleep(5 * time.Millisecond)
	}
	installMu.Lock()
	n := installed
	installMu.Unlock()
	if n != 1 {
		t.Fatalf("installed %d sessions", n)
	}
	m.Stop()
}
