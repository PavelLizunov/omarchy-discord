package voice

import (
	"bytes"
	"context"
	"log/slog"
	"os/exec"
	"testing"
	"time"

	"github.com/disgoorg/godave"
	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
)

// Real camera access units arrive individually; the previous decoder test put
// twenty encoded pictures into one synthetic RTP timestamp and missed buffering.
func TestCameraDecoderStreamingAccessUnits(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	encoded, err := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=size=160x90:rate=10", "-frames:v", "20", "-pix_fmt", "yuv420p", "-c:v", "libx264", "-threads", "1", "-tune", "zerolatency", "-x264-params", "keyint=1:aud=1", "-f", "h264", "pipe:1").Output()
	if err != nil {
		t.Fatal(err)
	}
	var frames [][]byte
	for _, part := range bytes.Split(encoded, []byte{0, 0, 0, 1, 9}) {
		if len(part) == 0 {
			continue
		}
		frames = append(frames, append([]byte{0, 0, 0, 1, 9}, part...))
	}
	if len(frames) != 20 {
		t.Fatalf("access units %d", len(frames))
	}
	v := newCamera()
	v.announce(dvoice.GatewayMessageDataVideo{UserID: 123, VideoSSRC: 42})
	if _, err = v.selectUser(123); err != nil {
		t.Fatal(err)
	}
	defer v.close()
	ds := godave.NewNoopSession(slog.Default(), "123", nil)
	var seq uint16
	for i, f := range frames {
		var nals [][]byte
		for _, part := range bytes.Split(f, []byte{0, 0, 0, 1}) {
			for _, nal := range bytes.Split(part, []byte{0, 0, 1}) {
				if len(nal) > 0 {
					nals = append(nals, nal)
				}
			}
		}
		for j, nal := range nals {
			seq++
			v.receive(&dvoice.Packet{SSRC: 42, Type: dvoice.H264PayloadType, Sequence: seq, Timestamp: uint32(i+1) * 9000, Marker: j == len(nals)-1, Opus: nal}, ds)
		}
		time.Sleep(100 * time.Millisecond)
		if s := v.snapshot(); s.Sequence > 0 {
			t.Logf("first picture after %d access units", i+1)
			return
		}
	}
	t.Fatalf("no streamed image after 20 access units without EOF: %+v", v.snapshot())
}
