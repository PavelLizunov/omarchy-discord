package voice

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/ningen/v3"
	dgateway "github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/godave"
	"github.com/disgoorg/snowflake/v2"
	"github.com/gorilla/websocket"
	"github.com/hraban/opus"
	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
)

func TestConvertVoiceState(t *testing.T) {
	ev := &gateway.VoiceStateUpdateEvent{VoiceState: discord.VoiceState{GuildID: 1, UserID: 2, SessionID: "s", SelfMute: true}}
	if u := toVoiceState(ev); u.ChannelID != nil || u.GuildID != 1 || u.UserID != 2 || u.SessionID != "s" || !u.SelfMute {
		t.Fatalf("ChannelID 0 must convert to nil: %+v", u)
	}
	ev.ChannelID = 3
	if u := toVoiceState(ev); u.ChannelID == nil || *u.ChannelID != 3 {
		t.Fatalf("ChannelID 3 lost: %+v", toVoiceState(ev))
	}
}

func TestConvertVoiceServer(t *testing.T) {
	ev := &gateway.VoiceServerUpdateEvent{Token: "t", GuildID: 1}
	if u := toVoiceServer(ev); u.Endpoint != nil || u.Token != "t" || u.GuildID != 1 {
		t.Fatalf("Endpoint \"\" must convert to nil: %+v", u)
	}
	ev.Endpoint = "host:443"
	if u := toVoiceServer(ev); u.Endpoint == nil || *u.Endpoint != "host:443" {
		t.Fatalf("Endpoint lost: %+v", toVoiceServer(ev))
	}
}

func tone(amp float64) []int16 {
	pcm := make([]int16, frameLen)
	for i := 0; i < frameSamples; i++ {
		v := int16(amp * math.Sin(2*math.Pi*440*float64(i)/sampleRate))
		pcm[2*i], pcm[2*i+1] = v, v
	}
	return pcm
}

func TestGateHold(t *testing.T) {
	c, err := newCapture(slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	next := func() []byte {
		select {
		case f := <-c.frames:
			return f
		default:
			t.Fatal("no frame queued")
			return nil
		}
	}
	c.frame(tone(0))
	if f := next(); f != nil {
		t.Fatal("silence must be gated")
	}
	c.frame(tone(8000))
	if f := next(); len(f) == 0 {
		t.Fatal("loud frame must be encoded")
	}
	for i := 0; i < holdFrames; i++ {
		c.frame(tone(0))
		if f := next(); len(f) == 0 {
			t.Fatalf("hold frame %d must still be encoded", i)
		}
	}
	c.frame(tone(0))
	if f := next(); f != nil {
		t.Fatal("gate must close after the hold")
	}
	c.setMuted(true)
	if _, err := c.write(tone(8000)); err != nil || len(c.frames) != 0 {
		t.Fatal("muted capture must queue nothing")
	}
	if f, err := c.ProvideOpusFrame(); f != nil || err != nil {
		t.Fatal("muted provider must return nil, nil")
	}
}

func TestIdleQueueDropsOldest(t *testing.T) {
	c, err := newCapture(slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < cap(c.frames)+2; i++ {
		c.frame(tone(8000))
	}
	c.hold = 0
	for i := 0; i < cap(c.frames); i++ {
		c.frame(tone(0))
	}
	for i := 0; i < cap(c.frames); i++ {
		if f, _ := c.ProvideOpusFrame(); f != nil {
			t.Fatalf("frame %d: stale speech left in the queue", i)
		}
	}
}

func TestMuteDropsCapturedAudio(t *testing.T) {
	c, err := newCapture(slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	loud := tone(8000)
	c.frame(loud)
	if _, err := c.write(loud[:frameLen/2]); err != nil {
		t.Fatal(err)
	}
	c.setMuted(true)
	if _, err := c.write(loud[frameLen/2:]); err != nil {
		t.Fatal(err)
	}
	if len(c.frames) != 0 || c.n != 0 {
		t.Fatalf("mute must drop queued and pending audio: frames=%d pending=%d", len(c.frames), c.n)
	}
	c.setMuted(false)
	if f, _ := c.ProvideOpusFrame(); f != nil {
		t.Fatal("pre-mute frame sent after unmute")
	}
	if _, err := c.write(loud); err != nil {
		t.Fatal(err)
	}
	if f, _ := c.ProvideOpusFrame(); len(f) == 0 {
		t.Fatal("capture must resume after unmute")
	}
}

func TestQueueOrdering(t *testing.T) {
	u := &user{}
	for _, s := range []uint16{12, 10, 11, 11, 9} {
		u.push(frame{seq: s, opus: []byte{byte(s)}})
	}
	for _, want := range []uint16{9, 10, 11, 12} {
		f, plc := u.take()
		if plc || f == nil || f.seq != want {
			t.Fatalf("want %d got %+v plc=%v", want, f, plc)
		}
	}
	if len(u.q) != 0 {
		t.Fatal("duplicate must be dropped")
	}
	u.push(frame{seq: 5})
	if len(u.q) != 0 {
		t.Fatal("late frame must be dropped")
	}
	u.push(frame{seq: 14})
	if f, plc := u.take(); f != nil || !plc {
		t.Fatal("missing 13 must conceal")
	}
	if f, _ := u.take(); f == nil || f.seq != 14 {
		t.Fatal("14 must follow the concealed frame")
	}
	for i := 0; i < maxPLC; i++ {
		if _, plc := u.take(); !plc {
			t.Fatalf("PLC %d expected", i)
		}
	}
	if f, plc := u.take(); f != nil || plc {
		t.Fatal("stream must end after maxPLC")
	}
	u.push(frame{seq: 3})
	if len(u.q) != 1 {
		t.Fatal("unprimed queue must accept any sequence")
	}
	u = &user{}
	for s := uint16(0); s < queueCap+2; s++ {
		u.push(frame{seq: s})
	}
	if len(u.q) != queueCap || u.q[0].seq != 2 {
		t.Fatalf("overflow must drop oldest: len=%d first=%d", len(u.q), u.q[0].seq)
	}
}

func TestMixerSaturation(t *testing.T) {
	if saturate(40000) != 32767 || saturate(-40000) != -32768 || saturate(-5) != -5 {
		t.Fatal("saturate")
	}
	enc, err := opus.NewEncoder(sampleRate, channels, opus.AppVoIP)
	if err != nil {
		t.Fatal(err)
	}
	pcm := make([]int16, frameLen)
	for i := range pcm {
		pcm[i] = 30000
	}
	data := make([]byte, maxOpusBytes)
	n, err := enc.Encode(pcm, data)
	if err != nil {
		t.Fatal(err)
	}
	var events []bool
	r := newReceiver(slog.Default(), func(_ snowflake.ID, on bool) { events = append(events, on) })
	for _, id := range []snowflake.ID{1, 2, 3} {
		for seq := uint16(0); seq < primeFrames+2; seq++ {
			if err := r.ReceiveOpusFrame(id, &dvoice.Packet{Sequence: seq, Opus: data[:n]}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(events) != 3 || !events[0] {
		t.Fatalf("speaking events %v", events)
	}
	out := make([]int16, frameLen*3)
	if k, _ := r.read(out); k != len(out) {
		t.Fatal("read must fill")
	}
	peak := int16(0)
	for _, s := range out[frameLen:] {
		if s > peak {
			peak = s
		}
	}
	if peak != 32767 {
		t.Fatalf("three users at 30000 must saturate, peak=%d", peak)
	}
	r.Close()
	if len(events) != 6 {
		t.Fatalf("Close must clear speaking for every user: %v", events)
	}
	r.Close()
}

func TestSpeakingSilenceAndTimeout(t *testing.T) {
	events := make(chan bool, 16)
	r := newReceiver(slog.Default(), func(_ snowflake.ID, on bool) { events <- on })
	next := func(want bool, what string) {
		select {
		case got := <-events:
			if got != want {
				t.Fatalf("%s: speaking=%v", what, got)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s: no speaking event", what)
		}
	}
	pkt := func(seq uint16, opus []byte) { _ = r.ReceiveOpusFrame(7, &dvoice.Packet{Sequence: seq, Opus: opus}) }
	pkt(1, []byte{1, 2, 3, 4})
	next(true, "first packet")
	for i := 0; i < silenceToStop; i++ {
		pkt(uint16(2+i), silenceFrame)
	}
	next(false, "silence burst")
	r.deafened.Store(true)
	r.mu.Lock()
	before := len(r.users[7].q)
	r.mu.Unlock()
	pkt(20, []byte{1, 2, 3, 4})
	next(true, "deafened still tracks")
	r.mu.Lock()
	after := len(r.users[7].q)
	r.users[7].timer.Reset(0)
	r.mu.Unlock()
	if after != before {
		t.Fatal("deafened must drop packets")
	}
	next(false, "timeout")
	r.CleanupUser(7)
	r.Close()
	select {
	case got := <-events:
		t.Fatalf("unexpected event %v", got)
	default:
	}
}

func TestEngineOffline(t *testing.T) {
	var states []State
	e := New(ningen.New("not-a-token"), Events{State: func(s State) { states = append(states, s) }}, nil)
	if err := e.SetMute(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if st := e.State(); !st.Muted || st.Status != StatusIdle {
		t.Fatalf("mute must persist while idle: %+v", st)
	}
	if err := e.Join(context.Background(), 1, 2); err == nil {
		t.Fatal("Join before READY must fail")
	}
	if st := e.State(); st.Status != StatusIdle || !st.Muted {
		t.Fatalf("failed Join must stay idle: %+v", st)
	}
	n := len(states)
	_ = e.Leave(context.Background())
	e.Close()
	if len(states) != n || n != 1 {
		t.Fatalf("Leave/Close while idle must not emit: %d events", len(states))
	}
}

type harness struct {
	t      *testing.T
	e      *Engine
	mu     sync.Mutex
	log    []string
	states []State
	lost   atomic.Bool
	block  chan struct{}
}

type stubConn struct {
	dvoice.Conn
	h      *harness
	guild  snowflake.ID
	op4    dvoice.StateUpdateFunc
	remove func()
	opened chan struct{}
	once   sync.Once
}

func (c *stubConn) GuildID() snowflake.ID { return c.guild }
func (c *stubConn) Open(ctx context.Context, ch snowflake.ID, mute, deaf bool) error {
	c.h.record(fmt.Sprintf("open %d/%d", c.guild, ch))
	if err := c.op4(ctx, c.guild, &ch, mute, deaf); err != nil {
		return err
	}
	select {
	case <-c.opened:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (c *stubConn) Close(ctx context.Context) {
	c.h.record(fmt.Sprintf("close %d", c.guild))
	_ = c.op4(ctx, c.guild, nil, false, false)
	c.remove()
}
func (c *stubConn) HandleVoiceStateUpdate(u dgateway.EventVoiceStateUpdate) {
	if u.ChannelID != nil {
		c.once.Do(func() { close(c.opened) })
	}
}
func (c *stubConn) HandleVoiceServerUpdate(dgateway.EventVoiceServerUpdate) {}
func (c *stubConn) SetOpusFrameProvider(dvoice.OpusFrameProvider)           {}
func (c *stubConn) SetOpusFrameReceiver(dvoice.OpusFrameReceiver)           {}

func newHarness(t *testing.T) *harness {
	h := &harness{t: t}
	n := ningen.New("not-a-token")
	if err := n.Cabinet.MyselfSet(discord.User{ID: 7}, false); err != nil {
		t.Fatal(err)
	}
	h.e = New(n, Events{State: func(s State) {
		h.mu.Lock()
		h.states = append(h.states, s)
		h.mu.Unlock()
	}}, nil)
	h.e.op4 = func(ctx context.Context, g snowflake.ID, c *snowflake.ID, mute, deaf bool) error {
		if c == nil {
			h.record(fmt.Sprintf("op4 %d leave", g))
			return nil
		}
		h.record(fmt.Sprintf("op4 %d/%d mute=%v deaf=%v", g, *c, mute, deaf))
		h.mu.Lock()
		b := h.block
		h.mu.Unlock()
		if b != nil {
			<-b
		}
		return nil
	}
	h.e.newAudio = func() (*audio, error) {
		tx, err := newCapture(slog.Default())
		if err != nil {
			return nil, err
		}
		return &audio{tx: tx, rx: newReceiver(slog.Default(), h.e.speaking), done: make(chan struct{}), serverLost: h.lost.Load}, nil
	}
	h.e.mgr = dvoice.NewManager(h.e.op4, 7, dvoice.WithConnCreateFunc(
		func(guildID, _ snowflake.ID, op4 dvoice.StateUpdateFunc, remove func(), _ ...dvoice.ConnConfigOpt) dvoice.Conn {
			return &stubConn{h: h, guild: guildID, op4: op4, remove: remove, opened: make(chan struct{})}
		}))
	t.Cleanup(h.e.Close)
	return h
}

func (h *harness) record(s string) {
	h.mu.Lock()
	h.log = append(h.log, s)
	h.mu.Unlock()
}

func (h *harness) logged() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.log)
}

func (h *harness) has(entry string) bool { return slices.Contains(h.logged(), entry) }

func (h *harness) noError() {
	h.t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, s := range h.states {
		if s.Status == StatusError {
			h.t.Fatalf("unexpected error state %+v (log %v)", s, h.log)
		}
	}
}

func (h *harness) wait(what string, cond func() bool) {
	h.t.Helper()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		if cond() {
			return
		}
	}
	h.t.Fatalf("timed out waiting for %s; log=%v state=%+v", what, h.logged(), h.e.State())
}

func (h *harness) echo(guild discord.GuildID, ch discord.ChannelID) {
	h.e.n.Call(&gateway.VoiceStateUpdateEvent{VoiceState: discord.VoiceState{GuildID: guild, UserID: 7, ChannelID: ch, SessionID: "s"}})
}

func (h *harness) join(guild discord.GuildID, ch discord.ChannelID, prev discord.GuildID) error {
	h.t.Helper()
	done := make(chan error, 1)
	go func() { done <- h.e.Join(context.Background(), guild, ch) }()
	h.wait("op 4 join", func() bool { return h.has(fmt.Sprintf("open %d/%d", guild, ch)) })
	if prev != 0 {
		h.echo(prev, 0)
		time.Sleep(20 * time.Millisecond)
	}
	h.echo(guild, ch)
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		h.t.Fatalf("Join did not return; log=%v", h.logged())
		return nil
	}
}

func (h *harness) conn() dvoice.Conn {
	h.e.mu.Lock()
	defer h.e.mu.Unlock()
	return h.e.conn
}

func TestJoinSwitchSameGuild(t *testing.T) {
	h := newHarness(t)
	if err := h.join(10, 11, 0); err != nil {
		t.Fatal(err)
	}
	old := h.conn()
	if err := h.join(10, 12, 10); err != nil {
		t.Fatal(err)
	}
	if st := h.e.State(); st.Status != StatusConnected || st.ChannelID != 12 {
		t.Fatalf("state after switch %+v", st)
	}
	if c := h.conn(); c == nil || c == old || h.e.mgr.GetConn(10) != c {
		t.Fatalf("switch must close the old conn before creating a fresh one: conn=%v old=%v mgr=%v", c, old, h.e.mgr.GetConn(10))
	}
	want := []string{
		"open 10/11", "op4 10/11 mute=false deaf=false",
		"close 10", "op4 10 leave",
		"open 10/12", "op4 10/12 mute=false deaf=false",
	}
	if got := h.logged(); !slices.Equal(got, want) {
		t.Fatalf("wire order\n got %v\nwant %v", got, want)
	}
	h.noError()
}

func TestJoinSwitchCrossGuild(t *testing.T) {
	h := newHarness(t)
	if err := h.join(10, 11, 0); err != nil {
		t.Fatal(err)
	}
	if err := h.join(20, 21, 10); err != nil {
		t.Fatal(err)
	}
	if st := h.e.State(); st.Status != StatusConnected || st.GuildID != 20 || st.ChannelID != 21 {
		t.Fatalf("state after switch %+v", st)
	}
	if h.e.mgr.GetConn(10) != nil || h.e.mgr.GetConn(20) != h.conn() {
		t.Fatal("old guild's conn must be gone, new guild's installed")
	}
	got := h.logged()
	if slices.Index(got, "op4 10 leave") > slices.Index(got, "open 20/21") {
		t.Fatalf("must leave the old guild before joining the new: %v", got)
	}
	h.noError()
	h.echo(10, 0)
	time.Sleep(20 * time.Millisecond)
	if st := h.e.State(); st.Status != StatusConnected {
		t.Fatalf("foreign guild echo must be ignored: %+v", st)
	}
	h.echo(20, 0)
	h.wait("kick", func() bool { return h.e.State().Status == StatusError })
	if st := h.e.State(); st.Error != "disconnected from the voice channel" {
		t.Fatalf("kick %+v", st)
	}
}

func TestSetMuteDoesNotOutliveLeave(t *testing.T) {
	h := newHarness(t)
	if err := h.join(10, 11, 0); err != nil {
		t.Fatal(err)
	}
	gate := make(chan struct{})
	release := sync.OnceFunc(func() { close(gate) })
	defer release()
	h.mu.Lock()
	h.block = gate
	h.mu.Unlock()
	muteDone := make(chan error, 1)
	go func() { muteDone <- h.e.SetMute(context.Background(), true) }()
	h.wait("mute op 4", func() bool { return h.has("op4 10/11 mute=true deaf=false") })
	leaveDone := make(chan struct{})
	go func() {
		_ = h.e.Leave(context.Background())
		close(leaveDone)
	}()
	time.Sleep(20 * time.Millisecond)
	if h.has("close 10") {
		t.Fatalf("Leave ran while the mute op 4 was in flight; it would have re-joined: %v", h.logged())
	}
	release()
	if err := <-muteDone; err != nil {
		t.Fatal(err)
	}
	<-leaveDone
	got := h.logged()
	if tail := got[len(got)-3:]; !slices.Equal(tail, []string{"op4 10/11 mute=true deaf=false", "close 10", "op4 10 leave"}) {
		t.Fatalf("mute must be sent before the leave: %v", got)
	}
	if st := h.e.State(); st.Status != StatusIdle || !st.Muted {
		t.Fatalf("after leave %+v", st)
	}
}

func TestLostAudioServerFailsCall(t *testing.T) {
	prev := audioWatchInterval
	audioWatchInterval = 5 * time.Millisecond
	defer func() { audioWatchInterval = prev }()
	h := newHarness(t)
	if err := h.join(10, 11, 0); err != nil {
		t.Fatal(err)
	}
	h.lost.Store(true)
	h.wait("failure leaves the channel", func() bool { return h.has("op4 10 leave") })
	if st := h.e.State(); st.Status != StatusError || st.Error != "audio server lost" {
		t.Fatalf("%+v", st)
	}
	if got := h.logged(); !slices.Equal(got[len(got)-2:], []string{"close 10", "op4 10 leave"}) {
		t.Fatalf("wire order: %v", got)
	}
}

func TestInvalidSessionRecoversOnceAndPreservesFlags(t *testing.T) {
	h := newHarness(t)
	if err := h.join(10, 11, 0); err != nil {
		t.Fatal(err)
	}
	_ = h.e.SetMute(context.Background(), true)
	_ = h.e.SetDeaf(context.Background(), true)
	countJoins := func() int {
		n := 0
		for _, s := range h.logged() {
			if s == "open 10/11" {
				n++
			}
		}
		return n
	}
	gen := h.e.gen
	h.e.observeClose(gen, &websocket.CloseError{Code: 4006})
	h.wait("fresh join", func() bool { return countJoins() == 2 })
	h.echo(10, 11)
	h.wait("reconnected", func() bool { return h.e.State().Status == StatusConnected })
	st := h.e.State()
	if !st.Muted || !st.Deafened || st.ChannelID != 11 {
		t.Fatalf("flags/room lost: %+v", st)
	}
	h.e.observeClose(h.e.gen, &websocket.CloseError{Code: 4006})
	h.wait("bounded failure", func() bool { return h.e.State().Status == StatusError })
	if countJoins() != 2 {
		t.Fatal("recovery loop")
	}
}

type fixtureDave struct{ godave.Session }

func (fixtureDave) SetChannelID(godave.ChannelID)    {}
func (fixtureDave) MaxSupportedProtocolVersion() int { return 0 }

func TestGateway4006FreshJoinThroughActualWebsocket(t *testing.T) {
	h := newHarness(t)
	if err := h.join(10, 11, 0); err != nil {
		t.Fatal(err)
	}
	countJoins := func() int {
		n := 0
		for _, s := range h.logged() {
			if s == "open 10/11" {
				n++
			}
		}
		return n
	}
	closeNow := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		// Isolate the actual close delivery from Disgo's unrelated Hello/heartbeat
		// startup race; the full-handshake race is retained in the evidence log.
		_ = c.WriteJSON(map[string]any{"op": 2, "s": 1, "d": map[string]any{"ssrc": 1, "ip": "127.0.0.1", "port": 9, "modes": []string{"aead_xchacha20_poly1305_rtpsize"}}})
		<-closeNow
		_ = c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(4006, "fixture invalid session"))
	}))
	defer server.Close()
	old := websocket.DefaultDialer
	dialer := *old
	dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	websocket.DefaultDialer = &dialer
	defer func() { websocket.DefaultDialer = old }()
	g := h.e.gatewayCreate(fixtureDave{}, func(dvoice.Gateway, dvoice.Opcode, int, dvoice.GatewayMessageData) {}, nil)
	defer g.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := g.Open(ctx, dvoice.State{Endpoint: strings.TrimPrefix(server.URL, "https://"), SessionID: "fixture", GuildID: 10, ChannelID: 11, UserID: 7}); err != nil {
		t.Fatal(err)
	}
	close(closeNow)
	h.wait("fresh join from actual gateway close", func() bool { return countJoins() == 2 })
	h.echo(10, 11)
	h.wait("recovered", func() bool { return h.e.State().Status == StatusConnected })
}

func TestRecoveryCancelledByLeaveAndStaleClose(t *testing.T) {
	h := newHarness(t)
	if err := h.join(10, 11, 0); err != nil {
		t.Fatal(err)
	}
	gen := h.e.gen
	_ = h.e.Leave(context.Background())
	h.e.observeClose(gen, &websocket.CloseError{Code: 4006})
	if err := h.e.join(context.Background(), 10, 11, &gen); !errors.Is(err, context.Canceled) {
		t.Fatalf("stale recovery: %v", err)
	}
	if st := h.e.State(); st.Status != StatusIdle {
		t.Fatalf("leave resurrected: %+v", st)
	}
	if n := len(h.logged()); n != 4 {
		t.Fatalf("unexpected account command: %v", h.logged())
	}
}

func TestTerminalVoiceCodesNeverRecover(t *testing.T) {
	for _, code := range []int{4004, 4014, 4021, 4022} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			h := newHarness(t)
			if err := h.join(10, 11, 0); err != nil {
				t.Fatal(err)
			}
			h.e.observeClose(h.e.gen, &websocket.CloseError{Code: code})
			if st := h.e.State(); st.Status != StatusError {
				t.Fatalf("terminal close: %+v", st)
			}
			if len(h.logged()) != 4 {
				t.Fatalf("terminal close rejoined: %v", h.logged())
			}
		})
	}
}

func TestResumedBoundToGeneration(t *testing.T) {
	e := New(ningen.New("not-a-token"), Events{}, nil)
	e.mu.Lock()
	e.gen = 5
	e.st = State{Status: StatusConnecting, GuildID: 1, ChannelID: 2}
	e.mu.Unlock()
	e.onEvent(4, dvoice.OpcodeResumed, dvoice.GatewayMessageDataResumed{})
	if st := e.State(); st.Status != StatusConnecting {
		t.Fatalf("a stale call's resume must not connect this one: %+v", st)
	}
	e.onEvent(5, dvoice.OpcodeResumed, dvoice.GatewayMessageDataResumed{})
	if st := e.State(); st.Status != StatusConnected {
		t.Fatalf("own resume must connect: %+v", st)
	}
}

func TestStaleGatewayNeverDials(t *testing.T) {
	e := New(ningen.New("not-a-token"), Events{}, nil)
	evh := func(dvoice.Gateway, dvoice.Opcode, int, dvoice.GatewayMessageData) {}
	g := e.gatewayCreate(nil, evh, nil)
	e.mu.Lock()
	e.gen++
	e.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := g.Open(ctx, dvoice.State{Endpoint: "127.0.0.1:1"})
	var ce *websocket.CloseError
	if !errors.As(err, &ce) || ce.Code != dvoice.GatewayCloseEventCodeDisconnected.Code {
		t.Fatalf("stale gateway must be refused with a non-resumable close (ends disgo's retry loop), got %v", err)
	}
	g = e.gatewayCreate(nil, evh, nil)
	ctx, cancel = context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := g.Open(ctx, dvoice.State{Endpoint: "127.0.0.1:1"}); errors.As(err, &ce) {
		t.Fatalf("live gateway must dial, got %v", err)
	}
}

type notReady struct{ godave.Session }

func (notReady) Ready() bool { return false }

type daveConn struct {
	dvoice.Conn
	calls atomic.Int32
}

func (c *daveConn) DAVE() godave.Session {
	c.calls.Add(1)
	return notReady{}
}

func TestSenderCloseWaitsForStart(t *testing.T) {
	c := &daveConn{}
	s := newSender(slog.Default(), &capture{frames: make(chan []byte)}, c)
	s.Open()
	s.Close()
	s.Close()
	if c.calls.Load() == 0 {
		t.Fatal("Close must wait for the sender goroutine to start")
	}
	time.Sleep(50 * time.Millisecond)
	n := c.calls.Load()
	time.Sleep(50 * time.Millisecond)
	if c.calls.Load() != n {
		t.Fatal("sender goroutine survived Close")
	}
}

func TestPulseSmoke(t *testing.T) {
	if os.Getenv("VOICE_PULSE_SMOKE") == "" {
		t.Skip("set VOICE_PULSE_SMOKE=1")
	}
	a, err := openAudio(slog.Default(), func(snowflake.ID, bool) {})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	frames := 0
	for i := 0; i < 5; i++ {
		if f, err := a.tx.ProvideOpusFrame(); err != nil {
			t.Fatal(err)
		} else if f != nil {
			frames++
		}
	}
	t.Logf("capture running=%v playback running=%v underflow=%v frames_above_gate=%d rec_err=%v play_err=%v",
		a.rec.Running(), a.play.Running(), a.play.Underflow(), frames, a.rec.Error(), a.play.Error())
	if !a.rec.Running() || !a.play.Running() {
		t.Fatal("streams must be running")
	}
	a.close()
	if !a.rec.Closed() || !a.play.Closed() {
		t.Fatal("streams must be closed")
	}
}
