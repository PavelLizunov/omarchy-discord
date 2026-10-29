package voice

import (
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/hraban/opus"
	"github.com/jfreymuth/pulse"
)

const (
	sampleRate   = 48000
	channels     = 2
	frameSamples = 960
	frameLen     = frameSamples * channels
	frameBytes   = frameLen * 2
	bitrate      = 64000
	maxOpusBytes = 1400
)

var audioWatchInterval = time.Second

type audio struct {
	client  *pulse.Client
	rec     *pulse.RecordStream
	play    *pulse.PlaybackStream
	tx      *capture
	rx      *receiver
	metrics *audioMetrics

	done       chan struct{}
	serverLost func() bool
}

func openAudio(log *slog.Logger, speaking func(snowflake.ID, bool)) (*audio, error) {
	client, err := pulse.NewClient(pulse.ClientApplicationName("omarchy-discord"))
	if err != nil {
		return nil, err
	}
	a := &audio{client: client, done: make(chan struct{}), metrics: &audioMetrics{}}
	a.serverLost = func() bool { return a.rec.Closed() || a.play.Closed() }
	if a.tx, err = newCapture(log); err != nil {
		client.Close()
		return nil, err
	}
	a.tx.metrics = a.metrics
	a.rx = newReceiver(log, speaking)
	a.rx.metrics = a.metrics
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

func (a *audio) close() {
	close(a.done)
	if a.client != nil {
		a.rec.Stop()
		a.rec.Close()
		a.play.Stop()
		a.play.Close()
		a.client.Close()
	}
	a.rx.Close()
}

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

type capture struct {
	log     *slog.Logger
	metrics *audioMetrics
	enc     *opus.Encoder
	muted   atomic.Bool
	frames  chan []byte

	mu  sync.Mutex
	buf []int16
	n   int
}

func newCapture(log *slog.Logger) (*capture, error) {
	enc, err := opus.NewEncoder(sampleRate, channels, opus.AppVoIP)
	if err != nil {
		return nil, err
	}
	if err := enc.SetBitrate(bitrate); err != nil {
		return nil, err
	}
	// Let libopus classify silence instead of cutting quiet speech before encoding.
	if err := enc.SetDTX(true); err != nil {
		return nil, err
	}
	return &capture{log: log, enc: enc, frames: make(chan []byte, 4), buf: make([]int16, frameLen)}, nil
}

func (c *capture) write(p []int16) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	total := len(p)
	if c.metrics != nil && total > 0 {
		c.metrics.inputLevel.Store(int64(min(100.0, rms(p)/32768*100)))
		c.metrics.inputAt.Store(time.Now().UnixMilli())
	}
	if c.muted.Load() || (c.metrics != nil && c.metrics.testUntil.Load() > time.Now().UnixMilli()) {
		c.n = 0
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

func (c *capture) setMuted(m bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.muted.Store(m)
	c.n = 0
	for {
		select {
		case <-c.frames:
		default:
			return
		}
	}
}

func (c *capture) frame(pcm []int16) {
	data := make([]byte, maxOpusBytes)
	n, err := c.enc.Encode(pcm, data)
	if err != nil {
		c.log.Warn("voice: opus encode", "err", err)
		return
	}
	var out []byte
	// libopus recommends omitting packets of at most two bytes during DTX.
	// Keep its comfort-noise updates and all encoded speech, regardless of RMS.
	if n > 2 {
		out = data[:n]
	}
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

func (c *capture) ProvideOpusFrame() ([]byte, error) {
	if c.muted.Load() || (c.metrics != nil && c.metrics.testUntil.Load() > time.Now().UnixMilli()) {
		return nil, nil
	}
	select {
	case f := <-c.frames:
		if c.muted.Load() || (c.metrics != nil && c.metrics.testUntil.Load() > time.Now().UnixMilli()) {
			return nil, nil
		}
		return f, nil
	case <-time.After(2 * 20 * time.Millisecond):
		return nil, nil
	}
}

func (c *capture) Close() {}

func rms(pcm []int16) float64 {
	var sum float64
	for _, s := range pcm {
		f := float64(s)
		sum += f * f
	}
	return math.Sqrt(sum / float64(len(pcm)))
}
