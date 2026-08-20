// Package session owns the Discord session: ningen/arikawa lifecycle, the
// keyring-backed token, and the mapping from cache state to wire objects.
package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	arikawasession "github.com/diamondburned/arikawa/v3/session"
	"github.com/diamondburned/ningen/v3"
	"github.com/diamondburned/ningen/v3/states/read"

	"github.com/mattcalayo/omarchy-discord/backend/internal/keyring"
	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/redact"
)

// ConfigureIdentity sets the dissent-parity identify fingerprint. It must run
// before any ningen/arikawa state is constructed.
func ConfigureIdentity() {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "PC"
	}
	api.UserAgent = "OmarchyDiscord (https://github.com/mattcalayo/omarchy-discord)"
	gateway.DefaultIdentity = gateway.IdentifyProperties{
		gateway.IdentifyOS:      runtime.GOOS,
		gateway.IdentifyDevice:  "Arikawa",
		gateway.IdentifyBrowser: "Omarchy Discord on " + host,
	}
}

// Keyring is the subset of keyring.Keyring the manager needs.
type Keyring interface {
	Lookup(ctx context.Context) (string, error)
	Store(ctx context.Context, token string) error
	Clear(ctx context.Context) error
}

// Manager drives one user session and publishes protocol events.
type Manager struct {
	kr     Keyring
	events chan any

	mu         sync.Mutex
	lifecycle  string
	user       *protocol.User
	presence   string
	mentions   int
	unreadDM   *string
	generation int64
	errText    string

	token     string
	n         *ningen.State
	everReady bool
	cancel    context.CancelFunc
	loopDone  chan struct{}
}

// New creates a manager in the `starting` lifecycle.
func New(kr Keyring) *Manager {
	return &Manager{kr: kr, events: make(chan any, 1024), lifecycle: protocol.LifecycleStarting, generation: 1}
}

// Events yields state_changed / guilds_synced events in the order they were
// produced. The consumer forwards them to the socket fan-out.
func (m *Manager) Events() <-chan any { return m.events }

func (m *Manager) push(ev any) {
	select {
	case m.events <- ev:
	default:
		redact.Logf("session: event queue full, dropping %T", ev)
	}
}

// stateLocked builds the wire state. Caller holds mu.
func (m *Manager) stateLocked() protocol.State {
	return protocol.State{
		ProtocolVersion:   protocol.Version,
		BackendVersion:    protocol.BackendVersion,
		Lifecycle:         m.lifecycle,
		User:              m.user,
		Presence:          m.presence,
		TotalMentionCount: m.mentions,
		UnreadDMChannelID: m.unreadDM,
		Generation:        m.generation,
		Error:             redact.Redact(m.errText),
	}
}

// bump records a state change and queues state_changed. Caller holds mu.
func (m *Manager) bump() {
	m.generation++
	m.push(protocol.NewStateChanged(m.stateLocked()))
}

func (m *Manager) setLifecycleLocked(lc, errText string) {
	if m.lifecycle == lc && m.errText == errText {
		return
	}
	m.lifecycle = lc
	m.errText = errText
	if lc != protocol.LifecycleReady {
		m.presence = ""
	}
	if lc == protocol.LifecycleLoggedOut || lc == protocol.LifecycleReauthNeeded {
		m.user = nil
	}
	m.bump()
}

// Start resolves the keyring token and connects if one exists.
func (m *Manager) Start(ctx context.Context) {
	tok, err := m.kr.Lookup(ctx)
	m.mu.Lock()
	defer m.mu.Unlock()
	switch {
	case err == nil:
		redact.Logf("session: token found in keyring, connecting")
		m.connectLocked(ningen.New(tok), tok)
	case errors.Is(err, keyring.ErrNotFound):
		redact.Logf("session: no token in keyring")
		m.setLifecycleLocked(protocol.LifecycleLoggedOut, "")
	default:
		redact.Logf("session: keyring lookup failed: %v", err)
		m.setLifecycleLocked(protocol.LifecycleError, fmt.Sprintf("keyring unavailable: %v", err))
	}
}

// Stop closes the gateway and waits for the connect loop.
func (m *Manager) Stop() {
	m.mu.Lock()
	n, done := m.teardownLocked()
	m.mu.Unlock()
	closeAndWait(n, done)
}

// teardownLocked cancels the loop and detaches the session. Caller holds mu;
// the returned session must be closed outside the lock.
func (m *Manager) teardownLocked() (*ningen.State, chan struct{}) {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	n, done := m.n, m.loopDone
	m.n, m.loopDone, m.token, m.everReady = nil, nil, "", false
	return n, done
}

func closeAndWait(n *ningen.State, done chan struct{}) {
	if n != nil {
		if err := n.Close(); err != nil && !errors.Is(err, arikawasession.ErrClosed) {
			redact.Logf("session: close: %v", err)
		}
	}
	if done != nil {
		<-done
	}
}

// connectLocked installs n as the live session and starts the connect loop.
func (m *Manager) connectLocked(n *ningen.State, token string) {
	m.n, m.token, m.everReady = n, token, false
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.loopDone = make(chan struct{})
	m.installHandlers(n)
	m.setLifecycleLocked(protocol.LifecycleConnecting, "")
	go m.loop(ctx, n, m.loopDone)
}

func (m *Manager) installHandlers(n *ningen.State) {
	// Sync handlers run inside ningen's dispatch after its sub-states updated,
	// so caches are consistent here. Keep them cheap and never block on the
	// network: structure is read through Offline().
	n.AddSyncHandler(func(*ningen.ConnectedEvent) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.n != n {
			return
		}
		off := n.Offline()
		if me, err := off.Cabinet.Me(); err == nil {
			u := wireUser(*me)
			m.user = &u
		}
		m.presence = presence(off)
		m.mentions = off.ReadState.TotalMentionCount()
		m.unreadDM = UnreadDM(off)
		m.everReady = true
		m.lifecycle, m.errText = protocol.LifecycleReady, ""
		m.bump()
		m.pushStructureLocked(off)
	})
	n.AddSyncHandler(func(ev *ningen.DisconnectedEvent) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.n != n {
			return
		}
		if ev.IsLoggedOut() {
			// The loop observes the fatal close and finishes the reauth
			// cleanup (token drop, keyring clear) outside this handler.
			m.setLifecycleLocked(protocol.LifecycleReauthNeeded, fmt.Sprintf("gateway closed session (code %d)", ev.Code))
			return
		}
		if m.lifecycle == protocol.LifecycleReady {
			m.setLifecycleLocked(protocol.LifecycleConnecting, "")
		}
	})
	n.AddSyncHandler(func(*read.UpdateEvent) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.n != n || m.lifecycle != protocol.LifecycleReady {
			return
		}
		off := n.Offline()
		mentions, dm := off.ReadState.TotalMentionCount(), UnreadDM(off)
		if mentions == m.mentions && ptrEq(dm, m.unreadDM) {
			return
		}
		m.mentions, m.unreadDM = mentions, dm
		m.bump()
	})
	resync := func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.n != n || m.lifecycle != protocol.LifecycleReady {
			return
		}
		m.pushStructureLocked(n.Offline())
	}
	n.AddSyncHandler(func(*gateway.GuildCreateEvent) { resync() })
	n.AddSyncHandler(func(*gateway.GuildDeleteEvent) { resync() })
}

func (m *Manager) pushStructureLocked(n *ningen.State) {
	guilds, err := Guilds(n)
	if err != nil {
		redact.Logf("session: guilds: %v", err)
	}
	dms, err := DMs(n)
	if err != nil {
		redact.Logf("session: dms: %v", err)
	}
	m.push(protocol.NewGuildsSynced(guilds, dms))
}

// loop opens the gateway and keeps it open until ctx is cancelled or the
// session is fatally closed (token invalid).
func (m *Manager) loop(ctx context.Context, n *ningen.State, done chan struct{}) {
	defer close(done)
	const minBackoff, maxBackoff = 2 * time.Second, 60 * time.Second
	backoff := minBackoff
	for {
		err := n.Open(ctx)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			backoff = minBackoff
			err = n.Wait(ctx)
			if ctx.Err() != nil {
				return
			}
		}
		if err != nil && n.GatewayOpts().ErrorIsFatalClose(err) {
			redact.Logf("session: fatal gateway close: %v", err)
			m.reauth(n, err)
			return
		}
		redact.Logf("session: gateway down (%v), retrying in %s", err, backoff)
		m.mu.Lock()
		if m.n == n && m.lifecycle != protocol.LifecycleReauthNeeded {
			m.setLifecycleLocked(protocol.LifecycleConnecting, "")
		}
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff *= 2; backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// reauth drops the token (memory + keyring) and keeps the cache for read-only
// structure queries until the next login.
func (m *Manager) reauth(n *ningen.State, cause error) {
	m.mu.Lock()
	if m.n != n {
		m.mu.Unlock()
		return
	}
	m.token = ""
	m.cancel = nil
	m.setLifecycleLocked(protocol.LifecycleReauthNeeded, fmt.Sprintf("session invalidated: %v", cause))
	m.mu.Unlock()
	if err := m.kr.Clear(context.Background()); err != nil {
		redact.Logf("session: keyring clear after reauth: %v", err)
	}
}

// Login validates the token with REST /users/@me, stores it, and connects.
func (m *Manager) Login(ctx context.Context, token string) (protocol.User, *protocol.Error) {
	n := ningen.New(token)
	me, err := n.WithContext(ctx).Me()
	if err != nil {
		return protocol.User{}, protocol.Errorf(protocol.CodeLoginFailed, "token rejected: %v", err)
	}
	if err := m.kr.Store(ctx, token); err != nil {
		// Still usable for this process; the user will have to log in again
		// after a restart.
		redact.Logf("session: keyring store failed: %v", err)
	}
	m.mu.Lock()
	old, done := m.teardownLocked()
	m.mu.Unlock()
	closeAndWait(old, done)

	m.mu.Lock()
	m.connectLocked(n, token)
	m.mu.Unlock()
	return wireUser(*me), nil
}

// Logout disconnects, drops the token, and clears the keyring.
func (m *Manager) Logout(ctx context.Context) *protocol.Error {
	m.mu.Lock()
	if m.n == nil {
		m.mu.Unlock()
		return protocol.Errorf(protocol.CodeNotLoggedIn, "not logged in")
	}
	n, done := m.teardownLocked()
	m.user, m.mentions, m.unreadDM = nil, 0, nil
	m.setLifecycleLocked(protocol.LifecycleLoggedOut, "")
	m.mu.Unlock()
	closeAndWait(n, done)
	if err := m.kr.Clear(ctx); err != nil {
		redact.Logf("session: keyring clear: %v", err)
	}
	return nil
}

// Snapshot implements socket.Backend.
func (m *Manager) Snapshot() []any {
	m.mu.Lock()
	defer m.mu.Unlock()
	evs := []any{protocol.NewStateChanged(m.stateLocked())}
	if m.lifecycle == protocol.LifecycleReady && m.n != nil {
		off := m.n.Offline()
		guilds, _ := Guilds(off)
		dms, _ := DMs(off)
		evs = append(evs, protocol.NewGuildsSynced(guilds, dms))
	}
	return evs
}

// Handle implements socket.Backend.
func (m *Manager) Handle(ctx context.Context, req *protocol.Request) (any, *protocol.Error) {
	switch req.Command {
	case "get_state":
		m.mu.Lock()
		defer m.mu.Unlock()
		return m.stateLocked(), nil
	case "login":
		var p protocol.LoginParams
		if e := req.Params(&p); e != nil {
			return nil, e
		}
		if p.Token == "" {
			return nil, protocol.Errorf(protocol.CodeInvalidArgument, "token is required")
		}
		u, e := m.Login(ctx, p.Token)
		if e != nil {
			return nil, e
		}
		return protocol.LoginResult{User: u}, nil
	case "logout":
		if e := m.Logout(ctx); e != nil {
			return nil, e
		}
		return protocol.EmptyResult{}, nil
	case "list_guilds":
		n, e := m.cachedSession()
		if e != nil {
			return nil, e
		}
		gs, err := Guilds(n)
		if err != nil {
			return nil, protocol.Errorf(protocol.CodeInternalError, "%v", err)
		}
		if gs == nil {
			gs = []protocol.Guild{}
		}
		return protocol.ListGuildsResult{Guilds: gs}, nil
	case "list_channels":
		var p protocol.ListChannelsParams
		if e := req.Params(&p); e != nil {
			return nil, e
		}
		sf, err := discord.ParseSnowflake(p.GuildID)
		if err != nil || !sf.IsValid() {
			return nil, protocol.Errorf(protocol.CodeInvalidArgument, "guild_id must be a snowflake string")
		}
		n, e := m.cachedSession()
		if e != nil {
			return nil, e
		}
		chs, err := Channels(n, discord.GuildID(sf))
		if errors.Is(err, ErrUnknownGuild) {
			return nil, protocol.Errorf(protocol.CodeUnknownGuild, "guild %s is not in this session", p.GuildID)
		}
		if err != nil {
			return nil, protocol.Errorf(protocol.CodeInternalError, "%v", err)
		}
		if chs == nil {
			chs = []protocol.Channel{}
		}
		return protocol.ListChannelsResult{Channels: chs}, nil
	case "list_dms":
		n, e := m.cachedSession()
		if e != nil {
			return nil, e
		}
		dms, err := DMs(n)
		if err != nil {
			return nil, protocol.Errorf(protocol.CodeInternalError, "%v", err)
		}
		if dms == nil {
			dms = []protocol.Channel{}
		}
		return protocol.ListChannelsResult{Channels: dms}, nil
	}
	return nil, protocol.Errorf(protocol.CodeUnknownCommand, "command %q is not implemented", req.Command)
}

// cachedSession returns a session whose cache has seen READY at least once.
func (m *Manager) cachedSession() (*ningen.State, *protocol.Error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch {
	case m.n == nil:
		return nil, protocol.Errorf(protocol.CodeNotLoggedIn, "not logged in")
	case !m.everReady:
		return nil, protocol.Errorf(protocol.CodeGatewayUnavailable, "gateway not ready yet")
	}
	return m.n, nil
}

// presence reports the own status: the live presence when known, else the
// status carried in READY's user settings (SESSIONS_REPLACE arrives later).
func presence(n *ningen.State) string {
	me, _ := n.Cabinet.Me()
	if me != nil {
		if p, _ := n.PresenceStore.Presence(0, me.ID); p != nil && p.Status != "" {
			return presenceString(p.Status)
		}
	}
	if us := n.Ready().UserSettings; us != nil {
		return presenceString(us.Status)
	}
	return ""
}

func presenceString(s discord.Status) string {
	switch s {
	case discord.OnlineStatus, discord.IdleStatus, discord.DoNotDisturbStatus, discord.InvisibleStatus:
		return string(s)
	case discord.OfflineStatus:
		return string(discord.InvisibleStatus)
	}
	return ""
}

func ptrEq(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
