package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	"github.com/mattcalayo/omarchy-discord/backend/internal/panics"
	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/redact"
	"github.com/mattcalayo/omarchy-discord/backend/internal/voice"
)

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

type Keyring interface {
	Lookup(ctx context.Context) (string, error)
	Store(ctx context.Context, token string) error
	Clear(ctx context.Context) error
}

type Manager struct {
	kr     Keyring
	events chan any

	opMu       sync.Mutex
	readAckMu  sync.Mutex
	forwarding atomic.Bool

	runLoop      func(ctx context.Context, n *ningen.State, done chan struct{})
	fetchTail    func(ctx context.Context, n *ningen.State, chID discord.ChannelID, limit uint) ([]discord.Message, error)
	fetchBefore  func(ctx context.Context, n *ningen.State, chID discord.ChannelID, before discord.MessageID, limit uint) ([]discord.Message, error)
	fetchChannel func(ctx context.Context, n *ningen.State, chID discord.ChannelID) (*discord.Channel, error)
	rest         restOps
	now          func() time.Time
	runQR        qrRunner
	newState     func(token string) *ningen.State
	qrWait       time.Duration

	typers         typingThrottle
	members        memberTracker
	memberDebounce time.Duration
	media          *media.Cache
	stagedDir      string
	qrPath         string

	newVoice func(n *ningen.State, ev voice.Events) voiceEngine

	mu         sync.Mutex
	lifecycle  string
	user       *protocol.User
	presence   string
	mentions   int
	unreadDM   *string
	generation int64
	errText    string
	voice      voiceEngine
	voiceState voice.State

	token     string
	n         *ningen.State
	everReady bool
	cancel    context.CancelFunc
	loopDone  chan struct{}
	qr        *qrFlow
}

func New(kr Keyring) *Manager {
	m := &Manager{kr: kr, events: make(chan any, 1024), lifecycle: protocol.LifecycleStarting, generation: 1, voiceState: idleVoice}
	m.runLoop = m.loop
	m.fetchTail, m.fetchBefore, m.fetchChannel = fetchTail, fetchBefore, fetchChannel
	m.memberDebounce = memberDebounce
	m.members.reset()
	m.rest, m.now = liveREST(), time.Now
	m.runQR, m.newState, m.qrWait = liveQR, defaultNewState, qrFirstCodeWait
	return m
}

func (m *Manager) Configure(runtimeDir string, cache *media.Cache) {
	m.stagedDir = filepath.Join(runtimeDir, "staged")
	m.qrPath = qrImagePath(runtimeDir)
	m.media = cache
}

func (m *Manager) EnableVoice(log *slog.Logger) {
	m.newVoice = func(n *ningen.State, ev voice.Events) voiceEngine { return voice.New(n, ev, log) }
}

func (m *Manager) Events() <-chan any { return m.events }

type flushToken struct{ done chan struct{} }

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

func (m *Manager) push(ev any) bool {
	select {
	case m.events <- ev:
		return true
	default:
		redact.Logf("session: event queue full, dropping %T", ev)
		return false
	}
}

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
		Voice:             wireVoice(m.voiceState),
	}
}

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

func (m *Manager) Stop() {
	if m.qrRunning() {
		m.CancelQRLogin(context.Background())
	}
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	n, done, v := m.teardownLocked()
	m.mu.Unlock()
	closeAndWait(n, done, v)
}

func (m *Manager) teardownLocked() (*ningen.State, chan struct{}, voiceEngine) {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	n, done, v := m.n, m.loopDone, m.voice
	m.n, m.loopDone, m.token, m.everReady = nil, nil, "", false
	m.voice, m.voiceState = nil, idleVoice
	return n, done, v
}

func closeAndWait(n *ningen.State, done chan struct{}, v voiceEngine) {
	if v != nil {
		v.Close()
	}
	if n != nil {
		if err := n.Close(); err != nil && !errors.Is(err, arikawasession.ErrClosed) {
			redact.Logf("session: close: %v", err)
		}
	}
	if done != nil {
		<-done
	}
}

func (m *Manager) connectLocked(n *ningen.State, token string) {
	m.n, m.token, m.everReady = n, token, false
	m.user, m.presence, m.mentions, m.unreadDM = nil, "", 0, nil
	m.voiceState = idleVoice
	if m.newVoice != nil {
		m.voice = m.newVoice(n, voice.Events{State: m.onVoiceState, Speaking: m.onVoiceSpeaking})
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.loopDone = make(chan struct{})
	m.installHandlers(n)
	m.lifecycle, m.errText = protocol.LifecycleConnecting, ""
	m.bump()
	loop, done := m.runLoop, m.loopDone
	panics.Go("session: connect loop", func() { loop(ctx, n, done) })
}

func (m *Manager) installHandlers(n *ningen.State) {
	addSyncHandler(n, "connected", func(ev *ningen.ConnectedEvent) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.n != n {
			return
		}
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
		m.pushVoiceMembersLocked(off)
	})
	addSyncHandler(n, "disconnected", func(ev *ningen.DisconnectedEvent) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.n != n {
			return
		}
		if ev.IsLoggedOut() {
			m.setLifecycleLocked(protocol.LifecycleReauthNeeded, fmt.Sprintf("gateway closed session (code %d)", ev.Code))
			return
		}
		if m.lifecycle == protocol.LifecycleReady {
			m.setLifecycleLocked(protocol.LifecycleConnecting, "")
		}
	})
	addSyncHandler(n, "read_update", func(ev *read.UpdateEvent) {
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
	m.installVoiceHandlers(n)
	resync := func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.n != n || m.lifecycle != protocol.LifecycleReady {
			return
		}
		m.pushStructureLocked(n.Offline())
	}
	addSyncHandler(n, "guild_create", func(*gateway.GuildCreateEvent) { resync() })
	addSyncHandler(n, "guild_delete", func(*gateway.GuildDeleteEvent) { resync() })
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
	m.generation++
	m.push(protocol.NewGuildsSynced(m.generation, guilds, dms))
}

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

func (m *Manager) reauth(n *ningen.State, cause error) {
	m.mu.Lock()
	if m.n != n {
		m.mu.Unlock()
		return
	}
	m.token = ""
	m.cancel = nil
	v := m.voice
	m.voice, m.voiceState = nil, idleVoice
	m.setLifecycleLocked(protocol.LifecycleReauthNeeded, fmt.Sprintf("session invalidated: %v", cause))
	m.mu.Unlock()
	if v != nil {
		v.Close()
	}
	if err := m.kr.Clear(context.Background()); err != nil {
		redact.Logf("session: keyring clear after reauth: %v", err)
	}
}

func (m *Manager) Login(ctx context.Context, token string) (protocol.LoginResult, *protocol.Error) {
	if m.qrRunning() {
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

func (m *Manager) finishLogin(ctx context.Context, n *ningen.State, token string, user protocol.User) protocol.LoginResult {
	stored := true
	if err := m.kr.Store(ctx, token); err != nil {
		stored = false
		redact.Logf("session: keyring store failed (login will not survive a restart): %v", err)
	}
	m.replaceSession(n, token)
	return protocol.LoginResult{User: user, KeyringStored: stored}
}

func (m *Manager) replaceSession(n *ningen.State, token string) {
	m.mu.Lock()
	old, done, v := m.teardownLocked()
	m.mu.Unlock()
	closeAndWait(old, done, v)

	m.mu.Lock()
	m.connectLocked(n, token)
	m.mu.Unlock()
}

func (m *Manager) Logout(ctx context.Context) *protocol.Error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	if m.n == nil {
		m.mu.Unlock()
		return protocol.Errorf(protocol.CodeNotLoggedIn, "not logged in")
	}
	n, done, v := m.teardownLocked()
	m.user, m.mentions, m.unreadDM = nil, 0, nil
	m.setLifecycleLocked(protocol.LifecycleLoggedOut, "")
	m.mu.Unlock()
	closeAndWait(n, done, v)
	if err := m.kr.Clear(ctx); err != nil {
		redact.Logf("session: keyring clear: %v", err)
	}
	return nil
}

func (m *Manager) Snapshot() []any {
	m.mu.Lock()
	defer m.mu.Unlock()
	evs := []any{protocol.NewStateChanged(m.stateLocked())}
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
		for _, ev := range allVoiceMembers(off) {
			if len(ev.Channels) > 0 {
				evs = append(evs, ev)
			}
		}
	}
	return evs
}

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
	case "guild_stats", "guild_settings", "set_guild_mute", "leave_guild", "mark_guild_read":
		return m.guildAction(ctx, req)
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
		return m.ack(ctx, req)
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
	case "voice_join":
		return m.voiceJoin(ctx, req)
	case "voice_leave":
		return m.voiceLeave(ctx)
	case "voice_set":
		return m.voiceSet(ctx, req)
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
