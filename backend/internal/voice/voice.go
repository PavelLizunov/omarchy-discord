//go:build cgo

// Package voice joins guild voice channels on the user session: disgo/voice
// for the voice gateway + UDP, dave-go for DAVE E2EE, hraban/opus (cgo) for
// the codec and jfreymuth/pulse for the microphone and speaker. One call at a
// time; the Engine owns the retry budget (see .claude/docs/voice-plan.md D3).
package voice

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/ningen/v3"
	dvoice "github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/godave"
	"github.com/disgoorg/snowflake/v2"
	"github.com/gorilla/websocket"
	davesession "github.com/thomas-vilte/dave-go/session"
)

// Status is the call status as shown on the wire.
type Status string

const (
	StatusIdle       Status = "idle"
	StatusConnecting Status = "connecting"
	StatusConnected  Status = "connected"
	StatusError      Status = "error"
)

// State is the engine's snapshot; Muted/Deafened persist across calls.
type State struct {
	Status    Status
	GuildID   discord.GuildID   // 0 when idle
	ChannelID discord.ChannelID // 0 when idle
	Muted     bool
	Deafened  bool
	Error     string // human-readable, "" unless Status == StatusError
}

// Events are the engine's callbacks. Both may be called from any goroutine;
// they must not call back into Join/Leave/SetMute/SetDeaf (State is fine).
type Events struct {
	State    func(State)                                // called on every change
	Speaking func(userID discord.UserID, speaking bool) // other participants only
}

const joinTimeout = 30 * time.Second

// Engine is one voice call at a time on top of a ningen user session.
type Engine struct {
	n   *ningen.State
	ev  Events
	log *slog.Logger

	// opMu serialises public calls and State emissions (outer lock); mu is
	// the state mutex (inner). Never take opMu while holding mu.
	opMu sync.Mutex
	mu   sync.Mutex

	st     State
	selfID discord.UserID
	mgr    dvoice.Manager
	gen    uint64 // bumps on every Join/teardown; async callbacks compare
	drops  int    // established voice sessions lost during this call

	// Per-call handles, detached under mu and closed outside it.
	conn       dvoice.Conn
	audio      *audio
	sender     dvoice.AudioSender
	rxDriver   *rxDriver
	openCancel context.CancelFunc
}

// New registers the two voice sync handlers on n. The op 4 commands go through
// n.Gateway().Send. log nil → slog.Default().
func New(n *ningen.State, ev Events, log *slog.Logger) *Engine {
	if log == nil {
		log = slog.Default()
	}
	e := &Engine{n: n, ev: ev, log: log, st: State{Status: StatusIdle}}
	e.installHandlers()
	return e
}

// State returns a snapshot.
func (e *Engine) State() State {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.st
}

// update runs fn under both locks, closes whatever fn detached (outside mu)
// and emits the new state if it changed.
func (e *Engine) update(fn func() (after func())) {
	e.opMu.Lock()
	defer e.opMu.Unlock()
	e.mu.Lock()
	before := e.st
	after := fn()
	st := e.st
	e.mu.Unlock()
	if after != nil {
		after()
	}
	if st != before && e.ev.State != nil {
		e.ev.State(st)
	}
}

// Join leaves any current call, then joins channelID. Audio devices are
// opened before any Discord traffic, so a broken Pulse setup never joins.
func (e *Engine) Join(ctx context.Context, guildID discord.GuildID, channelID discord.ChannelID) error {
	if !guildID.IsValid() || !channelID.IsValid() {
		return errors.New("voice: guild and channel are required")
	}
	// Pulse I/O happens outside the locks; a failure still leaves the
	// previous call (Join always leaves first).
	a, audioErr := openAudio(e.log, e.speaking)
	var (
		conn    dvoice.Conn
		openCtx context.Context
		gen     uint64
		joinErr error
		muted   bool
		deaf    bool
	)
	e.update(func() func() {
		after := e.detachLocked()
		me, err := e.n.Cabinet.Me()
		if err != nil {
			joinErr = fmt.Errorf("voice: session not ready: %w", err)
			e.st = State{Status: StatusIdle, Muted: e.st.Muted, Deafened: e.st.Deafened}
			return after
		}
		if audioErr != nil {
			joinErr = fmt.Errorf("voice: audio: %w", audioErr)
			e.st = State{Status: StatusError, GuildID: guildID, ChannelID: channelID, Muted: e.st.Muted, Deafened: e.st.Deafened, Error: joinErr.Error()}
			return after
		}
		e.selfID = me.ID
		if e.mgr == nil {
			e.mgr = e.newManager(snowflake.ID(me.ID))
		}
		e.audio = a
		a.tx.muted.Store(e.st.Muted)
		a.rx.deafened.Store(e.st.Deafened)
		e.gen++
		e.drops = 0
		gen = e.gen
		conn = e.mgr.CreateConn(snowflake.ID(guildID))
		e.conn = conn
		openCtx, e.openCancel = context.WithTimeout(ctx, joinTimeout)
		e.st = State{Status: StatusConnecting, GuildID: guildID, ChannelID: channelID, Muted: e.st.Muted, Deafened: e.st.Deafened}
		muted, deaf = e.st.Muted, e.st.Deafened
		return after
	})
	if joinErr != nil {
		if a != nil {
			a.close() // opened but never attached
		}
		return joinErr
	}

	// Blocks until the voice session description arrives (opMu released so
	// Leave / failures can cancel us).
	err := conn.Open(openCtx, snowflake.ID(channelID), muted, deaf)

	var audio *audio
	e.update(func() func() {
		if gen != e.gen {
			// Superseded by Leave/Join/failure while opening.
			if e.st.Status == StatusError {
				err = errors.New(e.st.Error)
			} else {
				err = nil
			}
			return nil
		}
		if e.openCancel != nil {
			e.openCancel()
			e.openCancel = nil
		}
		if err != nil {
			err = fmt.Errorf("voice: join failed: %w", err)
			after := e.detachLocked()
			e.st.Status, e.st.Error = StatusError, err.Error()
			return after
		}
		audio = e.audio
		e.st.Status = StatusConnected
		return func() {
			// Create funcs store the sender/receiver handles under mu, so
			// wire the audio outside it.
			conn.SetOpusFrameReceiver(audio.rx)
			conn.SetOpusFrameProvider(audio.tx)
		}
	})
	return err
}

// Leave ends the call (or clears an error) and returns to idle.
func (e *Engine) Leave(ctx context.Context) error {
	e.update(func() func() {
		after := e.detachLocked()
		e.st = State{Status: StatusIdle, Muted: e.st.Muted, Deafened: e.st.Deafened}
		return after
	})
	return nil
}

// SetMute stops the provider and sends self_mute; the flag persists.
func (e *Engine) SetMute(ctx context.Context, muted bool) error {
	return e.setFlags(ctx, &muted, nil)
}

// SetDeaf stops the mixer and sends self_deaf; the flag persists.
func (e *Engine) SetDeaf(ctx context.Context, deafened bool) error {
	return e.setFlags(ctx, nil, &deafened)
}

func (e *Engine) setFlags(ctx context.Context, muted, deafened *bool) error {
	var (
		send bool
		st   State
	)
	e.update(func() func() {
		if muted != nil {
			e.st.Muted = *muted
		}
		if deafened != nil {
			e.st.Deafened = *deafened
		}
		if e.audio != nil {
			e.audio.tx.muted.Store(e.st.Muted)
			e.audio.rx.deafened.Store(e.st.Deafened)
		}
		send = e.conn != nil && (e.st.Status == StatusConnected || e.st.Status == StatusConnecting)
		st = e.st
		return nil
	})
	if !send {
		return nil
	}
	cid := snowflake.ID(st.ChannelID)
	return e.stateUpdate(ctx, snowflake.ID(st.GuildID), &cid, st.Muted, st.Deafened)
}

// Close leaves if needed and releases the audio streams.
func (e *Engine) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = e.Leave(ctx)
}

// detachLocked takes the per-call handles out of the engine and returns the
// closer to run outside mu. Bumping gen makes every in-flight callback of the
// old call a no-op.
func (e *Engine) detachLocked() func() {
	e.gen++
	conn, audio, sender, rx, cancel := e.conn, e.audio, e.sender, e.rxDriver, e.openCancel
	e.conn, e.audio, e.sender, e.rxDriver, e.openCancel = nil, nil, nil, nil, nil
	if cancel != nil {
		cancel()
	}
	if conn == nil && audio == nil {
		return nil
	}
	return func() {
		// Stop the pumps before the conn: disgo only stops them on the
		// self voice-state echo, which the manager no longer routes once
		// the conn is removed.
		if sender != nil {
			closeQuietly(sender)
		}
		if rx != nil {
			rx.Close()
		}
		if conn != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			conn.Close(ctx) // op 4 leave, gateway/udp/dave close, RemoveConn
			cancel()
		}
		if audio != nil {
			audio.close() // clears speaking for every tracked user
		}
	}
}

// closeQuietly closes disgo's audio sender, whose Close nil-derefs when it
// runs before the sender goroutine has stored its cancel func.
func closeQuietly(c interface{ Close() }) {
	defer func() { _ = recover() }()
	c.Close()
}

// fail ends call gen with an error. No-op when gen is stale.
func (e *Engine) fail(gen uint64, msg string) {
	e.update(func() func() {
		if gen != e.gen || e.st.Status == StatusIdle {
			return nil
		}
		e.log.Warn("voice: call failed", "err", msg)
		after := e.detachLocked()
		e.st.Status, e.st.Error = StatusError, msg
		return after
	})
}

func (e *Engine) speaking(id snowflake.ID, on bool) {
	if e.ev.Speaking != nil {
		e.ev.Speaking(discord.UserID(id), on)
	}
}

// newManager wires disgo exactly as the spike did: dave-go sessions, our
// close handling instead of disgo's fresh re-join, a non-spinning receiver
// and a tracked sender.
func (e *Engine) newManager(self snowflake.ID) dvoice.Manager {
	log := slog.New(infoHandler{e.log.Handler()}) // disgo's debug lines carry the token/secret key
	return dvoice.NewManager(e.stateUpdate, self,
		dvoice.WithLogger(log),
		dvoice.WithDaveSessionLogger(log),
		dvoice.WithDaveSessionCreateFunc(davesession.CreateFunc(davesession.WithLogger(log))),
		dvoice.WithConnConfigOpts(
			dvoice.WithConnGatewayCreateFunc(e.gatewayCreate),
			dvoice.WithConnEventHandlerFunc(e.onEvent),
			dvoice.WithConnAudioSenderCreateFunc(func(l *slog.Logger, p dvoice.OpusFrameProvider, c dvoice.Conn) dvoice.AudioSender {
				s := dvoice.NewAudioSender(l, p, c)
				e.mu.Lock()
				e.sender = s
				e.mu.Unlock()
				return s
			}),
			dvoice.WithConnAudioReceiverCreateFunc(func(l *slog.Logger, r dvoice.OpusFrameReceiver, c dvoice.Conn) dvoice.AudioReceiver {
				d := &rxDriver{log: l, rx: r, conn: c}
				e.mu.Lock()
				e.rxDriver = d
				e.mu.Unlock()
				return d
			}),
		),
	)
}

// gatewayCreate is called synchronously from CreateConn (inside Join, mu
// held), so e.gen is the new call's generation. Our close handler never calls
// disgo's: that one re-joins with a fresh op 4, which is our user's decision.
func (e *Engine) gatewayCreate(ds godave.Session, evh dvoice.EventHandlerFunc, _ dvoice.CloseHandlerFunc, opts ...dvoice.GatewayConfigOpt) dvoice.Gateway {
	gen := e.gen
	opts = append(opts, dvoice.WithGatewayCloseObserver(func(err error) { e.observeClose(gen, err) }))
	return dvoice.NewGateway(ds, evh, func(_ dvoice.Gateway, err error) {
		// disgo gave up: a non-resumable close code or five failed resumes.
		e.fail(gen, closeMessage(err))
	}, opts...)
}

// observeClose sees every drop of an established voice session, before disgo
// starts its own resume. Budget: one resume per call.
func (e *Engine) observeClose(gen uint64, err error) {
	resumable := true
	if code := closeCode(err); code != 0 {
		resumable = dvoice.GatewayCloseEventCodeByCode(code).Reconnect
	}
	e.update(func() func() {
		if gen != e.gen || e.st.Status == StatusIdle {
			return nil
		}
		e.drops++
		if !resumable || e.drops > 1 {
			e.log.Warn("voice: connection lost", "err", closeMessage(err), "drops", e.drops)
			after := e.detachLocked()
			e.st.Status, e.st.Error = StatusError, closeMessage(err)
			return after
		}
		e.log.Warn("voice: connection dropped, resuming once", "err", closeMessage(err))
		e.st.Status = StatusConnecting
		return nil
	})
}

func (e *Engine) onEvent(_ dvoice.Gateway, op dvoice.Opcode, _ int, data dvoice.GatewayMessageData) {
	switch d := data.(type) {
	case dvoice.GatewayMessageDataReady:
		e.log.Info("voice: ready", "ssrc", d.SSRC, "modes", d.Modes)
	case dvoice.GatewayMessageDataSessionDescription:
		e.log.Info("voice: session description", "mode", d.Mode, "dave_protocol_version", d.DaveProtocolVersion)
	case dvoice.GatewayMessageDataClientsConnect:
		e.log.Info("voice: clients connect", "users", d.UserIDs)
	case dvoice.GatewayMessageDataClientDisconnect:
		e.log.Info("voice: client disconnect", "user", d.UserID)
	case dvoice.GatewayMessageDataResumed:
		e.log.Info("voice: resumed")
		e.update(func() func() {
			if e.conn != nil && e.st.Status == StatusConnecting {
				e.st.Status = StatusConnected
			}
			return nil
		})
	default:
		e.log.Debug("voice: gateway event", "op", int(op))
	}
}

func closeCode(err error) int {
	var ce *websocket.CloseError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return 0
}

func closeMessage(err error) string {
	if code := closeCode(err); code != 0 {
		cc := dvoice.GatewayCloseEventCodeByCode(code)
		if cc.Description != "" {
			return fmt.Sprintf("voice gateway closed (%d): %s", code, cc.Description)
		}
		return fmt.Sprintf("voice gateway closed (%d)", code)
	}
	if err == nil {
		return "voice connection lost"
	}
	return "voice connection lost: " + err.Error()
}

// infoHandler hides Debug records: disgo dumps raw voice-gateway payloads
// (token, secret_key) at that level.
type infoHandler struct{ slog.Handler }

func (h infoHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return l >= slog.LevelInfo && h.Handler.Enabled(ctx, l)
}
func (h infoHandler) WithAttrs(as []slog.Attr) slog.Handler {
	return infoHandler{h.Handler.WithAttrs(as)}
}
func (h infoHandler) WithGroup(g string) slog.Handler { return infoHandler{h.Handler.WithGroup(g)} }
