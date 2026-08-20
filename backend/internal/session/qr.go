package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/ningen/v3"

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

// qrFlow is one running QR login.
type qrFlow struct {
	cancel context.CancelFunc
	done   chan struct{}
	// firstCode is closed when the gateway issued the first code.
	firstCode chan struct{}
	gotCode   bool
	// prevLifecycle/prevErr are restored when the flow ends without login.
	prevLifecycle, prevErr string
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
	q.m.push(protocol.NewQRCode(url, fingerprint, expiresIn.Milliseconds(), image))
	if !q.f.gotCode {
		q.f.gotCode = true
		close(q.f.firstCode)
	}
}

func (q qrEvents) Scanned(u remoteauth.User) {
	q.m.push(protocol.NewQRScanned(protocol.QRUser{ID: u.ID, Username: u.Username, Discriminator: u.Discriminator, AvatarHash: u.AvatarHash}))
}

func (q qrEvents) Approved() { q.m.push(protocol.NewQRApproved()) }

// StartQRLogin implements start_qr_login.
func (m *Manager) StartQRLogin(ctx context.Context) *protocol.Error {
	m.mu.Lock()
	if m.qr != nil {
		m.mu.Unlock()
		return protocol.Errorf(protocol.CodeQRUnavailable, "a QR login is already in progress")
	}
	if m.n != nil && m.lifecycle != protocol.LifecycleReauthNeeded {
		m.mu.Unlock()
		return protocol.Errorf(protocol.CodeQRUnavailable, "already logged in")
	}
	fctx, cancel := context.WithCancel(context.Background())
	f := &qrFlow{cancel: cancel, done: make(chan struct{}), firstCode: make(chan struct{}), prevLifecycle: m.lifecycle, prevErr: m.errText}
	m.qr = f
	m.setLifecycleLocked(protocol.LifecycleQRPending, "")
	m.mu.Unlock()

	go m.runQRFlow(fctx, f)

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
	m.setLifecycleLocked(f.prevLifecycle, f.prevErr)
	m.mu.Unlock()
}

// finishQRLogin stores the token and connects exactly like login.
func (m *Manager) finishQRLogin(f *qrFlow, res remoteauth.Result) {
	user := protocol.User{ID: res.User.ID, Username: res.User.Username, DisplayName: res.User.Username}
	if sf, err := discord.ParseSnowflake(res.User.ID); err == nil && res.User.AvatarHash != "" {
		user.AvatarURL = discord.User{ID: discord.UserID(sf), Avatar: discord.Hash(res.User.AvatarHash)}.AvatarURL()
	}
	m.mu.Lock()
	m.qr = nil
	m.mu.Unlock()
	m.finishLogin(context.Background(), m.newState(res.Token), res.Token, user)
}

// CancelQRLogin implements cancel_qr_login.
func (m *Manager) CancelQRLogin() *protocol.Error {
	m.mu.Lock()
	f := m.qr
	m.mu.Unlock()
	if f == nil {
		return protocol.Errorf(protocol.CodeQRUnavailable, "no QR login in progress")
	}
	f.cancel()
	<-f.done
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
