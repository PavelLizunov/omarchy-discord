package voice

import (
	"errors"
	"io"
	"log/slog"
	"math"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/jfreymuth/pulse/proto"
	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
)

type diagnosticUDP struct {
	dvoice.UDPConn
	err error
}

func (u diagnosticUDP) Write(p []byte) (int, error) {
	if u.err != nil {
		return 0, u.err
	}
	return len(p), nil
}
func TestAudioDiagnosticsBoundaries(t *testing.T) {
	m := &audioMetrics{}
	d := m.snapshot()
	if d.InputAgeMS != -1 || d.SentAgeMS != -1 || d.OutputAgeMS != -1 {
		t.Fatal(d)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	tx, err := newCapture(log)
	if err != nil {
		t.Fatal(err)
	}
	tx.metrics = m
	pcm := make([]int16, frameLen)
	for i := range pcm {
		pcm[i] = int16(8000 * math.Sin(float64(i)*0.1))
	}
	tx.setMuted(true)
	tx.write(pcm)
	d = m.snapshot()
	if d.InputLevel == 0 || d.InputAgeMS < 0 || d.SentAgeMS != -1 {
		t.Fatal(d)
	}
	u := observedUDP{UDPConn: diagnosticUDP{}, metrics: m}
	u.Write(silenceFrame)
	if m.sentAt.Load() != 0 {
		t.Fatal("silence advertised as sent voice")
	}
	u.Write([]byte{1, 2})
	if m.sentAt.Load() == 0 {
		t.Fatal("successful UDP write not observed")
	}
	before := m.sentAt.Load()
	u.UDPConn = diagnosticUDP{err: errors.New("synthetic send failure")}
	u.Write([]byte{1})
	if m.sentAt.Load() != before || m.sendErrors.Load() != 1 {
		t.Fatal("failed send advertised")
	}
	rx := newReceiver(log, func(snowflake.ID, bool) {})
	rx.metrics = m
	defer rx.Close()
	rx.ReceiveOpusFrame(123, &dvoice.Packet{Opus: []byte{0xff}, Sequence: 1})
	if m.receivedAt.Load() == 0 || m.decodedAt.Load() != 0 {
		t.Fatal("received is not decoded")
	}
	rx.ReceiveOpusFrame(123, &dvoice.Packet{Opus: []byte{0xff}, Sequence: 2})
	rx.read(make([]int16, frameLen))
	if m.decodeErrors.Load() == 0 || m.outputAt.Load() != 0 {
		t.Fatal("invalid Opus advertised as output")
	}
}

func TestLocalOutputToneBoundedAndNotSent(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := newReceiver(log, nil)
	m := &audioMetrics{}
	r.metrics = m
	e := &Engine{st: State{Status: StatusConnected}, audio: &audio{rx: r, metrics: m, tx: &capture{frames: make(chan []byte, 4)}}}
	if err := e.TestOutput(); err != nil {
		t.Fatal(err)
	}
	tx, err := newCapture(log)
	if err != nil {
		t.Fatal(err)
	}
	tx.metrics = m
	tx.write([]int16{10000, 10000})
	if f, _ := tx.ProvideOpusFrame(); len(f) > 0 {
		t.Fatal("test leaked through microphone")
	}
	if err := e.TestOutput(); err == nil {
		t.Fatal("duplicate test stacked")
	}
	out := make([]int16, sampleRate*channels/2)
	r.read(out)
	if rms(out) == 0 || r.testSamples.Load() != 0 || m.sentAt.Load() != 0 || m.receivedAt.Load() != 0 || m.outputAt.Load() == 0 {
		t.Fatal("local test crossed media boundary")
	}
	r.read(out)
	if rms(out) != 0 {
		t.Fatal("tone continued")
	}
	e.st.Status = StatusIdle
	if e.TestOutput() == nil {
		t.Fatal("idle test accepted")
	}
	if d := e.Diagnostics(); d.Active {
		t.Fatal("detached stats active")
	}
	if age(time.Now().UnixMilli(), time.Now().Add(-2*time.Second).UnixMilli()) < 1900 {
		t.Fatal("stale age")
	}
	if !zeroVolume(proto.ChannelVolumes{0, 0}) || zeroVolume(nil) || zeroVolume(proto.ChannelVolumes{0, 1}) {
		t.Fatal("volume semantics")
	}
}
