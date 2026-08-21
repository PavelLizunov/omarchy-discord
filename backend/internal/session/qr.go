package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/ningen/v3"

	"github.com/mattcalayo/omarchy-discord/backend/internal/panics"
	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/redact"
	"github.com/mattcalayo/omarchy-discord/backend/internal/remoteauth"
)

const (
	// qrFirstCodeWait bounds how long start_qr_login waits for the gateway to
	// issue a code before reporting qr_unavailable.
	qrFirstCodeWait = 10 * time.Second
	// qrImageSize is the rendered QR PNG edge in pixels.
	qrImageSize = 512
	qrImageName = "qr.png"
)

// qrRunner performs one remote-auth flow; tests inject a fake.
type qrRunner func(ctx context.Context, ev remoteauth.Events) (remoteauth.Result, error)

func liveQR(ctx context.Context, ev remoteauth.Events) (remoteauth.Result, error) {
	return remoteauth.Run(ctx, ev, remoteauth.Options{})
}

// qrFlow is one running QR login. It stays installed as m.qr from
// start_qr_login until the flow has either ended without login or installed
// its session, so no other auth operation can slip in between.
type qrFlow struct {
	cancel context.CancelFunc
	done   chan struct{}
	// firstCode is closed when the gateway issued the first code.
	firstCode chan struct{}

	// The fields below are guarded by Manager.mu.
	gotCode bool
	// finishing is set once the phone approved and the token is being
	// installed; cancel is refused from then on.
	finishing bool
	// code/deadline are the current code, replayed by Snapshot to clients
	// that connect mid-flow.
	code     *protocol.QRCodeEvent
	deadline time.Time
	// prevLifecycle/prevErr/prevN are restored when the flow ends without
	// login, provided nothing else changed the session meanwhile.
	prevLifecycle, prevErr string
	prevN                  *ningen.State
}

// qrEvents adapts remoteauth callbacks to protocol events. Callbacks run on
// the flow goroutine.
type qrEvents struct {
	m *Manager
	f *qrFlow
}

func (q qrEvents) QRCode(url, fingerprint string, expiresIn time.Duration) {
	image := ""
	if q.m.qrPath != "" {
		if png, err := remoteauth.QRPNG(url, qrImageSize); err != nil {
			redact.Logf("session: render QR: %v", err)
		} else if err := os.WriteFile(q.m.qrPath, png, 0o600); err != nil {
			redact.Logf("session: write QR image: %v", err)
		} else {
			image = q.m.qrPath
		}
	}
	ev := protocol.NewQRCode(url, fingerprint, expiresIn.Milliseconds(), image)
	q.m.mu.Lock()
	q.f.code, q.f.deadline = &ev, q.m.now().Add(expiresIn)
	q.m.push(ev)
	first := !q.f.gotCode
	q.f.gotCode = true
	q.m.mu.Unlock()
	if first {
		close(q.f.firstCode)
	}
}

func (q qrEvents) Scanned(u remoteauth.User) {
	q.m.push(protocol.NewQRScanned(protocol.QRUser{ID: u.ID, Username: u.Username, Discriminator: u.Discriminator, AvatarHash: u.AvatarHash}))
}

func (q qrEvents) Approved() { q.m.push(protocol.NewQRApproved()) }

// StartQRLogin implements start_qr_login. The guard-and-install step runs
// under opMu so it cannot interleave with a login or a finishing QR flow;
// the wait for the first code runs outside it (the installed flow is itself
// the guard that refuses other auth operations).
func (m *Manager) StartQRLogin(ctx context.Context) *protocol.Error {
	// Fast refusal while a flow is finishing (it holds opMu for the keyring
	// store + install); the guard is re-checked under opMu below.
	if m.qrRunning() {
		return protocol.Errorf(protocol.CodeQRUnavailable, "a QR login is already in progress")
	}
	m.opMu.Lock()
	m.mu.Lock()
	if m.qr != nil {
		m.mu.Unlock()
		m.opMu.Unlock()
		return protocol.Errorf(protocol.CodeQRUnavailable, "a QR login is already in progress")
	}
	if m.n != nil && m.lifecycle != protocol.LifecycleReauthNeeded {
		m.mu.Unlock()
		m.opMu.Unlock()
		return protocol.Errorf(protocol.CodeQRUnavailable, "already logged in")
	}
	fctx, cancel := context.WithCancel(context.Background())
	f := &qrFlow{cancel: cancel, done: make(chan struct{}), firstCode: make(chan struct{}), prevLifecycle: m.lifecycle, prevErr: m.errText, prevN: m.n}
	m.qr = f
	m.setLifecycleLocked(protocol.LifecycleQRPending, "")
	m.mu.Unlock()
	m.opMu.Unlock()

	panics.Go("session: qr flow", func() { m.runQRFlow(fctx, f) })

	// Every path flushes so the documented events (state_changed, qr_code)
	// reach the socket before the response.
	defer m.Flush(ctx)
	select {
	case <-f.firstCode:
		return nil
	case <-f.done:
		return protocol.Errorf(protocol.CodeQRUnavailable, "remote-auth gateway unavailable")
	case <-time.After(m.qrWait):
		cancel()
		<-f.done
		return protocol.Errorf(protocol.CodeQRUnavailable, "remote-auth gateway did not answer in time")
	case <-ctx.Done():
		cancel()
		<-f.done
		return protocol.Errorf(protocol.CodeQRUnavailable, "request cancelled")
	}
}

// runQRFlow drives one flow to completion and publishes the outcome.
func (m *Manager) runQRFlow(ctx context.Context, f *qrFlow) {
	defer close(f.done)
	// Release the flow context on every exit so nothing derived from it (the
	// remote-auth reader goroutine, timers) outlives the flow.
	defer f.cancel()
	res, err := m.runQR(ctx, qrEvents{m: m, f: f})
	if m.qrPath != "" {
		os.Remove(m.qrPath)
	}
	if err == nil {
		m.finishQRLogin(f, res)
		return
	}
	reason, text := protocol.QRReasonError, err.Error()
	switch {
	case errors.Is(err, remoteauth.ErrDeclined):
		reason, text = protocol.QRReasonDeclined, ""
	case errors.Is(err, remoteauth.ErrExpired):
		reason, text = protocol.QRReasonExpired, ""
	case errors.Is(err, context.Canceled):
		reason, text = protocol.QRReasonCancelled, ""
	default:
		redact.Logf("session: QR login failed: %v", err)
	}
	m.mu.Lock()
	m.qr = nil
	// Before the first code the caller is still waiting on start_qr_login
	// and gets qr_unavailable; the event would be noise.
	if f.gotCode {
		m.push(protocol.NewQRCancelled(reason, text))
	}
	// Only undo our own lifecycle change: if a logout or another session
	// change happened meanwhile, its state stands.
	if m.lifecycle == protocol.LifecycleQRPending && m.n == f.prevN {
		m.setLifecycleLocked(f.prevLifecycle, f.prevErr)
	}
	m.mu.Unlock()
}

// finishQRLogin stores the token and connects exactly like login. The flow
// stays installed (refusing other auth operations) until the session is.
func (m *Manager) finishQRLogin(f *qrFlow, res remoteauth.Result) {
	user := protocol.User{ID: res.User.ID, Username: res.User.Username, DisplayName: res.User.Username}
	if sf, err := discord.ParseSnowflake(res.User.ID); err == nil && res.User.AvatarHash != "" {
		user.AvatarURL = discord.User{ID: discord.UserID(sf), Avatar: discord.Hash(res.User.AvatarHash)}.AvatarURL()
	}
	m.mu.Lock()
	f.finishing = true
	m.mu.Unlock()

	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.finishLogin(context.Background(), m.newState(res.Token), res.Token, user)
	m.mu.Lock()
	m.qr = nil
	m.mu.Unlock()
}

// CancelQRLogin implements cancel_qr_login. It refuses once the phone has
// approved and the token is being installed.
func (m *Manager) CancelQRLogin(ctx context.Context) *protocol.Error {
	m.mu.Lock()
	f := m.qr
	finishing := f != nil && f.finishing
	m.mu.Unlock()
	if f == nil {
		return protocol.Errorf(protocol.CodeQRUnavailable, "no QR login in progress")
	}
	if finishing {
		return protocol.Errorf(protocol.CodeQRUnavailable, "QR login already approved; logging in")
	}
	f.cancel()
	<-f.done
	// qr_cancelled and the lifecycle state_changed precede the response.
	m.Flush(ctx)
	return nil
}

// qrRunning reports whether a QR flow is active.
func (m *Manager) qrRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.qr != nil
}

// newState constructs a session for a token; tests replace it.
func defaultNewState(token string) *ningen.State { return ningen.New(token) }

// qrImagePath is where the rendered QR lands, inside the runtime dir.
func qrImagePath(runtimeDir string) string {
	if runtimeDir == "" {
		return ""
	}
	return filepath.Join(runtimeDir, qrImageName)
}
