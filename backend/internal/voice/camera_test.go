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

	"github.com/disgoorg/godave"
	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
)

func TestCameraMVPDecodeAndStop(t *testing.T) {
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	encoded, err := exec.CommandContext(ctx, path, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=size=160x90:rate=10", "-frames:v", "20", "-pix_fmt", "yuv420p", "-c:v", "libx264", "-threads", "1", "-tune", "zerolatency", "-x264-params", "keyint=1", "-f", "h264", "pipe:1").Output()
	if err != nil {
		t.Fatal(err)
	}
	v := newCamera()
	v.announce(dvoice.GatewayMessageDataVideo{UserID: 123, VideoSSRC: 42})
	_, err = v.selectUser(123)
	if err != nil {
		t.Fatal(err)
	}
	defer v.close()
	// The packet path reconstructs a STAP-A frame before DAVE and the actual decoder.
	var aggregate = []byte{24}
	for _, nal := range bytes.Split(encoded, []byte{0, 0, 0, 1}) {
		if len(nal) == 0 {
			continue
		}
		for _, n := range bytes.Split(nal, []byte{0, 0, 1}) {
			if len(n) == 0 {
				continue
			}
			if len(n) > 65535 {
				t.Fatal("fixture NAL too large")
			}
			aggregate = append(aggregate, byte(len(n)>>8), byte(len(n)))
			aggregate = append(aggregate, n...)
		}
	}
	v.receive(&dvoice.Packet{SSRC: 42, Type: 105, Sequence: 1, Timestamp: 1, Marker: true, Opus: aggregate}, godave.NewNoopSession(slog.Default(), "123", nil))
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		s := v.snapshot()
		if s.Sequence > 0 {
			raw, err := base64.StdEncoding.DecodeString(s.Image)
			if err != nil {
				t.Fatal(err)
			}
			im, err := jpeg.Decode(bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			if im.Bounds().Dx() > 640 || im.Bounds().Dy() > 360 {
				t.Fatal(im.Bounds())
			}
			v.close()
			if v.snapshot().UserID != "" || v.snapshot().Image != "" {
				t.Fatal("camera not cleared")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no decoded image: %+v", v.snapshot())
}
