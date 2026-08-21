// Package session owns the Discord session: ningen/arikawa lifecycle, the
// keyring-backed token, and the mapping from cache state to wire objects.
package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	arikawasession "github.com/diamondburned/arikawa/v3/session"
	"github.com/diamondburned/ningen/v3"
	"github.com/diamondburned/ningen/v3/states/read"

	"github.com/mattcalayo/omarchy-discord/backend/internal/keyring"
	"github.com/mattcalayo/omarchy-discord/backend/internal/media"
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

	// opMu serializes every auth operation (login, QR start/finish, logout,
	// stop): each spans several mu critical sections with blocking work
	// (REST validation, keyring store, gateway close) in between. mu only
	// guards the fields below and is never held across that work.
	opMu sync.Mutex
	// forwarding is set once Forward runs; Flush is a no-op before that.
	forwarding atomic.Bool

	// runLoop starts the connect loop for a freshly installed session. Tests
	// replace it to avoid the network.
	runLoop func(ctx context.Context, n *ningen.State, done chan struct{})
	// fetchTail / fetchBefore load messages for open_channel / history. Tests
	// replace them to avoid the network.
	fetchTail   func(ctx context.Context, n *ningen.State, chID discord.ChannelID, limit uint) ([]discord.Message, error)
	fetchBefore func(ctx context.Context, n *ningen.State, chID discord.ChannelID, before discord.MessageID, limit uint) ([]discord.Message, error)
	// fetchChannel resolves an uncached thread for open_channel (one REST
	// GET); tests replace it.
	fetchChannel func(ctx context.Context, n *ningen.State, chID discord.ChannelID) (*discord.Channel, error)
	// rest holds the write calls; now is the clock for throttles/progress.
	rest restOps
	now  func() time.Time
	// runQR / newState / qrWait are the QR login seams.
	runQR    qrRunner
	newState func(token string) *ningen.State
	qrWait   time.Duration

	typers typingThrottle
	// members tracks displayed users for presence routing; memberDebounce is
	// the member_list_update coalescing window (tests shorten it).
	members        memberTracker
	memberDebounce time.Duration
	// media is the media cache (nil until Configure; fetch_media then fails).
	media *media.Cache
	// stagedDir is where QML stages pasted uploads; files under it are
	// removed after a successful upload. qrPath is the rendered QR image.
	stagedDir string
	qrPath    string

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
	qr        *qrFlow
}

// New creates a manager in the `starting` lifecycle.
func New(kr Keyring) *Manager {
	m := &Manager{kr: kr, events: make(chan any, 1024), lifecycle: protocol.LifecycleStarting, generation: 1}
	m.runLoop = m.loop
	m.fetchTail, m.fetchBefore, m.fetchChannel = fetchTail, fetchBefore, fetchChannel
	m.memberDebounce = memberDebounce
	m.members.reset()
	m.rest, m.now = liveREST(), time.Now
	m.runQR, m.newState, m.qrWait = liveQR, defaultNewState, qrFirstCodeWait
	return m
}

// Configure sets the runtime directory (staged uploads, QR image) and the
// media cache. Call before Start.
func (m *Manager) Configure(runtimeDir string, cache *media.Cache) {
	m.stagedDir = filepath.Join(runtimeDir, "staged")
	m.qrPath = qrImagePath(runtimeDir)
	m.media = cache
}

// Events yields state_changed / guilds_synced events in the order they were
// produced. Production consumes it through Forward; tests read it directly.
func (m *Manager) Events() <-chan any { return m.events }

// flushToken is queued by Flush; Forward closes done when it reaches it,
// which proves every earlier event has been handed to the sink.
type flushToken struct{ done chan struct{} }

// Forward hands every event to sink, in order, until the manager's queue is
// closed (never, in practice). It is the production consumer of Events.
func (m *Manager) Forward(sink func(any)) {
	m.forwarding.Store(true)
	for ev := range m.events {
		if t, ok := ev.(flushToken); ok {
			close(t.done)
			continue
		}
		sink(ev)
	}
}

// Flush blocks until every event queued before the call has been passed to
// Forward's sink (or ctx ends). Commands whose documented contract puts their
// events before the response call it before returning. Without a running
// Forward there is nothing to order against and it returns at once.
func (m *Manager) Flush(ctx context.Context) {
	if !m.forwarding.Load() {
		return
	}
	t := flushToken{done: make(chan struct{})}
	if !m.push(t) {
		return
	}
	select {
	case <-t.done:
	case <-ctx.Done():
	}
}

// push queues an event; false when the queue is full and it was dropped.
func (m *Manager) push(ev any) bool {
	select {
	case m.events <- ev:
		return true
	default:
		redact.Logf("session: event queue full, dropping %T", ev)
		return false
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
	m.opMu.Lock()
	defer m.opMu.Unlock()
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
	if m.qrRunning() {
		m.CancelQRLogin(context.Background())
	}
	m.opMu.Lock()
	defer m.opMu.Unlock()
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
// Per-session state (user, presence, counts) is reset so a replaced session
// never shows the previous account's data; the state_changed is always sent
// even when the lifecycle was already `connecting`.
func (m *Manager) connectLocked(n *ningen.State, token string) {
	m.n, m.token, m.everReady = n, token, false
	m.user, m.presence, m.mentions, m.unreadDM = nil, "", 0, nil
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.loopDone = make(chan struct{})
	m.installHandlers(n)
	m.lifecycle, m.errText = protocol.LifecycleConnecting, ""
	m.bump()
	go m.runLoop(ctx, n, m.loopDone)
}

func (m *Manager) installHandlers(n *ningen.State) {
	// Sync handlers run inside ningen's dispatch after its sub-states updated,
	// so caches are consistent here. Keep them cheap and never block on the
	// network: structure is read through Offline().
	n.AddSyncHandler(func(ev *ningen.ConnectedEvent) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.n != n {
			return
		}
		// READY reset the cabinet; put our own member back before the first
		// permission-dependent read (unread dots below, every list later).
		if ready, ok := ev.Event.(*gateway.ReadyEvent); ok {
			seedSelfMembers(n, ready)
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
	n.AddSyncHandler(func(ev *read.UpdateEvent) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.n != n || m.lifecycle != protocol.LifecycleReady {
			return
		}
		off := n.Offline()
		mentions, dm := off.ReadState.TotalMentionCount(), UnreadDM(off)
		m.push(protocol.NewReadStateChanged(
			ev.ChannelID.String(), optSnowflake(discord.Snowflake(ev.GuildID)),
			ev.Unread, ev.MentionCount, optSnowflake(discord.Snowflake(ev.LastMessageID)), mentions))
		if mentions == m.mentions && ptrEq(dm, m.unreadDM) {
			return
		}
		m.mentions, m.unreadDM = mentions, dm
		m.bump()
	})
	m.installMessageHandlers(n)
	m.installChannelHandlers(n)
	m.installMemberHandlers(n)
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

// pushStructureLocked bumps the generation (structure changed) and queues a
// guilds_synced stamped with it. Caller holds mu.
func (m *Manager) pushStructureLocked(n *ningen.State) {
	guilds, err := Guilds(n)
	if err != nil {
		redact.Logf("session: guilds: %v", err)
	}
	dms, err := DMs(n)
	if err != nil {
		redact.Logf("session: dms: %v", err)
	}
	m.generation++
	m.push(protocol.NewGuildsSynced(m.generation, guilds, dms))
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
// The whole operation holds opMu so it cannot interleave with a QR flow's
// completion or another login: the QR guard is checked under the same lock
// the QR flow holds while installing its session.
func (m *Manager) Login(ctx context.Context, token string) (protocol.LoginResult, *protocol.Error) {
	if m.qrRunning() { // fast refusal; re-checked under opMu
		return protocol.LoginResult{}, protocol.Errorf(protocol.CodeQRUnavailable, "a QR login is in progress; cancel it first")
	}
	m.opMu.Lock()
	defer m.opMu.Unlock()
	if m.qrRunning() {
		return protocol.LoginResult{}, protocol.Errorf(protocol.CodeQRUnavailable, "a QR login is in progress; cancel it first")
	}
	n := ningen.New(token)
	me, err := n.WithContext(ctx).Me()
	if err != nil {
		return protocol.LoginResult{}, protocol.Errorf(protocol.CodeLoginFailed, "token rejected: %v", err)
	}
	return m.finishLogin(ctx, n, token, wireUser(*me)), nil
}

// finishLogin persists the validated token and installs the session. A keyring
// failure is not fatal — the in-memory session is valid for this process — but
// is reported in the result so the client can warn the user. Caller holds opMu.
func (m *Manager) finishLogin(ctx context.Context, n *ningen.State, token string, user protocol.User) protocol.LoginResult {
	stored := true
	if err := m.kr.Store(ctx, token); err != nil {
		stored = false
		redact.Logf("session: keyring store failed (login will not survive a restart): %v", err)
	}
	m.replaceSession(n, token)
	return protocol.LoginResult{User: user, KeyringStored: stored}
}

// replaceSession closes any live session and installs n. Caller holds opMu so
// two concurrent logins cannot each tear down and then both install, which
// would orphan a live gateway connection.
func (m *Manager) replaceSession(n *ningen.State, token string) {
	m.mu.Lock()
	old, done := m.teardownLocked()
	m.mu.Unlock()
	closeAndWait(old, done)

	m.mu.Lock()
	m.connectLocked(n, token)
	m.mu.Unlock()
}

// Logout disconnects, drops the token, and clears the keyring.
func (m *Manager) Logout(ctx context.Context) *protocol.Error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
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
	// A client (re)connecting mid-flow gets the live code again, otherwise
	// the QR UI would sit empty until the code expires.
	if m.lifecycle == protocol.LifecycleQRPending && m.qr != nil && m.qr.code != nil {
		code := *m.qr.code
		code.ExpiresInMS = max(0, m.qr.deadline.Sub(m.now()).Milliseconds())
		evs = append(evs, code)
	}
	if m.lifecycle == protocol.LifecycleReady && m.n != nil {
		off := m.n.Offline()
		guilds, _ := Guilds(off)
		dms, _ := DMs(off)
		evs = append(evs, protocol.NewGuildsSynced(m.generation, guilds, dms))
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
		res, e := m.Login(ctx, p.Token)
		if e != nil {
			return nil, e
		}
		return res, nil
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
	case "open_channel":
		return m.openChannel(ctx, req)
	case "close_channel":
		return m.closeChannel(ctx, req)
	case "history":
		return m.history(ctx, req)
	case "ack":
		return m.ack(req)
	case "send":
		return m.send(ctx, req)
	case "edit":
		return m.edit(ctx, req)
	case "delete":
		return m.deleteMessage(ctx, req)
	case "react":
		return m.react(ctx, req, true)
	case "unreact":
		return m.react(ctx, req, false)
	case "typing":
		return m.typing(ctx, req)
	case "set_presence":
		return m.setPresence(req)
	case "upload":
		return m.upload(ctx, req)
	case "fetch_media":
		return m.fetchMedia(ctx, req)
	case "set_config":
		return m.setConfig(req)
	case "quick_switch":
		return m.quickSwitch(req)
	case "list_threads":
		return m.listThreads(req)
	case "list_emoji":
		return m.listEmoji()
	case "subscribe_members":
		return m.subscribeMembers(ctx, req)
	case "unsubscribe_members":
		return m.unsubscribeMembers(ctx, req)
	case "start_qr_login":
		if e := m.StartQRLogin(ctx); e != nil {
			return nil, e
		}
		return protocol.EmptyResult{}, nil
	case "cancel_qr_login":
		if e := m.CancelQRLogin(ctx); e != nil {
			return nil, e
		}
		return protocol.EmptyResult{}, nil
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
