package voice

import (
	"bytes"
	"errors"
	"math"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jfreymuth/pulse/proto"
	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
)

// Diagnostics describes observations, never remote hearing or physical audibility.
type Diagnostics struct {
	Active          bool   `json:"active"`
	InputLevel      int64  `json:"input_level"`
	InputAgeMS      int64  `json:"input_age_ms"`
	SentAgeMS       int64  `json:"sent_age_ms"`
	ReceivedAgeMS   int64  `json:"received_age_ms"`
	DecodedAgeMS    int64  `json:"decoded_age_ms"`
	OutputAgeMS     int64  `json:"output_age_ms"`
	SendErrors      uint64 `json:"send_errors"`
	ReceiveErrors   uint64 `json:"receive_errors"`
	DecodeErrors    uint64 `json:"decode_errors"`
	EncryptionReady bool   `json:"encryption_ready"`
	InputDevice     string `json:"input_device"`
	OutputDevice    string `json:"output_device"`
	InputBlocked    *bool  `json:"input_blocked"`
	OutputBlocked   *bool  `json:"output_blocked"`
	DeviceError     string `json:"device_error"`
}

type audioMetrics struct {
	inputLevel    atomic.Int64
	testUntil     atomic.Int64
	inputAt       atomic.Int64
	sentAt        atomic.Int64
	receivedAt    atomic.Int64
	decodedAt     atomic.Int64
	outputAt      atomic.Int64
	sendErrors    atomic.Uint64
	receiveErrors atomic.Uint64
	decodeErrors  atomic.Uint64
}

func age(now int64, then int64) int64 {
	if then == 0 {
		return -1
	}
	return max(0, now-then)
}

func (m *audioMetrics) snapshot() Diagnostics {
	now := time.Now().UnixMilli()
	return Diagnostics{Active: true, InputLevel: m.inputLevel.Load(), InputAgeMS: age(now, m.inputAt.Load()),
		SentAgeMS: age(now, m.sentAt.Load()), ReceivedAgeMS: age(now, m.receivedAt.Load()),
		DecodedAgeMS: age(now, m.decodedAt.Load()), OutputAgeMS: age(now, m.outputAt.Load()),
		SendErrors: m.sendErrors.Load(), ReceiveErrors: m.receiveErrors.Load(), DecodeErrors: m.decodeErrors.Load()}
}

// Wrap the actual encrypted UDP write, not merely the frame provider.
type observedUDP struct {
	dvoice.UDPConn
	metrics *audioMetrics
}

func (u observedUDP) Write(p []byte) (int, error) {
	n, err := u.UDPConn.Write(p)
	if err != nil {
		u.metrics.sendErrors.Add(1)
	} else if n == len(p) && len(p) > 0 && !bytes.Equal(p, silenceFrame) {
		u.metrics.sentAt.Store(time.Now().UnixMilli())
	}
	return n, err
}

func (e *Engine) Diagnostics() Diagnostics {
	e.opMu.Lock()
	defer e.opMu.Unlock()
	e.mu.Lock()
	a, c, st := e.audio, e.conn, e.st
	e.mu.Unlock()
	if a == nil || st.Status != StatusConnected {
		return Diagnostics{}
	}
	d := a.metrics.snapshot()
	if c != nil {
		d.EncryptionReady = c.DAVE().Ready()
	}
	if a.client != nil {
		a.devices(&d)
	}
	return d
}

func zeroVolume(v proto.ChannelVolumes) bool {
	for _, x := range v {
		if x > 0 {
			return false
		}
	}
	return len(v) > 0
}

func (a *audio) devices(d *Diagnostics) {
	var outputs proto.GetSourceOutputInfoListReply
	if err := a.client.RawRequest(&proto.GetSourceOutputInfoList{}, &outputs); err != nil {
		d.DeviceError = "Input device unavailable"
	} else {
		for _, s := range outputs {
			if strings.TrimRight(string(s.Properties["application.process.id"]), "\x00") != strconv.Itoa(os.Getpid()) || strings.TrimRight(string(s.Properties["media.name"]), "\x00") != "Voice call" {
				continue
			}
			var info proto.GetSourceInfoReply
			if err := a.client.RawRequest(&proto.GetSourceInfo{SourceIndex: s.SourceIndex}, &info); err != nil {
				d.DeviceError = "Input device unavailable"
				break
			}
			d.InputDevice = info.Device
			blocked := s.Muted || s.Corked || info.Mute || zeroVolume(s.ChannelVolumes) || zeroVolume(info.ChannelVolumes)
			d.InputBlocked = &blocked
			break
		}
	}
	var stream proto.GetSinkInputInfoReply
	if err := a.client.RawRequest(&proto.GetSinkInputInfo{SinkInputIndex: a.play.StreamInputIndex()}, &stream); err != nil {
		d.DeviceError = "Output device unavailable"
		return
	}
	var sink proto.GetSinkInfoReply
	if err := a.client.RawRequest(&proto.GetSinkInfo{SinkIndex: stream.SinkIndex}, &sink); err != nil {
		d.DeviceError = "Output device unavailable"
		return
	}
	d.OutputDevice = sink.Device
	blocked := stream.Muted || stream.Corked || sink.Mute || zeroVolume(stream.ChannelVolumes) || zeroVolume(sink.ChannelVolumes)
	d.OutputBlocked = &blocked
}

func (e *Engine) TestOutput() error {
	e.opMu.Lock()
	defer e.opMu.Unlock()
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.audio == nil || e.st.Status != StatusConnected {
		return errors.New("join a voice room before testing its output")
	}
	// One bounded local-only tone; repeated clicks do not stack or extend it.
	if !e.audio.rx.testSamples.CompareAndSwap(0, sampleRate*channels/2) {
		return errors.New("output test already playing")
	}
	e.audio.metrics.testUntil.Store(time.Now().Add(time.Second).UnixMilli())
	for {
		select {
		case <-e.audio.tx.frames:
		default:
			return nil
		}
	}

}

func testSample(remaining int64) int16 {
	elapsed := sampleRate*channels/2 - remaining
	t := float64(elapsed/channels) / sampleRate
	envelope := min(1.0, t*100, (0.5-t)*100)
	return int16(1800 * max(0, envelope) * math.Sin(2*math.Pi*440*t))
}
