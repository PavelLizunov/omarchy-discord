package voice

import (
	"bytes"
	"image/jpeg"
	"os/exec"
	"testing"
	"time"
)

func TestCameraPresentationCadenceAndNoUpscale(t *testing.T) {
	testCameraCadence(t, "640x360", 15)
}

func TestCameraHDPresentationCadence(t *testing.T) {
	testCameraCadence(t, "1920x1080", 25)
}

func testCameraCadence(t *testing.T, size string, minimum uint64) {
	raw, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size="+size+":rate=30", "-frames:v", "45", "-pix_fmt", "yuv420p", "-c:v", "libx264", "-threads", "1", "-tune", "zerolatency", "-x264-params", "keyint=30:aud=1", "-f", "h264", "pipe:1").Output()
	if err != nil {
		t.Fatal(err)
	}
	d, err := newCameraDecoderQuality(360)
	if err != nil {
		t.Fatal(err)
	}
	defer d.close()
	frames := bytes.Split(raw, []byte{0, 0, 0, 1, 9})
	start := time.Now()
	for _, f := range frames {
		if len(f) == 0 {
			continue
		}
		select {
		case d.frames <- append([]byte{0, 0, 0, 1, 9}, f...):
		case <-time.After(time.Second):
			t.Fatal("decoder backlog")
		}
		time.Sleep(time.Second / 30)
	}
	d.mu.Lock()
	sequence := d.sequence
	picture := append([]byte(nil), d.image...)
	d.mu.Unlock()
	t.Logf("published=%d decoded=%d dropped=%d resize_ms=%d", sequence, d.decoded.Load(), d.dropped.Load(), d.resizeMS.Load())
	if sequence < minimum {
		t.Fatalf("only %d images in %s; presentation still throttled", sequence, time.Since(start))
	}
	img, err := jpeg.Decode(bytes.NewReader(picture))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 640 || img.Bounds().Dy() != 360 {
		t.Fatal(img.Bounds())
	}
}
