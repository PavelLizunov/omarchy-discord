package voice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image/jpeg"
	"log/slog"
	"os/exec"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/disgoorg/godave"
	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
	daveframe "github.com/thomas-vilte/dave-go/frame"
)

type cameraGatewayProbe struct {
	dvoice.Gateway
	wants []map[string]any
}

func (g *cameraGatewayProbe) SSRC() uint32 { return 100 }
func (g *cameraGatewayProbe) Send(_ context.Context, op dvoice.Opcode, data dvoice.GatewayMessageData) error {
	if op == dvoice.OpcodeMediaSinkWants {
		var wants map[string]any
		if err := json.Unmarshal(data.(dvoice.GatewayMessageDataUnknown), &wants); err != nil {
			return err
		}
		g.wants = append(g.wants, wants)
	}
	return nil
}

type cameraConnProbe struct {
	dvoice.Conn
	gateway *cameraGatewayProbe
}

func (c cameraConnProbe) Gateway() dvoice.Gateway { return c.gateway }
func (cameraConnProbe) UDP() dvoice.UDPConn       { return nil }

// Exercise the actual subscription commands, not a model of the protocol.
func TestCameraSubscriptionDoesNotGloballyDisableCallAudio(t *testing.T) {
	v := newCamera()
	v.announce(dvoice.GatewayMessageDataVideo{UserID: 1, AudioSSRC: 10, VideoSSRC: 20})
	g := &cameraGatewayProbe{}
	e := &Engine{video: v, conn: cameraConnProbe{gateway: g}, st: State{Status: StatusConnected}}
	defer v.close()
	for i, id := range []snowflake.ID{1, 0} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err := e.WatchCamera(ctx, id, uint64(i+1))
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		wants := g.wants[i]
		if _, global := wants["any"]; global {
			t.Fatalf("camera overwrites unspecified media: %v", wants)
		}
		if wants["10"] != float64(100) {
			t.Fatalf("call audio disabled: %v", wants)
		}
		want := 100
		if id == 0 {
			want = 0
		}
		if id != 0 {
			pixels, ok := wants["pixelCounts"].(map[string]any)
			if !ok || pixels["20"] != float64(640*360) {
				t.Fatal("missing visible pixel hint", wants)
			}
		}
		if wants["20"] != float64(want) {
			t.Fatalf("camera request not applied: %v", wants)
		}
	}
}

func TestCameraSinkWantsPreservesAudioAndStopsOnlyVideo(t *testing.T) {
	v := newCamera()
	v.announce(dvoice.GatewayMessageDataVideo{UserID: 1, AudioSSRC: 10, VideoSSRC: 20, Streams: []dvoice.VideoStream{{SSRC: 21, Active: true}}})
	v.announce(dvoice.GatewayMessageDataVideo{UserID: 2, AudioSSRC: 11, VideoSSRC: 22})
	v.announceAudio(12)
	for _, selected := range []uint32{21, 0} {
		var wants map[string]any
		if err := json.Unmarshal(v.sinkWants(selected), &wants); err != nil {
			t.Fatal(err)
		}
		if _, global := wants["any"]; global {
			t.Fatal("camera must not overwrite unspecified media")
		}
		for _, audio := range []string{"10", "11", "12"} {
			if wants[audio] != float64(100) {
				t.Fatalf("audio %s disabled: %v", audio, wants)
			}
		}
		for _, video := range []string{"20", "21", "22"} {
			want := 0
			if video == "21" && selected == 21 {
				want = 100
			}
			if wants[video] != float64(want) {
				t.Fatalf("video %s: %v", video, wants)
			}
		}
	}
}

type encryptedCameraSession struct {
	godave.Session
	key []byte
}

func (s encryptedCameraSession) Decrypt(_ godave.UserID, in, out []byte) (int, error) {
	plain, _, err := daveframe.Decrypt(daveframe.DecryptParams{Ciphertext: in, Key: s.key})
	return copy(out, plain), err
}

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
	// Reconstruct and authenticate real DAVE frame bytes before FFmpeg. This
	// uses a synthetic key, not a live MLS session or a remote camera.
	var canonical []byte
	var clear []daveframe.Range
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
			clear = append(clear, daveframe.Range{Offset: len(canonical), Length: 5})
			canonical = append(canonical, 0, 0, 0, 1)
			canonical = append(canonical, n...)
		}
	}
	key := bytes.Repeat([]byte{42}, 16)
	encrypted, err := daveframe.Encrypt(daveframe.EncryptParams{Plaintext: canonical, Key: key, TruncatedNonce: 7, UnencryptedRanges: clear})
	if err != nil {
		t.Fatal(err)
	}
	// Keep original NAL boundaries: arbitrary ciphertext may contain start codes.
	for i, r := range clear {
		end := len(encrypted)
		if i+1 < len(clear) {
			end = clear[i+1].Offset
		}
		nal := encrypted[r.Offset+4 : end]
		if len(nal) > 65535 {
			t.Fatal("encrypted fixture NAL too large")
		}
		aggregate = append(aggregate, byte(len(nal)>>8), byte(len(nal)))
		aggregate = append(aggregate, nal...)
	}
	v.receive(&dvoice.Packet{SSRC: 42, Type: dvoice.H264PayloadType, Sequence: 1, Timestamp: 1, Marker: true, Opus: aggregate}, encryptedCameraSession{Session: godave.NewNoopSession(slog.Default(), "123", nil), key: key})
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		s := v.snapshot()
		if s.Sequence > 0 {
			if s.Packets != 1 || s.PacketAgeMS < 0 {
				t.Fatal("camera packet observation missing", s)
			}
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
			if v.snapshot().UserID != "" || v.snapshot().Image != "" || v.snapshot().Packets != 0 || v.snapshot().PacketAgeMS != -1 {
				t.Fatal("camera not cleared")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no decoded image: %+v", v.snapshot())
}
