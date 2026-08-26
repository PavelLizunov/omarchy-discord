package voice

import (
	"log/slog"
	"math"
	"sync/atomic"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/hraban/opus"
	"github.com/jfreymuth/pulse"
)

const (
	sampleRate   = 48000
	channels     = 2
	frameSamples = 960                     // per channel: 20 ms at 48 kHz
	frameLen     = frameSamples * channels // int16 values per frame
	frameBytes   = frameLen * 2
	bitrate      = 64000
	holdFrames   = 15 // ~300 ms of gate hold
	maxOpusBytes = 1400
)

// gateRMS is the capture gate threshold on the int16 scale (≈ -36 dBFS; a
// USB webcam mic idles at RMS 80–330 in a quiet room, speech runs well over
// 1000). ponytail: fixed threshold, no adaptive noise floor — tune here if
// mics with a low default gain get cut.
var gateRMS = 500.0

// audioWatchInterval is how often watch polls the streams for a lost server.
var audioWatchInterval = time.Second

// audio is one call's Pulse client with its record and playback streams.
type audio struct {
	client *pulse.Client
	rec    *pulse.RecordStream
	play   *pulse.PlaybackStream
	tx     *capture
	rx     *receiver

	done       chan struct{} // closed by close; stops watch
	serverLost func() bool   // either stream marked closed by a lost server
}

func openAudio(log *slog.Logger, speaking func(snowflake.ID, bool)) (*audio, error) {
	client, err := pulse.NewClient(pulse.ClientApplicationName("omarchy-discord"))
	if err != nil {
		return nil, err
	}
	a := &audio{client: client, done: make(chan struct{})}
	a.serverLost = func() bool { return a.rec.Closed() || a.play.Closed() }
	if a.tx, err = newCapture(log); err != nil {
		client.Close()
		return nil, err
	}
	a.rx = newReceiver(log, speaking)
	a.rec, err = client.NewRecord(pulse.Int16Writer(a.tx.write),
		pulse.RecordStereo, pulse.RecordSampleRate(sampleRate),
		pulse.RecordBufferFragmentSize(frameBytes), pulse.RecordMediaName("Voice call"))
	if err != nil {
		client.Close()
		return nil, err
	}
	a.play, err = client.NewPlayback(pulse.Int16Reader(a.rx.read),
		pulse.PlaybackStereo, pulse.PlaybackSampleRate(sampleRate),
		pulse.PlaybackLatency(0.04), pulse.PlaybackMediaName("Voice call"))
	if err != nil {
		a.rec.Close()
		client.Close()
		return nil, err
	}
	a.rec.Start()
	a.play.Start()
	return a, nil
}

// close stops both streams and the client. A lost Pulse server marks the
// streams closed already; Close is then a no-op.
func (a *audio) close() {
	close(a.done)
	if a.client != nil { // nil in the offline tests
		a.rec.Stop()
		a.rec.Close()
		a.play.Stop()
		a.play.Close()
		a.client.Close()
	}
	a.rx.Close()
}

// watch surfaces a lost Pulse server (pipewire restart) during a call: the
// streams flip to closed and simply stop calling back, so nothing else
// would notice. lost is called at most once, then watch returns.
func (a *audio) watch(every time.Duration, lost func()) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-a.done:
			return
		case <-t.C:
			if a.serverLost() {
				lost()
				return
			}
		}
	}
}

// capture turns 20 ms PCM fragments from Pulse into Opus frames for disgo's
// AudioSender. Pulse's reader goroutine is the capture goroutine: it gates,
// encodes and drops a frame into frames; ProvideOpusFrame pulls one per tick.
type capture struct {
	log    *slog.Logger
	enc    *opus.Encoder
	muted  atomic.Bool
	frames chan []byte // nil element: gated (silence) frame

	buf  []int16 // accumulating frame
	n    int
	hold int
}

func newCapture(log *slog.Logger) (*capture, error) {
	enc, err := opus.NewEncoder(sampleRate, channels, opus.AppVoIP)
	if err != nil {
		return nil, err
	}
	if err := enc.SetBitrate(bitrate); err != nil {
		return nil, err
	}
	return &capture{log: log, enc: enc, frames: make(chan []byte, 4), buf: make([]int16, frameLen)}, nil
}

// write is the pulse.Int16Writer callback. Muted drops the PCM, a partly
// filled frame included, so nothing captured before or during the mute is
// completed and sent after it.
func (c *capture) write(p []int16) (int, error) {
	total := len(p)
	if c.muted.Load() {
		c.n, c.hold = 0, 0
		return total, nil
	}
	for len(p) > 0 {
		k := copy(c.buf[c.n:], p)
		c.n += k
		p = p[k:]
		if c.n == frameLen {
			c.frame(c.buf)
			c.n = 0
		}
	}
	return total, nil
}

// setMuted flips the flag and drops the frames already encoded: what was
// said before the flip must not leave after it. Draining before the store
// makes the unmute airtight (a frame mid-encode at mute time lands after
// the mute's drain and is caught by the unmute's).
func (c *capture) setMuted(m bool) {
	for {
		select {
		case <-c.frames:
		default:
			c.muted.Store(m)
			return
		}
	}
}

// frame gates and encodes one 20 ms frame.
func (c *capture) frame(pcm []int16) {
	loud := rms(pcm) >= gateRMS
	if loud {
		c.hold = holdFrames
	}
	open := loud || c.hold > 0
	if !loud && c.hold > 0 {
		c.hold--
	}
	var out []byte
	if open {
		data := make([]byte, maxOpusBytes)
		n, err := c.enc.Encode(pcm, data)
		if err != nil {
			c.log.Warn("voice: opus encode", "err", err)
			return
		}
		out = data[:n]
	}
	// Sender behind (or not pulling at all while alone in the channel):
	// drop the oldest, so the queue is always the last ≤80 ms and the gate's
	// nil frames flush it once speech stops. Nothing stale can go out later.
	for {
		select {
		case c.frames <- out:
			return
		default:
			select {
			case <-c.frames:
			default:
			}
		}
	}
}

// ProvideOpusFrame implements disgo's OpusFrameProvider. nil, nil means
// silence: disgo sends the silence burst and Speaking itself.
func (c *capture) ProvideOpusFrame() ([]byte, error) {
	if c.muted.Load() {
		return nil, nil
	}
	select {
	case f := <-c.frames:
		return f, nil
	case <-time.After(2 * 20 * time.Millisecond):
		return nil, nil
	}
}

// Close implements OpusFrameProvider; the streams are owned by audio.
func (c *capture) Close() {}

func rms(pcm []int16) float64 {
	var sum float64
	for _, s := range pcm {
		f := float64(s)
		sum += f * f
	}
	return math.Sqrt(sum / float64(len(pcm)))
}
