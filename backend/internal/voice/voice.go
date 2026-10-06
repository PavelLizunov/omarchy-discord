package voice

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
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

type Status string

const (
	StatusIdle       Status = "idle"
	StatusConnecting Status = "connecting"
	StatusConnected  Status = "connected"
	StatusError      Status = "error"
)

type State struct {
	Status    Status
	GuildID   discord.GuildID
	ChannelID discord.ChannelID
	Muted     bool
	Deafened  bool
	Error     string
}

type Events struct {
	State    func(State)
	Speaking func(userID discord.UserID, speaking bool)
}

const joinTimeout = 30 * time.Second

type Engine struct {
	n   *ningen.State
	ev  Events
	log *slog.Logger

	opMu sync.Mutex
	mu   sync.Mutex

	st     State
	selfID discord.UserID
	mgr    dvoice.Manager
	gen    uint64
	drops  int

	op4      dvoice.StateUpdateFunc
	newAudio func() (*audio, error)

	conn       dvoice.Conn
	audio      *audio
	sender     dvoice.AudioSender
	rxDriver   *rxDriver
	openCancel context.CancelFunc
}

func New(n *ningen.State, ev Events, log *slog.Logger) *Engine {
	if log == nil {
		log = slog.Default()
	}
	e := &Engine{n: n, ev: ev, log: log, st: State{Status: StatusIdle}}
	e.op4 = e.stateUpdate
	e.newAudio = func() (*audio, error) { return openAudio(e.log, e.speaking) }
	e.installHandlers()
	return e
}

func (e *Engine) State() State {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.st
}

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

func (e *Engine) Join(ctx context.Context, guildID discord.GuildID, channelID discord.ChannelID) error {
	if !guildID.IsValid() || !channelID.IsValid() {
		return errors.New("voice: guild and channel are required")
	}
	a, audioErr := e.newAudio()
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
		a.tx.setMuted(e.st.Muted)
		a.rx.deafened.Store(e.st.Deafened)
		e.gen++
		e.drops = 0
		gen = e.gen
		openCtx, e.openCancel = context.WithTimeout(ctx, joinTimeout)
		e.st = State{Status: StatusConnecting, GuildID: guildID, ChannelID: channelID, Muted: e.st.Muted, Deafened: e.st.Deafened}
		muted, deaf = e.st.Muted, e.st.Deafened
		return func() {
			if after != nil {
				after()
			}
			go a.watch(audioWatchInterval, func() { e.fail(gen, "audio server lost") })
			conn = e.mgr.CreateConn(snowflake.ID(guildID))
			e.mu.Lock()
			e.conn = conn
			e.mu.Unlock()
		}
	})
	if joinErr != nil {
		if a != nil {
			a.close()
		}
		return joinErr
	}

	err := conn.Open(openCtx, snowflake.ID(channelID), muted, deaf)

	var audio *audio
	e.update(func() func() {
		if gen != e.gen {
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
			conn.SetOpusFrameReceiver(audio.rx)
			conn.SetOpusFrameProvider(audio.tx)
		}
	})
	return err
}

func (e *Engine) Leave(ctx context.Context) error {
	e.update(func() func() {
		after := e.detachLocked()
		e.st = State{Status: StatusIdle, Muted: e.st.Muted, Deafened: e.st.Deafened}
		return after
	})
	return nil
}

func (e *Engine) SetMute(ctx context.Context, muted bool) error {
	return e.setFlags(ctx, &muted, nil)
}

func (e *Engine) SetDeaf(ctx context.Context, deafened bool) error {
	return e.setFlags(ctx, nil, &deafened)
}

func (e *Engine) setFlags(ctx context.Context, muted, deafened *bool) (err error) {
	e.update(func() func() {
		if muted != nil {
			e.st.Muted = *muted
		}
		if deafened != nil {
			e.st.Deafened = *deafened
		}
		if e.audio != nil {
			e.audio.tx.setMuted(e.st.Muted)
			e.audio.rx.deafened.Store(e.st.Deafened)
		}
		if e.conn == nil || (e.st.Status != StatusConnected && e.st.Status != StatusConnecting) {
			return nil
		}
		st := e.st
		return func() {
			cid := snowflake.ID(st.ChannelID)
			err = e.op4(ctx, snowflake.ID(st.GuildID), &cid, st.Muted, st.Deafened)
		}
	})
	return err
}

func (e *Engine) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = e.Leave(ctx)
}

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
		if sender != nil {
			sender.Close()
		}
		if rx != nil {
			rx.Close()
		}
		if conn != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			conn.Close(ctx)
			cancel()
		}
		if audio != nil {
			audio.close()
		}
	}
}

type sender struct {
	dvoice.AudioSender
	started <-chan struct{}
	once    sync.Once
}

type senderConn struct {
	dvoice.Conn
	started chan struct{}
	once    sync.Once
}

func (c *senderConn) DAVE() godave.Session {
	c.once.Do(func() { close(c.started) })
	return c.Conn.DAVE()
}

func newSender(l *slog.Logger, p dvoice.OpusFrameProvider, c dvoice.Conn) *sender {
	sc := &senderConn{Conn: c, started: make(chan struct{})}
	return &sender{AudioSender: dvoice.NewAudioSender(l, p, sc), started: sc.started}
}

func (s *sender) Close() {
	<-s.started
	s.once.Do(s.AudioSender.Close)
}

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

func (e *Engine) newManager(self snowflake.ID) dvoice.Manager {
	log := slog.New(infoHandler{e.log.Handler()})
	return dvoice.NewManager(e.op4, self,
		dvoice.WithLogger(log),
		dvoice.WithDaveSessionLogger(log),
		dvoice.WithDaveSessionCreateFunc(davesession.CreateFunc(davesession.WithLogger(log))),
		dvoice.WithConnConfigOpts(
			dvoice.WithConnGatewayCreateFunc(e.gatewayCreate),
			dvoice.WithConnAudioSenderCreateFunc(func(l *slog.Logger, p dvoice.OpusFrameProvider, c dvoice.Conn) dvoice.AudioSender {
				s := newSender(l, p, c)
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

func (e *Engine) gatewayCreate(ds godave.Session, evh dvoice.EventHandlerFunc, _ dvoice.CloseHandlerFunc, opts ...dvoice.GatewayConfigOpt) dvoice.Gateway {
	e.mu.Lock()
	gen := e.gen
	e.mu.Unlock()
	dialer := *websocket.DefaultDialer
	dialer.NetDialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		e.mu.Lock()
		live := gen == e.gen
		e.mu.Unlock()
		if !live {
			return nil, &websocket.CloseError{Code: dvoice.GatewayCloseEventCodeDisconnected.Code, Text: "call ended"}
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	opts = append(opts,
		dvoice.WithGatewayDialer(&dialer),
		dvoice.WithGatewayCloseObserver(func(err error) { e.observeClose(gen, err) }))
	return dvoice.NewGateway(ds, func(g dvoice.Gateway, op dvoice.Opcode, seq int, data dvoice.GatewayMessageData) {
		evh(g, op, seq, data)
		e.onEvent(gen, op, data)
	}, func(_ dvoice.Gateway, err error) {
		e.fail(gen, closeMessage(err))
	}, opts...)
}

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

func (e *Engine) onEvent(gen uint64, op dvoice.Opcode, data dvoice.GatewayMessageData) {
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
			if gen == e.gen && e.st.Status == StatusConnecting {
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

type infoHandler struct{ slog.Handler }

func (h infoHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return l >= slog.LevelInfo && h.Handler.Enabled(ctx, l)
}
func (h infoHandler) WithAttrs(as []slog.Attr) slog.Handler {
	return infoHandler{h.Handler.WithAttrs(as)}
}
func (h infoHandler) WithGroup(g string) slog.Handler { return infoHandler{h.Handler.WithGroup(g)} }
