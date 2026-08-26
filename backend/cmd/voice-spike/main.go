//go:build cgo

// Command voice-spike is the throwaway Phase 0 experiment from
// .claude/docs/voice-plan.md: prove that a user-account ningen session can
// join a guild voice channel through disgo/voice + dave-go (DAVE E2EE) and
// move Opus frames both ways. Deleted at the end of Phase 1.
//
// Assumptions (recorded instead of asked):
//   - The token is the daemon's secret-tool entry (keyring.Keyring{}.Lookup).
//   - "Owned" guild = Guild.OwnerID == our user ID; only type-2 voice channels
//     are listed (stage channels are out of scope).
//   - disgo is pinned to master (v0.19.7-0.20260825183231-aa92366d296d): its
//     AudioSender gates on DAVE Ready() and its resume loop is capped at 5.
//     Our close handler does NOT call disgo's (which would re-join with a
//     fresh op 4 on 4006/4015): any close code logs and ends the run.
//   - Received frames are Opus-decoded only to count decodable frames.
//   - dave-go's per-frame "frame encrypted" debug line is dropped; the token
//     and secret_key in disgo's raw-payload debug lines are scrubbed.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"regexp"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/ningen/v3"
	ddiscord "github.com/disgoorg/disgo/discord"
	dgateway "github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/godave"
	"github.com/disgoorg/snowflake/v2"
	"github.com/gorilla/websocket"
	"github.com/hraban/opus"
	davesession "github.com/thomas-vilte/dave-go/session"

	"github.com/mattcalayo/omarchy-discord/backend/internal/keyring"
	"github.com/mattcalayo/omarchy-discord/backend/internal/session"
)

var opNames = map[voice.Opcode]string{
	voice.OpcodeIdentify: "Identify", voice.OpcodeSelectProtocol: "SelectProtocol", voice.OpcodeReady: "Ready",
	voice.OpcodeHeartbeat: "Heartbeat", voice.OpcodeSessionDescription: "SessionDescription", voice.OpcodeSpeaking: "Speaking",
	voice.OpcodeHeartbeatACK: "HeartbeatACK", voice.OpcodeResume: "Resume", voice.OpcodeHello: "Hello", voice.OpcodeResumed: "Resumed",
	voice.OpcodeClientsConnect: "ClientsConnect", voice.OpcodeClientDisconnect: "ClientDisconnect", voice.OpcodeGuildSync: "GuildSync",
	voice.OpcodeDavePrepareTransition: "DavePrepareTransition", voice.OpcodeDaveExecuteTransition: "DaveExecuteTransition",
	voice.OpcodeDaveTransitionReady: "DaveTransitionReady", voice.OpcodeDavePrepareEpoch: "DavePrepareEpoch",
	voice.OpcodeDaveMLSExternalSenderPackage: "DaveMLSExternalSenderPackage", voice.OpcodeDaveMLSKeyPackage: "DaveMLSKeyPackage",
	voice.OpcodeDaveMLSProposals: "DaveMLSProposals", voice.OpcodeDaveMLSCommitWelcome: "DaveMLSCommitWelcome",
	voice.OpcodeDaveMLSPrepareCommitTransition: "DaveMLSPrepareCommitTransition", voice.OpcodeDaveMLSWelcome: "DaveMLSWelcome",
	voice.OpcodeDaveMLSInvalidCommitWelcome: "DaveMLSInvalidCommitWelcome",
}

func opName(op voice.Opcode) string {
	if n, ok := opNames[op]; ok {
		return fmt.Sprintf("%d/%s", op, n)
	}
	return fmt.Sprintf("%d/?", op)
}

// scrubHandler drops dave-go's per-frame debug line and masks secrets in
// disgo's raw payload dumps.
type scrubHandler struct{ slog.Handler }

var secretRe = regexp.MustCompile(`("(?:token|secret_key)":)(?:"[^"]*"|\[[^\]]*\])`)

func (h scrubHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Message == "frame encrypted" {
		return nil
	}
	nr := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "data" && a.Value.Kind() == slog.KindString {
			a.Value = slog.StringValue(secretRe.ReplaceAllString(a.Value.String(), `$1"<scrubbed>"`))
		}
		nr.AddAttrs(a)
		return true
	})
	return h.Handler.Handle(ctx, nr)
}

func (h scrubHandler) WithAttrs(as []slog.Attr) slog.Handler {
	return scrubHandler{h.Handler.WithAttrs(as)}
}
func (h scrubHandler) WithGroup(g string) slog.Handler { return scrubHandler{h.Handler.WithGroup(g)} }

// receiver counts packets per user and how many of them libopus can decode.
type receiver struct {
	mu      sync.Mutex
	counts  map[snowflake.ID]int
	decoded map[snowflake.ID]int
	errs    int
	dec     map[snowflake.ID]*opus.Decoder
	buf     []int16
}

func (r *receiver) ReceiveOpusFrame(userID snowflake.ID, p *voice.Packet) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counts[userID]++
	if len(p.Opus) == 0 {
		return nil
	}
	dec := r.dec[userID]
	if dec == nil {
		var err error
		if dec, err = opus.NewDecoder(48000, 2); err != nil {
			return err
		}
		r.dec[userID] = dec
	}
	if _, err := dec.Decode(p.Opus, r.buf); err != nil {
		r.errs++
		return nil // keep counting; one bad frame is not a run failure
	}
	r.decoded[userID]++
	return nil
}

func (r *receiver) CleanupUser(snowflake.ID) {}
func (r *receiver) Close()                   {}

func (r *receiver) snapshot() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make([]snowflake.ID, 0, len(r.counts))
	for id := range r.counts {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	s := ""
	for _, id := range ids {
		s += fmt.Sprintf(" user=%d packets=%d decoded=%d", id, r.counts[id], r.decoded[id])
	}
	if s == "" {
		s = " (none)"
	}
	if r.errs > 0 {
		s += fmt.Sprintf(" decode_errors=%d", r.errs)
	}
	return s
}

// toneProvider encodes a 440 Hz sine. disgo's AudioSender only pulls frames
// once DAVE is Ready, and sends the silence frames + Speaking itself.
type toneProvider struct {
	enc   *opus.Encoder
	phase float64
	pcm   []int16
	out   []byte
	sent  int
}

func (t *toneProvider) ProvideOpusFrame() ([]byte, error) {
	for i := 0; i < 960; i++ {
		v := int16(0.3 * math.MaxInt16 * math.Sin(t.phase))
		t.phase += 2 * math.Pi * 440 / 48000
		t.pcm[2*i], t.pcm[2*i+1] = v, v
	}
	n, err := t.enc.Encode(t.pcm, t.out)
	if err != nil {
		return nil, err
	}
	t.sent++
	return t.out[:n], nil
}
func (t *toneProvider) Close() {}

// silentProvider never yields audio (--tone off).
type silentProvider struct{}

func (silentProvider) ProvideOpusFrame() ([]byte, error) { return nil, nil }
func (silentProvider) Close()                            {}

func main() {
	guildFlag := flag.Uint64("guild", 0, "guild ID to join")
	channelFlag := flag.Uint64("channel", 0, "voice channel ID to join")
	duration := flag.Duration("duration", 20*time.Second, "how long to stay connected")
	list := flag.Bool("list", false, "list owned guilds with voice channels and occupants, then exit")
	tone := flag.Bool("tone", false, "send a 440 Hz tone (default: silence frames only)")
	flag.Parse()

	slog.SetDefault(slog.New(scrubHandler{slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})}))
	os.Exit(run(*guildFlag, *channelFlag, *duration, *list, *tone))
}

func run(guildID, channelID uint64, duration time.Duration, list, tone bool) int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if !list && (guildID == 0 || channelID == 0) {
		slog.Error("need --guild and --channel (or --list)")
		return 2
	}

	session.ConfigureIdentity()
	tok, err := keyring.Keyring{}.Lookup(ctx)
	if err != nil {
		slog.Error("keyring lookup failed", "err", err)
		return 1
	}
	n := ningen.New(tok)

	// Forward arikawa's two voice events into disgo. Sync handlers keep the
	// order Discord sent them in (state update first, then server update).
	var mgr voice.Manager
	n.AddSyncHandler(func(ev *gateway.VoiceStateUpdateEvent) {
		if mgr == nil {
			return
		}
		u := dgateway.EventVoiceStateUpdate{VoiceState: ddiscord.VoiceState{
			GuildID: snowflake.ID(ev.GuildID), UserID: snowflake.ID(ev.UserID), SessionID: ev.SessionID,
			GuildDeaf: ev.Deaf, GuildMute: ev.Mute, SelfDeaf: ev.SelfDeaf, SelfMute: ev.SelfMute,
			SelfStream: ev.SelfStream, SelfVideo: ev.SelfVideo, Suppress: ev.Suppress,
		}}
		if ev.ChannelID.IsValid() {
			id := snowflake.ID(ev.ChannelID)
			u.ChannelID = &id
		}
		slog.Info("main gw: VOICE_STATE_UPDATE", "guild", ev.GuildID, "user", ev.UserID, "channel", ev.ChannelID, "session_id_len", len(ev.SessionID))
		mgr.HandleVoiceStateUpdate(u)
	})
	n.AddSyncHandler(func(ev *gateway.VoiceServerUpdateEvent) {
		if mgr == nil {
			return
		}
		u := dgateway.EventVoiceServerUpdate{Token: ev.Token, GuildID: snowflake.ID(ev.GuildID)}
		if ev.Endpoint != "" {
			e := ev.Endpoint
			u.Endpoint = &e
		}
		slog.Info("main gw: VOICE_SERVER_UPDATE", "guild", ev.GuildID, "endpoint", ev.Endpoint, "voice_gateway_url", fmt.Sprintf("wss://%s?v=%d", ev.Endpoint, voice.GatewayVersion))
		mgr.HandleVoiceServerUpdate(u)
	})

	slog.Info("opening main gateway")
	if err := n.Open(ctx); err != nil {
		slog.Error("main gateway open failed", "err", err)
		return 1
	}
	defer n.Close()
	me, err := n.Cabinet.Me()
	if err != nil {
		slog.Error("no self user", "err", err)
		return 1
	}
	slog.Info("main gateway ready", "user", me.ID, "username", me.Username)

	if list {
		return listOwned(n, me.ID)
	}

	// Refuse joins outside owned guilds and report occupancy before joining.
	g, err := n.Cabinet.Guild(discord.GuildID(guildID))
	if err != nil {
		slog.Error("guild not in cache", "guild", guildID, "err", err)
		return 2
	}
	if g.OwnerID != me.ID {
		slog.Error("refusing: guild not owned by this account", "guild", guildID, "owner", g.OwnerID)
		return 2
	}
	slog.Info("target", "guild", g.Name, "channel", channelID, "occupants_before_join", occupants(n, g.ID)[discord.ChannelID(channelID)])

	// DAVE session handle + close signalling.
	var daveMu sync.Mutex
	var dave *davesession.Session
	getDave := func() *davesession.Session { daveMu.Lock(); defer daveMu.Unlock(); return dave }
	closed := make(chan struct{})
	var closeOnce sync.Once
	var closeCode int

	stateUpdate := func(ctx context.Context, gid snowflake.ID, cid *snowflake.ID, mute, deaf bool) error {
		cmd := &gateway.UpdateVoiceStateCommand{GuildID: discord.GuildID(gid), SelfMute: mute, SelfDeaf: deaf}
		if cid != nil {
			cmd.ChannelID = discord.ChannelID(*cid)
		}
		slog.Info("main gw: sending op 4", "guild", cmd.GuildID, "channel", cmd.ChannelID, "self_mute", mute, "self_deaf", deaf)
		return n.Gateway().Send(ctx, cmd)
	}

	onEvent := func(_ voice.Gateway, op voice.Opcode, seq int, data voice.GatewayMessageData) {
		attrs := []any{"op", opName(op), "seq", seq}
		switch d := data.(type) {
		case voice.GatewayMessageDataReady:
			attrs = append(attrs, "ssrc", d.SSRC, "ip", d.IP, "port", d.Port, "modes", d.Modes)
			if m, err := voice.ChooseEncryptionMode(d.Modes); err == nil {
				attrs = append(attrs, "chosen_mode", m)
			}
		case voice.GatewayMessageDataSessionDescription:
			attrs = append(attrs, "mode", d.Mode, "dave_protocol_version", d.DaveProtocolVersion, "key_len", len(d.SecretKey))
		case voice.GatewayMessageDataSpeaking:
			attrs = append(attrs, "user", d.UserID, "ssrc", d.SSRC, "flags", d.Speaking)
		case voice.GatewayMessageDataClientsConnect:
			attrs = append(attrs, "users", d.UserIDs)
		case voice.GatewayMessageDataClientDisconnect:
			attrs = append(attrs, "user", d.UserID)
		case voice.GatewayMessageDataDaveProtocolPrepareTransition:
			attrs = append(attrs, "transition_id", d.TransitionID, "protocol_version", d.ProtocolVersion)
		case voice.GatewayMessageDataDaveProtocolExecuteTransition:
			attrs = append(attrs, "transition_id", d.TransitionID)
		case voice.GatewayMessageDataDaveProtocolPrepareEpoch:
			attrs = append(attrs, "epoch", d.Epoch, "protocol_version", d.ProtocolVersion)
		case voice.GatewayMessageDataDaveMLSExternalSenderPackage:
			attrs = append(attrs, "bytes", len(d))
		case voice.GatewayMessageDataDaveMLSProposals:
			attrs = append(attrs, "bytes", len(d))
		case voice.GatewayMessageDataDaveMLSAnnounceCommitTransition:
			attrs = append(attrs, "transition_id", d.TransitionID, "commit_bytes", len(d.CommitMessage))
		case voice.GatewayMessageDataDaveMLSWelcome:
			attrs = append(attrs, "transition_id", d.TransitionID, "welcome_bytes", len(d.WelcomeMessage))
		}
		if s := getDave(); s != nil {
			st := s.State()
			attrs = append(attrs, "dave_ready", st.Ready, "dave_epoch", st.EpochID, "dave_pv", st.ProtocolVersion)
		}
		slog.Info("voice gw event", attrs...)
	}

	// Log the close code and end the run. Deliberately does not call disgo's
	// own handler: on 4006/4015 it would re-join with a fresh op 4.
	gatewayCreate := func(ds godave.Session, evh voice.EventHandlerFunc, _ voice.CloseHandlerFunc, opts ...voice.GatewayConfigOpt) voice.Gateway {
		return voice.NewGateway(ds, evh, func(_ voice.Gateway, err error) {
			var ce *websocket.CloseError
			if errors.As(err, &ce) {
				cc := voice.GatewayCloseEventCodeByCode(ce.Code)
				closeCode = ce.Code
				slog.Error("VOICE GATEWAY CLOSED", "code", ce.Code, "text", ce.Text, "meaning", cc.Description, "explanation", cc.Explanation)
			} else {
				slog.Error("voice gateway closed", "err", err)
			}
			closeOnce.Do(func() { close(closed) })
		}, opts...)
	}

	mgr = voice.NewManager(stateUpdate, snowflake.ID(me.ID),
		voice.WithLogger(slog.Default()),
		voice.WithDaveSessionLogger(slog.Default()),
		voice.WithDaveSessionCreateFunc(davesession.CreateFunc(
			davesession.WithLogger(slog.Default()),
			davesession.WithSessionHook(func(s *davesession.Session) {
				daveMu.Lock()
				dave = s
				daveMu.Unlock()
				slog.Info("dave session created", "max_protocol_version", s.MaxSupportedProtocolVersion())
			}),
		)),
		voice.WithConnConfigOpts(
			voice.WithConnGatewayCreateFunc(gatewayCreate),
			voice.WithConnEventHandlerFunc(onEvent),
		),
	)
	defer mgr.Close(context.Background())

	conn := mgr.CreateConn(snowflake.ID(guildID))
	openCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	err = conn.Open(openCtx, snowflake.ID(channelID), false, false)
	cancel()
	if err != nil {
		slog.Error("voice conn open failed", "err", err)
		return 1
	}
	slog.Info("voice conn open (session description received)")

	rcv := &receiver{counts: map[snowflake.ID]int{}, decoded: map[snowflake.ID]int{}, dec: map[snowflake.ID]*opus.Decoder{}, buf: make([]int16, 5760*2)}
	conn.SetOpusFrameReceiver(rcv)
	var tp *toneProvider
	if tone {
		enc, err := opus.NewEncoder(48000, 2, opus.AppVoIP)
		if err != nil {
			slog.Error("opus encoder", "err", err)
			return 1
		}
		_ = enc.SetBitrate(64000)
		tp = &toneProvider{enc: enc, pcm: make([]int16, 1920), out: make([]byte, 1400)}
		conn.SetOpusFrameProvider(tp)
	} else {
		conn.SetOpusFrameProvider(silentProvider{})
	}

	start := time.Now()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	deadline := time.After(duration)
loop:
	for {
		select {
		case <-tick.C:
			attrs := []any{"elapsed", time.Since(start).Round(time.Second), "rx", rcv.snapshot(), "gw_status", conn.Gateway().Status()}
			if s := getDave(); s != nil {
				st, stats := s.State(), s.Stats()
				attrs = append(attrs, "dave_ready", s.Ready(), "dave_state", fmt.Sprintf("%+v", st),
					"passthrough", stats.PassthroughFrames, "decrypt_failures", stats.DecryptFailures, "encrypt_failures", stats.EncryptFailures)
				if st.Ready {
					code, err := s.EpochAuthenticatorCode(ctx)
					attrs = append(attrs, "epoch_authenticator_code", code, "code_err", err)
				}
			}
			if tp != nil {
				attrs = append(attrs, "tone_frames_sent", tp.sent)
			}
			slog.Info("tick", attrs...)
		case <-deadline:
			break loop
		case <-closed:
			break loop
		case <-ctx.Done():
			break loop
		}
	}

	select {
	case <-closed:
	default:
		slog.Info("leaving")
		conn.Close(context.Background())
	}
	summary := []any{"elapsed", time.Since(start).Round(time.Millisecond), "close_code", closeCode, "rx", rcv.snapshot()}
	if s := getDave(); s != nil {
		summary = append(summary, "dave_ready", s.Ready(), "dave_state", fmt.Sprintf("%+v", s.State()), "dave_stats", fmt.Sprintf("%+v", s.Stats()))
	}
	if tp != nil {
		summary = append(summary, "tone_frames_sent", tp.sent)
	}
	slog.Info("SUMMARY", summary...)
	if closeCode != 0 {
		return 3
	}
	return 0
}

func occupants(n *ningen.State, gid discord.GuildID) map[discord.ChannelID]int {
	out := map[discord.ChannelID]int{}
	vs, _ := n.Cabinet.VoiceStates(gid)
	for _, v := range vs {
		if v.ChannelID.IsValid() {
			out[v.ChannelID]++
		}
	}
	return out
}

func listOwned(n *ningen.State, me discord.UserID) int {
	guilds, err := n.Cabinet.Guilds()
	if err != nil {
		slog.Error("guilds", "err", err)
		return 1
	}
	owned := 0
	for _, g := range guilds {
		if g.OwnerID != me {
			continue
		}
		owned++
		fmt.Printf("guild %d  %q\n", g.ID, g.Name)
		chans, _ := n.Cabinet.Channels(g.ID)
		occ := occupants(n, g.ID)
		for _, c := range chans {
			if c.Type == discord.GuildVoice {
				fmt.Printf("  voice %d  %q  occupants=%d\n", c.ID, c.Name, occ[c.ID])
			}
		}
	}
	fmt.Printf("%d owned guild(s) of %d\n", owned, len(guilds))
	return 0
}
