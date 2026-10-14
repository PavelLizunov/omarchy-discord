package voice

import (
	"log/slog"
	"math"
	"testing"

	"github.com/disgoorg/snowflake/v2"
	"github.com/hraban/opus"
	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
)

func TestScreenAudioMixUsesCallOutputAndLocalControls(t *testing.T) {
	a := newScreenAudio(slog.Default(), 123)
	defer a.rx.Close()
	r := newReceiver(slog.Default(), func(_ snowflake.ID, _ bool) {})
	defer r.Close()
	e := &Engine{audio: &audio{rx: r}, st: State{Status: StatusConnected}}
	s := &Screen{audio: a}
	e.SetScreenAudio(s)
	enc, err := opus.NewEncoder(sampleRate, channels, opus.AppAudio)
	if err != nil {
		t.Fatal(err)
	}
	pcm := make([]int16, frameLen)
	for i := range pcm {
		pcm[i] = int16(3000 * math.Sin(float64(i/channels)*2*math.Pi*440/sampleRate))
	}
	wire := make([]byte, maxOpusBytes)
	n, err := enc.Encode(pcm, wire)
	if err != nil {
		t.Fatal(err)
	}
	feed := func(start uint16) {
		for i := uint16(0); i < 6; i++ {
			a.receive(123, &dvoice.Packet{Type: 120, Sequence: start + i, Opus: wire[:n]})
		}
	}
	a.receive(999, &dvoice.Packet{Type: 120, Opus: wire[:n]})
	a.receive(123, &dvoice.Packet{Type: 105, Opus: wire[:n]})
	if a.packets.Load() != 0 {
		t.Fatal("unselected/codec admitted")
	}
	out := make([]int16, frameLen)
	feed(10)
	r.read(out)
	if rms(out) == 0 || a.rx.metrics.decodedAt.Load() == 0 {
		t.Fatal("stream did not reach shared output")
	}
	muted := true
	if err = e.SetUserAudio(123, nil, &muted); err != nil {
		t.Fatal(err)
	}
	feed(20)
	r.read(out)
	if rms(out) != 0 {
		t.Fatal("local mute bypassed")
	}
	muted = false
	e.SetUserAudio(123, nil, &muted)
	r.deafened.Store(true)
	feed(30)
	r.read(out)
	if rms(out) != 0 {
		t.Fatal("deafen bypassed")
	}
	r.deafened.Store(false)
	feed(40)
	r.read(out)
	if rms(out) == 0 {
		t.Fatal("microphone mute should not block screen audio")
	}
	e.SetScreenAudio(nil)
	r.read(out)
	if rms(out) != 0 {
		t.Fatal("audio survived stop")
	}
}
