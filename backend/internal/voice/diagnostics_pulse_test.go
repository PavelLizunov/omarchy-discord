package voice

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/jfreymuth/pulse/proto"
)

// Opt in only against the private null-device Pulse server used by the harness.
func TestPulseDiagnosticsSmoke(t *testing.T) {
	if os.Getenv("VOICE_DIAGNOSTICS_NULL_SMOKE") != "1" {
		t.Skip("private null audio server required")
	}
	a, err := openAudio(slog.Default(), func(snowflake.ID, bool) {})
	if err != nil {
		t.Fatal(err)
	}
	defer a.close()
	time.Sleep(150 * time.Millisecond)
	d := a.metrics.snapshot()
	a.devices(&d)
	if d.InputBlocked == nil || d.OutputBlocked == nil || d.InputDevice == "" || d.OutputDevice == "" || d.DeviceError != "" {
		t.Fatalf("actual streams not identified: %+v", d)
	}
	if d.InputAgeMS < 0 {
		t.Fatal("no actual capture callbacks")
	}
	a.rx.testSamples.Store(sampleRate * channels / 2)
	deadline := time.Now().Add(2 * time.Second)
	for a.rx.testSamples.Load() > 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if a.rx.testSamples.Load() != 0 || a.metrics.outputAt.Load() == 0 || a.metrics.sentAt.Load() != 0 {
		t.Fatal("actual local tone not bounded/local")
	}
	var stream proto.GetSinkInputInfoReply
	if err := a.client.RawRequest(&proto.GetSinkInputInfo{SinkInputIndex: a.play.StreamInputIndex()}, &stream); err != nil {
		t.Fatal(err)
	}
	if err := a.client.RawRequest(&proto.SetSinkMute{SinkIndex: stream.SinkIndex, Mute: true}, nil); err != nil {
		t.Fatal(err)
	}
	d = a.metrics.snapshot()
	a.devices(&d)
	if d.OutputBlocked == nil || !*d.OutputBlocked {
		t.Fatalf("system mute missing: %+v", d)
	}
	if err := a.client.RawRequest(&proto.SetSinkMute{SinkIndex: stream.SinkIndex, Mute: false}, nil); err != nil {
		t.Fatal(err)
	}
}
