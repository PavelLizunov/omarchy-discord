package voice

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"log/slog"
	"net"
	"os/exec"
	"testing"
	"time"

	"github.com/disgoorg/godave"
	"github.com/disgoorg/snowflake/v2"
	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
	daveframe "github.com/thomas-vilte/dave-go/frame"
	"golang.org/x/crypto/chacha20poly1305"
)

type pipelineConn struct {
	dvoice.Conn
	udp     dvoice.UDPConn
	dave    godave.Session
	gateway *cameraGatewayProbe
}

func (c pipelineConn) UserIDBySSRC(uint32) snowflake.ID { return 123 }
func (c pipelineConn) UDP() dvoice.UDPConn              { return c.udp }
func (c pipelineConn) DAVE() godave.Session             { return c.dave }
func (c pipelineConn) Gateway() dvoice.Gateway          { return c.gateway }

type pipelineAudio struct {
	dvoice.OpusFrameReceiver
	seen chan []byte
}

func (a pipelineAudio) ReceiveOpusFrame(_ snowflake.ID, p *dvoice.Packet) error {
	a.seen <- append([]byte(nil), p.Opus...)
	return nil
}

// Actual UDP discovery/decryption, rxDriver dispatch, WatchCamera subscription,
// fragmented DAVE frame assembly and FFmpeg output. Local synthetic keys/media;
// this deliberately does NOT attest a live Discord MLS session or camera.
func TestCameraPipelineEncryptedFragmentsAndAudio(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	encoded, err := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=size=160x90:rate=10", "-frames:v", "20", "-pix_fmt", "yuv420p", "-c:v", "libx264", "-threads", "1", "-tune", "zerolatency", "-x264-params", "keyint=1:aud=1", "-f", "h264", "pipe:1").Output()
	if err != nil {
		t.Fatal(err)
	}
	var frames [][]byte
	for _, part := range bytes.Split(encoded, []byte{0, 0, 0, 1, 9}) {
		if len(part) != 0 {
			frames = append(frames, append([]byte{0, 0, 0, 1, 9}, part...))
		}
	}
	if len(frames) != 20 {
		t.Fatalf("access units: %d", len(frames))
	}
	frameKey := bytes.Repeat([]byte{42}, 16)
	for _, mode := range dvoice.AllEncryptionModes {
		t.Run(string(mode), func(t *testing.T) {
			server, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
			if err != nil {
				t.Fatal(err)
			}
			defer server.Close()
			server.SetDeadline(time.Now().Add(10 * time.Second))
			address := make(chan *net.UDPAddr, 1)
			discoveryErr := make(chan error, 1)
			go func() {
				buf := make([]byte, 74)
				n, addr, err := server.ReadFromUDP(buf)
				if err == nil && n != 74 {
					err = fmt.Errorf("discovery size %d", n)
				}
				if err == nil {
					binary.BigEndian.PutUint16(buf[:2], 2)
					copy(buf[8:], []byte("127.0.0.1"))
					binary.BigEndian.PutUint16(buf[72:], uint16(addr.Port))
					_, err = server.WriteToUDP(buf, addr)
				}
				discoveryErr <- err
				address <- addr
			}()
			ds := encryptedCameraSession{Session: godave.NewNoopSession(slog.Default(), "123", nil), key: frameKey}
			u := dvoice.NewUDPConn(ds, func(uint32) snowflake.ID { return 123 })
			if _, _, err = u.Open(ctx, "127.0.0.1", server.LocalAddr().(*net.UDPAddr).Port, 100); err != nil {
				t.Fatal(err)
			}
			defer u.Close()
			if err = <-discoveryErr; err != nil {
				t.Fatal(err)
			}
			addr := <-address
			transportKey := bytes.Repeat([]byte{11}, 32)
			if err = u.SetSecretKey(mode, transportKey); err != nil {
				t.Fatal(err)
			}
			var aead cipher.AEAD
			if mode == dvoice.EncryptionModeAEADAES256GCMRTPSize {
				b, _ := aes.NewCipher(transportKey)
				aead, _ = cipher.NewGCM(b)
			} else {
				aead, _ = chacha20poly1305.NewX(transportKey)
			}
			v := newCamera()
			defer v.close()
			g := &cameraGatewayProbe{}
			conn := pipelineConn{udp: u, dave: ds, gateway: g}
			e := &Engine{gen: 1, video: v, conn: conn, st: State{Status: StatusConnected}}
			// Parse and dispatch the real opcode12 envelope, not a prefilled map.
			var announcement dvoice.GatewayMessage
			if err = json.Unmarshal([]byte(`{"op":12,"d":{"user_id":"123","audio_ssrc":10,"video_ssrc":40,"streams":[{"ssrc":42,"active":true,"quality":100}]}}`), &announcement); err != nil {
				t.Fatal(err)
			}
			e.onEvent(1, announcement.Op, announcement.D)
			if err = e.WatchCamera(ctx, 123, 1); err != nil {
				t.Fatal(err)
			}
			if g.wants[0]["42"] != 100 || g.wants[0]["10"] != 100 {
				t.Fatal(g.wants)
			}
			audio := pipelineAudio{seen: make(chan []byte, 1024)}
			driver := &rxDriver{log: slog.Default(), rx: audio, conn: conn, video: v}
			readCtx, stop := context.WithCancel(ctx)
			defer stop()
			done := make(chan struct{})
			go func() { defer close(done); driver.loop(readCtx) }()
			defer func() {
				stop()
				u.Close()
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Error("receive goroutine survived close")
				}
			}()
			var seq uint16
			var nonceSeq uint32
			var timestamp uint32
			send := func(pt byte, payload []byte, marker bool, extension, padding bool) {
				t.Helper()
				h := make([]byte, 12)
				h[0] = 0x80
				h[1] = pt
				if marker {
					h[1] |= 0x80
				}
				if extension {
					h[0] |= 0x10
					h = append(h, 0xbe, 0xde, 0, 1)
					payload = append([]byte{0x10, 1, 0, 0}, payload...)
				}
				if padding {
					h[0] |= 0x20
					payload = append(append([]byte(nil), payload...), 0, 0, 0, 4)
				}
				if pt == dvoice.RTPPayloadType {
					binary.BigEndian.PutUint16(h[2:], 1)
				} else {
					binary.BigEndian.PutUint16(h[2:], seq)
					seq++
				}
				binary.BigEndian.PutUint32(h[4:], timestamp)
				ssrc := uint32(42)
				if pt == dvoice.RTPPayloadType {
					ssrc = 10
				}
				binary.BigEndian.PutUint32(h[8:], ssrc)
				nonce := make([]byte, aead.NonceSize())
				binary.LittleEndian.PutUint32(nonce, nonceSeq)
				nonceSeq++
				wire := aead.Seal(append([]byte(nil), h...), nonce, payload, h)
				wire = append(wire, nonce[:4]...)
				if _, err := server.WriteToUDP(wire, addr); err != nil {
					t.Fatal(err)
				}
				// Bound the fixture burst; avoid testing kernel queue capacity instead.
				time.Sleep(time.Millisecond)
			}
			plainAudio := []byte{0xf8, 0xff, 0xfe}
			encAudio, err := daveframe.Encrypt(daveframe.EncryptParams{Plaintext: plainAudio, Key: frameKey, TruncatedNonce: 99})
			if err != nil {
				t.Fatal(err)
			}
			for frameIndex, raw := range frames {
				timestamp = uint32(frameIndex+1) * 9000
				var canonical []byte
				var clear []daveframe.Range
				for _, part := range bytes.Split(raw, []byte{0, 0, 0, 1}) {
					for _, nal := range bytes.Split(part, []byte{0, 0, 1}) {
						if len(nal) == 0 {
							continue
						}
						clear = append(clear, daveframe.Range{Offset: len(canonical), Length: 5})
						canonical = append(canonical, 0, 0, 0, 1)
						canonical = append(canonical, nal...)
					}
				}
				encrypted, err := daveframe.Encrypt(daveframe.EncryptParams{Plaintext: canonical, Key: frameKey, TruncatedNonce: uint32(frameIndex + 7), UnencryptedRanges: clear})
				if err != nil {
					t.Fatal(err)
				}
				for i, r := range clear {
					end := len(encrypted)
					if i+1 < len(clear) {
						end = clear[i+1].Offset
					}
					nal := encrypted[r.Offset+4 : end]
					for off := 1; off < len(nal); {
						n := min(700, len(nal)-off)
						fu := []byte{(nal[0] & 0xe0) | 28, nal[0] & 31}
						if off == 1 {
							fu[1] |= 0x80
						}
						if off+n == len(nal) {
							fu[1] |= 0x40
						}
						fu = append(fu, nal[off:off+n]...)
						last := i == len(clear)-1 && off+n == len(nal)
						send(dvoice.H264PayloadType, fu, last, true, true)
						off += n
					}
				}
				// Interleave authenticated audio while the selected video is active.
				if frameIndex == 9 {
					send(dvoice.RTPPayloadType, encAudio, true, false, false)
				}
				time.Sleep(30 * time.Millisecond)
			}
			select {
			case got := <-audio.seen:
				if !bytes.Equal(got, plainAudio) {
					t.Fatal(got)
				}
			case <-time.After(time.Second):
				t.Fatal("audio lost while receiving video")
			}
			deadline := time.Now().Add(6 * time.Second)
			for time.Now().Before(deadline) {
				s := v.snapshot()
				if s.Sequence > 0 && s.Image != "" {
					if s.Dispatched != s.Packets || s.SSRCMismatch != 0 {
						t.Fatal(s)
					}
					raw, err := base64.StdEncoding.DecodeString(s.Image)
					if err != nil {
						t.Fatal(err)
					}
					im, err := jpeg.Decode(bytes.NewReader(raw))
					if err != nil {
						t.Fatal(err)
					}
					if im.Bounds().Dx() != 640 || im.Bounds().Dy() != 360 {
						t.Fatal(im.Bounds())
					}
					return
				}
				time.Sleep(20 * time.Millisecond)
			}
			t.Fatalf("no frame after actual transport/dispatch: %+v", v.snapshot())
		})
	}
}
