package voice

import (
	"bytes"
	"context"
	"crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image/jpeg"
	"log/slog"
	"net"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/godave"
	"github.com/disgoorg/snowflake/v2"
	"github.com/hraban/opus"
	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
	"golang.org/x/crypto/chacha20poly1305"
)

type screenGatewayFixture struct {
	dvoice.Gateway
	handler dvoice.EventHandlerFunc
	mu      sync.Mutex
	state   dvoice.State
	opcodes []dvoice.Opcode
	closed  bool
	key     []byte
	port    int
}

func (g *screenGatewayFixture) SSRC() uint32 { return 100 }
func (g *screenGatewayFixture) Status() dvoice.Status {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return dvoice.StatusDisconnected
	}
	return dvoice.StatusReady
}
func (g *screenGatewayFixture) Open(ctx context.Context, st dvoice.State) error {
	g.state = st
	g.handler(g, dvoice.OpcodeReady, 1, dvoice.GatewayMessageDataReady{IP: "127.0.0.1", Port: g.port, SSRC: 100, Modes: []dvoice.EncryptionMode{dvoice.EncryptionModeAEADXChaCha20Poly1305RTPSize}})
	g.handler(g, dvoice.OpcodeSessionDescription, 2, dvoice.GatewayMessageDataSessionDescription{Mode: dvoice.EncryptionModeAEADXChaCha20Poly1305RTPSize, SecretKey: g.key})
	g.handler(g, dvoice.OpcodeVideo, 3, dvoice.GatewayMessageDataVideo{UserID: 123, VideoSSRC: 42, AudioSSRC: 43})
	return nil
}
func (g *screenGatewayFixture) Send(_ context.Context, op dvoice.Opcode, _ dvoice.GatewayMessageData) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.opcodes = append(g.opcodes, op)
	return nil
}
func (g *screenGatewayFixture) Close() { g.mu.Lock(); g.closed = true; g.mu.Unlock() }

// Exercises OpenScreen, independent UDP discovery, AEAD admission, shared
// decoder, latest JPEG and concurrent shutdown. Inert gateway/noop DAVE here;
// camera_pipeline_test separately verifies synthetic frame authentication.
func TestScreenTransportReceivesFrameAndClosesWithoutVoice(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	raw, e := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=size=160x90:rate=10", "-frames:v", "20", "-pix_fmt", "yuv420p", "-c:v", "libx264", "-threads", "1", "-tune", "zerolatency", "-x264-params", "keyint=1:aud=1", "-f", "h264", "pipe:1").Output()
	if e != nil {
		t.Fatal(e)
	}
	udp, e := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if e != nil {
		t.Fatal(e)
	}
	defer udp.Close()
	udp.SetDeadline(time.Now().Add(10 * time.Second))
	address := make(chan *net.UDPAddr, 1)
	errs := make(chan error, 1)
	go func() {
		buf := make([]byte, 74)
		n, a, e := udp.ReadFromUDP(buf)
		if e == nil && n != 74 {
			e = fmt.Errorf("discovery %d", n)
		}
		if e == nil {
			binary.BigEndian.PutUint16(buf, 2)
			copy(buf[8:], "127.0.0.1")
			binary.BigEndian.PutUint16(buf[72:], uint16(a.Port))
			_, e = udp.WriteToUDP(buf, a)
		}
		errs <- e
		address <- a
	}()
	key := bytes.Repeat([]byte{12}, 32)
	g := &screenGatewayFixture{key: key, port: udp.LocalAddr().(*net.UDPAddr).Port}
	screen, e := openScreen(ctx, dvoice.State{GuildID: 500, ChannelID: 600, MediaGroupID: 499, UserID: 700, SessionID: "fixture", Token: "fixture", Endpoint: "fixture.discord.media"}, 123, slog.Default(), dvoice.WithConnDaveSessionCreateFunc(godave.NewNoopSession), dvoice.WithConnGatewayCreateFunc(func(_ godave.Session, h dvoice.EventHandlerFunc, _ dvoice.CloseHandlerFunc, _ ...dvoice.GatewayConfigOpt) dvoice.Gateway {
		g.handler = h
		return g
	}))
	if e != nil {
		t.Fatal(e)
	}
	defer screen.Close()
	if e := <-errs; e != nil {
		t.Fatal(e)
	}
	addr := <-address
	if !g.state.MediaOnly || g.state.MediaGroupID != 499 {
		t.Fatal("shared voice identity", g.state)
	}
	var aead cipher.AEAD
	aead, e = chacha20poly1305.NewX(key)
	if e != nil {
		t.Fatal(e)
	}
	var seq uint16
	var nonceID uint32
	for i, part := range bytes.Split(raw, []byte{0, 0, 0, 1, 9}) {
		if len(part) == 0 {
			continue
		}
		frame := append([]byte{0, 0, 0, 1, 9}, part...)
		var nals [][]byte
		for _, p := range bytes.Split(frame, []byte{0, 0, 0, 1}) {
			for _, n := range bytes.Split(p, []byte{0, 0, 1}) {
				if len(n) > 0 {
					nals = append(nals, n)
				}
			}
		}
		for j, nal := range nals {
			for off := 1; off < len(nal); {
				n := min(600, len(nal)-off)
				if off == 1 && n == len(nal)-1 && n > 1 {
					n--
				}
				fu := []byte{(nal[0] & 0xe0) | 28, nal[0] & 31}
				if off == 1 {
					fu[1] |= 128
				}
				if off+n == len(nal) {
					fu[1] |= 64
				}
				fu = append(fu, nal[off:off+n]...)
				if off == 1 && off+n == len(nal) {
					fu = append([]byte(nil), nal...)
				} // Tiny NAL uses single-packet format.
				h := make([]byte, 12)
				h[0] = 0x80
				h[1] = dvoice.H264PayloadType
				if j == len(nals)-1 && off+n == len(nal) {
					h[1] |= 128
				}
				binary.BigEndian.PutUint16(h[2:], seq)
				seq++
				binary.BigEndian.PutUint32(h[4:], uint32(i*9000))
				binary.BigEndian.PutUint32(h[8:], 42)
				nonce := make([]byte, 24)
				binary.LittleEndian.PutUint32(nonce, nonceID)
				nonceID++
				wire := aead.Seal(h, nonce, fu, h)
				wire = append(wire, nonce[:4]...)
				if _, e = udp.WriteToUDP(wire, addr); e != nil {
					t.Fatal(e)
				}
				off += n
				time.Sleep(time.Millisecond)
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	until := time.Now().Add(3 * time.Second)
	for {
		snapshot := screen.Snapshot()
		if snapshot.Sequence > 0 && snapshot.Image != "" {
			b, e := base64.StdEncoding.DecodeString(snapshot.Image)
			if e != nil {
				t.Fatal(e)
			}
			img, e := jpeg.Decode(bytes.NewReader(b))
			if e != nil || img.Bounds().Dx() != 160 {
				t.Fatal("invalid JPEG", e)
			}
			break
		}
		if time.Now().After(until) {
			t.Fatal("no screen JPEG", snapshot)
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Independent authenticated stream UDP must route real Opus to call output.
	enc, err := opus.NewEncoder(sampleRate, channels, opus.AppAudio)
	if err != nil {
		t.Fatal(err)
	}
	pcm := make([]int16, frameLen)
	for i := range pcm {
		pcm[i] = int16((i%100)*40 - 2000)
	}
	data := make([]byte, maxOpusBytes)
	size, err := enc.Encode(pcm, data)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		h := make([]byte, 12)
		h[0] = 0x80
		h[1] = 120
		binary.BigEndian.PutUint16(h[2:], uint16(i))
		binary.BigEndian.PutUint32(h[8:], 43)
		nonce := make([]byte, 24)
		binary.LittleEndian.PutUint32(nonce, nonceID)
		nonceID++
		wire := aead.Seal(h, nonce, data[:size], h)
		wire = append(wire, nonce[:4]...)
		if _, err = udp.WriteToUDP(wire, addr); err != nil {
			t.Fatal(err)
		}
	}
	until = time.Now().Add(time.Second)
	for screen.Snapshot().AudioPackets < 6 {
		if time.Now().After(until) {
			t.Fatal("stream Opus not routed", screen.Snapshot())
		}
		time.Sleep(time.Millisecond)
	}
	output := newReceiver(slog.Default(), func(snowflake.ID, bool) {})
	defer output.Close()
	engine := &Engine{audio: &audio{rx: output}}
	engine.SetScreenAudio(screen)
	mixed := make([]int16, frameLen)
	output.read(mixed)
	if rms(mixed) == 0 || screen.Snapshot().AudioDecodedAgeMS < 0 {
		t.Fatal("stream audio not decoded/mixed")
	}
	// Terminal loss must reap the decoder without a Close action from the UI.
	screen.video.mu.Lock()
	decoder := screen.video.decoder
	screen.video.mu.Unlock()
	g.Close()
	_ = screen.conn.UDP().Close()
	select {
	case <-decoder.done:
	case <-time.After(2 * time.Second):
		t.Fatal("decoder survived stream loss")
	}
	if screen.Snapshot().Error == "" {
		t.Fatal("terminal stream error lost")
	}
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); screen.Close() }()
	}
	wg.Wait()
	if screen.Snapshot().UserID != "" || screen.Snapshot().Image != "" {
		t.Fatal("screen retained after close")
	}
}
