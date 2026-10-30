package voice

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/disgoorg/godave"
	dave "github.com/mattcalayo/omarchy-discord/backend/internal/dave"
	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
)

type capturedFrame struct {
	data []byte
	pts  uint64
}

// Capture owns only the user-approved portal session and its encoder process.
// No network operation occurs here. Close interrupts selection as well as capture.
type Capture struct {
	cancel       context.CancelFunc
	cmd          *exec.Cmd
	frames       chan capturedFrame
	done         chan struct{}
	mu           sync.Mutex
	err          error
	lastKeyframe time.Time
}

func StartCapture(ctx context.Context, height, fps int) (*Capture, error) {
	if (height != 360 && height != 720 && height != 1080) || (fps != 15 && fps != 30) {
		return nil, errors.New("unsupported screen quality")
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	helper := filepath.Join(filepath.Dir(executable), "capture-screen.py")
	if _, err = os.Stat(helper); err != nil {
		return nil, errors.New("screen capture helper is not installed")
	}
	run, cancel := context.WithCancel(ctx)
	c := &Capture{cancel: cancel, frames: make(chan capturedFrame, 1), done: make(chan struct{})}
	c.cmd = exec.CommandContext(run, "/usr/bin/python3", helper, "--framed", "--height", fmt.Sprint(height), "--fps", fmt.Sprint(fps))
	c.cmd.Cancel = func() error { return c.cmd.Process.Signal(syscall.SIGTERM) }
	c.cmd.WaitDelay = 3 * time.Second
	out, err := c.cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err = c.cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	go func() {
		select {
		case <-run.Done():
			_ = out.Close()
		case <-c.done:
		}
	}()
	go func() {
		defer close(c.done)
		defer close(c.frames)
		var readErr error
		for {
			var header [12]byte
			if _, readErr = io.ReadFull(out, header[:]); readErr != nil {
				break
			}
			size := binary.BigEndian.Uint32(header[:4])
			if size == 0 || size > cameraFrameLimit {
				readErr = errors.New("invalid encoded screen frame size")
				break
			}
			frame := capturedFrame{data: make([]byte, size), pts: binary.BigEndian.Uint64(header[4:])}
			if _, readErr = io.ReadFull(out, frame.data); readErr != nil {
				break
			}
			select {
			case c.frames <- frame:
			case <-run.Done():
				readErr = run.Err()
			}
			if readErr != nil {
				break
			}
		}
		cancel()
		_ = c.cmd.Wait()
		c.mu.Lock()
		c.err = readErr
		c.mu.Unlock()
	}()
	return c, nil
}
func (c *Capture) Next(ctx context.Context) (capturedFrame, error) {
	select {
	case <-ctx.Done():
		return capturedFrame{}, ctx.Err()
	case f, ok := <-c.frames:
		if ok {
			return f, nil
		}
		c.mu.Lock()
		err := c.err
		c.mu.Unlock()
		if err == nil {
			err = io.EOF
		}
		return capturedFrame{}, err
	}
}
func (c *Capture) Keyframe() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.lastKeyframe) < time.Second {
		return
	}
	c.lastKeyframe = time.Now()
	_ = c.cmd.Process.Signal(syscall.SIGUSR1)
}
func (c *Capture) Close() { c.cancel(); <-c.done }

type videoCryptor interface {
	godave.Session
	AssignH264(uint32)
	EncryptVideo(uint32, []byte) ([]byte, error)
}
type videoTransport interface {
	WriteVideoPacket(*rtp.Packet) error
	ReadVideoFeedback() ([]rtcp.Packet, error)
	SendVideoReport(uint32, uint32, uint32, uint32)
}

type Broadcast struct {
	cancel       context.CancelFunc
	done         chan struct{}
	conn         dvoice.Conn
	capture      *Capture
	mu           sync.Mutex
	err          string
	packets      atomic.Uint64
	frames       atomic.Uint64
	cache        []*rtp.Packet
	rtxSequence  uint16
	lastFeedback time.Time
	closed       atomic.Bool
}
type BroadcastSnapshot struct {
	Status  string `json:"status"`
	Error   string `json:"error"`
	Frames  uint64 `json:"frames"`
	Packets uint64 `json:"packets"`
}

func (b *Broadcast) Snapshot() BroadcastSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	status := "sharing"
	if b.frames.Load() == 0 {
		status = "waiting"
	}
	if b.closed.Load() {
		status = "stopped"
	}
	if b.err != "" {
		status = "error"
	}
	return BroadcastSnapshot{status, b.err, b.frames.Load(), b.packets.Load()}
}
func (b *Broadcast) fail(message string) { b.mu.Lock(); b.err = message; b.mu.Unlock(); b.cancel() }

// OpenBroadcast uses only server-assigned SSRCs on a separate Go Live transport.
func OpenBroadcast(ctx context.Context, state dvoice.State, capture *Capture, first capturedFrame, height, fps int) (*Broadcast, error) {
	run, cancel := context.WithCancel(ctx)
	b := &Broadcast{cancel: cancel, done: make(chan struct{}), capture: capture}
	ready := make(chan dvoice.GatewayMessageDataReady, 1)
	state.VideoSend = true
	c := dvoice.NewConn(state.GuildID, state.UserID, nil, func() {}, dvoice.WithConnDaveSessionCreateFunc(dave.NewSession), dvoice.WithConnLogger(slog.Default()), dvoice.WithConnGatewayConfigOpts(dvoice.WithGatewayCloseObserver(func(error) { b.fail("Screen connection lost") })), dvoice.WithConnEventHandlerFunc(func(_ dvoice.Gateway, _ dvoice.Opcode, _ int, data dvoice.GatewayMessageData) {
		if r, ok := data.(dvoice.GatewayMessageDataReady); ok {
			select {
			case ready <- r:
			default:
			}
		}
	}))
	b.conn = c
	opened := false
	defer func() {
		if !opened {
			cancel()
			closeCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
			c.Close(closeCtx)
			stop()
		}
	}()
	opener := c.(interface {
		OpenMedia(context.Context, dvoice.State) error
	})
	handshake, stop := context.WithTimeout(run, 15*time.Second)
	defer stop()
	if err := opener.OpenMedia(handshake, state); err != nil {
		return nil, err
	}
	var allocation dvoice.GatewayMessageDataReady
	select {
	case allocation = <-ready:
	case <-handshake.Done():
		return nil, handshake.Err()
	}
	var stream dvoice.VideoStream
	for _, s := range allocation.Streams {
		if s.RID == "100" && s.SSRC != 0 && s.RTXSSRC != 0 {
			stream = s
			break
		}
	}
	if stream.SSRC == 0 {
		return nil, errors.New("Discord did not allocate a screen source")
	}
	ds, ok := c.DAVE().(videoCryptor)
	if !ok {
		return nil, errors.New("video encryption unavailable")
	}
	transport, ok := c.UDP().(videoTransport)
	if !ok {
		return nil, errors.New("video transport unavailable")
	}
	ds.AssignH264(stream.SSRC)
	stream.Type = "video"
	stream.RID = "100"
	stream.Quality = 100
	stream.Active = true
	stream.MaxBitrate = 2500000
	stream.MaxFramerate = fps
	stream.MaxResolution = &dvoice.VideoResolution{Type: "fixed", Width: height * 16 / 9, Height: height}
	if err := c.Gateway().Send(handshake, dvoice.OpcodeVideo, dvoice.GatewayMessageDataVideo{AudioSSRC: allocation.SSRC, VideoSSRC: stream.SSRC, RTXSSRC: stream.RTXSSRC, Streams: []dvoice.VideoStream{stream}}); err != nil {
		return nil, err
	}
	if err := c.SetSpeaking(handshake, 0); err != nil {
		return nil, err
	}
	opened = true
	go b.send(run, ds, transport, stream, first)
	return b, nil
}

func (b *Broadcast) send(ctx context.Context, ds videoCryptor, transport videoTransport, stream dvoice.VideoStream, first capturedFrame) {
	defer close(b.done)
	defer b.closed.Store(true)
	defer b.capture.Close()
	defer func() {
		bound, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		b.conn.Close(bound)
	}()
	feedbackDone := make(chan struct{})
	go func() { defer close(feedbackDone); b.feedback(ctx, transport, stream) }()
	defer func() { b.cancel(); _ = b.conn.UDP().Close(); <-feedbackDone }()
	sequence := rtp.NewRandomSequencer()
	payloader := &codecs.H264Payloader{DisableStapA: true}
	frame := first
	var base uint64
	var baseSet bool
	var octets uint32
	var lastReport time.Time
	waiting := true
	needKeyframe := true
	for ctx.Err() == nil {
		if ds.Ready() {
			if waiting {
				b.capture.Keyframe()
				waiting = false
				needKeyframe = true
			}
			if needKeyframe && !h264IDR(frame.data) {
				var err error
				frame, err = b.capture.Next(ctx)
				if err != nil {
					if ctx.Err() == nil {
						b.fail("Screen capture ended")
					}
					return
				}
				continue
			}
			needKeyframe = false
			// Normalize before authentication: Pion omits AUD/filler and reconstructs
			// four-byte Annex-B prefixes. These must not change after DAVE signs them.
			plain, err := normalizeH264(frame.data)
			if err != nil {
				b.fail("Invalid captured H.264 frame")
				return
			}
			encrypted, err := ds.EncryptVideo(stream.SSRC, plain)
			if err != nil {
				b.fail("Screen encryption failed")
				return
			}
			payloads := payloader.Payload(1150, encrypted)
			if !baseSet {
				baseSet = true
				base = frame.pts
			}
			timestamp := uint32((frame.pts - base) / 1000 * 90 / 1000)
			for i, payload := range payloads {
				if ctx.Err() != nil {
					return
				}
				packet := &rtp.Packet{Header: rtp.Header{Version: 2, PayloadType: dvoice.H264PayloadType, SequenceNumber: sequence.NextSequenceNumber(), Timestamp: timestamp, SSRC: stream.SSRC, Marker: i == len(payloads)-1}, Payload: payload}
				if err = transport.WriteVideoPacket(packet); err != nil {
					b.fail("Screen transmission failed")
					return
				}
				b.packets.Add(1)
				octets += uint32(len(payload))
				b.mu.Lock()
				if len(b.cache) >= 512 {
					b.cache = b.cache[1:]
				}
				b.cache = append(b.cache, packet)
				b.mu.Unlock()
			}
			b.frames.Add(1)
			if time.Since(lastReport) >= time.Second {
				transport.SendVideoReport(stream.SSRC, timestamp, uint32(b.packets.Load()), octets)
				lastReport = time.Now()
			}
		} else {
			waiting = true
		}
		var err error
		frame, err = b.capture.Next(ctx)
		if err != nil {
			if ctx.Err() == nil {
				b.fail("Screen capture ended")
			}
			return
		}
	}
}
func (b *Broadcast) feedback(ctx context.Context, transport videoTransport, stream dvoice.VideoStream) {
	for ctx.Err() == nil {
		_ = b.conn.UDP().SetReadDeadline(time.Now().Add(time.Second))
		packets, err := transport.ReadVideoFeedback()
		if err != nil {
			var networkError net.Error
			if errors.As(err, &networkError) && networkError.Timeout() {
				continue
			}
			if ctx.Err() == nil {
				b.fail("Screen feedback connection lost")
			}
			return
		}
		for _, p := range packets {
			switch f := p.(type) {
			case *rtcp.PictureLossIndication:
				if f.MediaSSRC == stream.SSRC {
					b.capture.Keyframe()
				}
			case *rtcp.FullIntraRequest:
				for _, request := range f.FIR {
					if request.SSRC == stream.SSRC {
						b.capture.Keyframe()
						break
					}
				}
			case *rtcp.TransportLayerNack:
				if f.MediaSSRC != stream.SSRC {
					continue
				}
				b.mu.Lock()
				if time.Since(b.lastFeedback) < 20*time.Millisecond {
					b.mu.Unlock()
					continue
				}
				b.lastFeedback = time.Now()
				var resend []*rtp.Packet
				for _, pair := range f.Nacks[:min(len(f.Nacks), 32)] {
					for _, seq := range pair.PacketList() {
						for _, original := range b.cache {
							if original.SequenceNumber == seq && len(resend) < 64 {
								r := &rtp.Packet{Header: original.Header, Payload: make([]byte, 2+len(original.Payload))}
								r.SSRC = stream.RTXSSRC
								r.PayloadType = dvoice.H264PayloadType + 1
								r.SequenceNumber = b.rtxSequence
								b.rtxSequence++
								binary.BigEndian.PutUint16(r.Payload, seq)
								copy(r.Payload[2:], original.Payload)
								resend = append(resend, r)
								break
							}
						}
					}
				}
				b.mu.Unlock()
				for _, packet := range resend {
					if ctx.Err() != nil {
						return
					}
					_ = transport.WriteVideoPacket(packet)
				}
			}
		}
	}
}
func normalizeH264(frame []byte) ([]byte, error) {
	payloader := &codecs.H264Payloader{DisableStapA: true}
	parser := &codecs.H264Packet{}
	var out []byte
	for _, payload := range payloader.Payload(1150, frame) {
		nal, err := parser.Unmarshal(payload)
		if err != nil {
			return nil, err
		}
		out = append(out, nal...)
	}
	if len(out) == 0 {
		return nil, errors.New("empty H.264 frame")
	}
	return out, nil
}

func (b *Broadcast) Close() { b.cancel(); _ = b.conn.UDP().Close(); <-b.done }
