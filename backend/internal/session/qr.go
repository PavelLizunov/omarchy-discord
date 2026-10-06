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
	qrFirstCodeWait = 10 * time.Second
	qrImageSize     = 512
	qrImageName     = "qr.png"
)

type qrRunner func(ctx context.Context, ev remoteauth.Events) (remoteauth.Result, error)

func liveQR(ctx context.Context, ev remoteauth.Events) (remoteauth.Result, error) {
	return remoteauth.Run(ctx, ev, remoteauth.Options{})
}

type qrFlow struct {
	cancel    context.CancelFunc
	done      chan struct{}
	firstCode chan struct{}

	gotCode                bool
	finishing              bool
	code                   *protocol.QRCodeEvent
	deadline               time.Time
	prevLifecycle, prevErr string
	prevN                  *ningen.State
}

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

func (m *Manager) StartQRLogin(ctx context.Context) *protocol.Error {
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

func (m *Manager) runQRFlow(ctx context.Context, f *qrFlow) {
	defer close(f.done)
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
	if f.gotCode {
		m.push(protocol.NewQRCancelled(reason, text))
	}
	if m.lifecycle == protocol.LifecycleQRPending && m.n == f.prevN {
		m.setLifecycleLocked(f.prevLifecycle, f.prevErr)
	}
	m.mu.Unlock()
}

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
	m.Flush(ctx)
	return nil
}

func (m *Manager) qrRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.qr != nil
}

func defaultNewState(token string) *ningen.State { return ningen.New(token) }

func qrImagePath(runtimeDir string) string {
	if runtimeDir == "" {
		return ""
	}
	return filepath.Join(runtimeDir, qrImageName)
}
