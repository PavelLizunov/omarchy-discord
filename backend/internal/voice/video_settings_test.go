package voice

import (
	"bytes"
	"context"
	"encoding/base64"
	"image/jpeg"
	"log/slog"
	"os/exec"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/hraban/opus"
	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
)

func TestVideoQualityChangesSubscriptionAndDecoder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=size=160x90:rate=10", "-frames:v", "20", "-pix_fmt", "yuv420p", "-c:v", "libx264", "-threads", "1", "-tune", "zerolatency", "-x264-params", "keyint=1:aud=1", "-f", "h264", "pipe:1").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []int{360, 720, 1080} {
		v := newCamera()
		v.announce(dvoice.GatewayMessageDataVideo{UserID: 123, VideoSSRC: 42, AudioSSRC: 43})
		g := &cameraGatewayProbe{}
		c := cameraConnProbe{gateway: g}
		if _, err = v.selectUser(123); err != nil {
			t.Fatal(err)
		}
		if err = v.setQuality(ctx, c, q); err != nil {
			v.close()
			t.Fatal(err)
		}
		if q != 360 {
			w := g.wants[len(g.wants)-1]
			if w["43"] != float64(100) || w["pixelCounts"].(map[string]any)["42"] != float64(q*16/9*q) {
				t.Fatal(w)
			}
		}
		v.decoder.frames <- raw
		until := time.Now().Add(2 * time.Second)
		for v.snapshot().Image == "" && time.Now().Before(until) {
			time.Sleep(10 * time.Millisecond)
		}
		b, _ := base64.StdEncoding.DecodeString(v.snapshot().Image)
		image, err := jpeg.Decode(bytes.NewReader(b))
		v.close()
		if err != nil || image.Bounds().Dy() != q {
			t.Fatalf("quality%d decoder: %v", q, err)
		}
	}
	v := newCamera()
	if err = v.setQuality(ctx, cameraConnProbe{}, 999); err == nil {
		t.Fatal("invalid quality")
	}
}
func TestScreenVolumeIsIndependentAndBounded(t *testing.T) {
	a := newScreenAudio(slog.Default(), 123)
	defer a.rx.Close()
	r := newReceiver(slog.Default(), func(snowflake.ID, bool) {})
	defer r.Close()
	e := &Engine{audio: &audio{rx: r}}
	s := &Screen{audio: a, conn: cameraConnProbe{}}
	e.SetScreenAudio(s)
	enc, _ := opus.NewEncoder(sampleRate, channels, opus.AppAudio)
	pcm := make([]int16, frameLen)
	for i := range pcm {
		pcm[i] = int16(i%100*50 - 2500)
	}
	wire := make([]byte, maxOpusBytes)
	n, _ := enc.Encode(pcm, wire)
	feed := func(start uint16) {
		for i := uint16(0); i < 6; i++ {
			a.receive(123, &dvoice.Packet{Type: 120, Sequence: start + i, Opus: wire[:n]})
		}
	}
	zero := 0
	if err := s.SetVideoSettings(context.Background(), nil, &zero); err != nil {
		t.Fatal(err)
	}
	feed(1)
	out := make([]int16, frameLen)
	r.read(out)
	if rms(out) != 0 {
		t.Fatal("stream volume0")
	}
	full := 100
	s.SetVideoSettings(context.Background(), nil, &full)
	feed(20)
	r.read(out)
	if rms(out) == 0 {
		t.Fatal("stream volume100")
	}
	bad := 201
	if err := s.SetVideoSettings(context.Background(), nil, &bad); err == nil {
		t.Fatal("invalid volume")
	}
	if len(r.levels) != 0 {
		t.Fatal("voice preference changed")
	}
}
