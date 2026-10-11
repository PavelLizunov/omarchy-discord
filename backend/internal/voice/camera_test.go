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
)

type cameraGatewayProbe struct {
	dvoice.Gateway
	wants []map[string]int
}

func (g *cameraGatewayProbe) SSRC() uint32 { return 100 }
func (g *cameraGatewayProbe) Send(_ context.Context, op dvoice.Opcode, data dvoice.GatewayMessageData) error {
	if op == dvoice.OpcodeMediaSinkWants {
		var wants map[string]int
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
		if wants["10"] != 100 {
			t.Fatalf("call audio disabled: %v", wants)
		}
		want := 100
		if id == 0 {
			want = 0
		}
		if wants["20"] != want {
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
		var wants map[string]int
		if err := json.Unmarshal(v.sinkWants(selected), &wants); err != nil {
			t.Fatal(err)
		}
		if _, global := wants["any"]; global {
			t.Fatal("camera must not overwrite unspecified media")
		}
		for _, audio := range []string{"10", "11", "12"} {
			if wants[audio] != 100 {
				t.Fatalf("audio %s disabled: %v", audio, wants)
			}
		}
		for _, video := range []string{"20", "21", "22"} {
			want := 0
			if video == "21" && selected == 21 {
				want = 100
			}
			if wants[video] != want {
				t.Fatalf("video %s: %v", video, wants)
			}
		}
	}
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
