//go:build cgo

package voice

import (
	"context"
	"log/slog"
	"math"
	"os"
	"testing"
	"time"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/ningen/v3"
	dvoice "github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/snowflake/v2"
	"github.com/hraban/opus"
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
	c.muted.Store(true)
	c.frame(tone(8000))
	if len(c.frames) != 0 {
		t.Fatal("muted capture must queue nothing")
	}
	if f, err := c.ProvideOpusFrame(); f != nil || err != nil {
		t.Fatal("muted provider must return nil, nil")
	}
}

func TestQueueOrdering(t *testing.T) {
	u := &user{}
	for _, s := range []uint16{12, 10, 11, 11, 9} { // out of order, a dup, then late once primed
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
	u.push(frame{seq: 5}) // late: next is 13
	if len(u.q) != 0 {
		t.Fatal("late frame must be dropped")
	}
	// Gap → PLC, then the frame after the gap.
	u.push(frame{seq: 14})
	if f, plc := u.take(); f != nil || !plc {
		t.Fatal("missing 13 must conceal")
	}
	if f, _ := u.take(); f == nil || f.seq != 14 {
		t.Fatal("14 must follow the concealed frame")
	}
	// Empty queue conceals up to maxPLC, then the stream ends and re-primes.
	for i := 0; i < maxPLC; i++ {
		if _, plc := u.take(); !plc {
			t.Fatalf("PLC %d expected", i)
		}
	}
	if f, plc := u.take(); f != nil || plc {
		t.Fatal("stream must end after maxPLC")
	}
	u.push(frame{seq: 3}) // sequence reset after silence: accepted, not "late"
	if len(u.q) != 1 {
		t.Fatal("unprimed queue must accept any sequence")
	}
	// Overflow drops the oldest.
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
	for _, s := range out[frameLen:] { // skip the first frame: opus pre-skip
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
	r.Close() // idempotent
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
	r.users[7].timer.Reset(0) // stand in for 250 ms without packets
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

// TestEngineOffline exercises the lock/emission flow without a session:
// flags persist while idle, Join refuses before READY, Leave is idempotent.
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

// TestPulseSmoke opens the real record/playback streams for half a second.
// Needs a Pulse/PipeWire socket: VOICE_PULSE_SMOKE=1 go test ./internal/voice -run Pulse
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
