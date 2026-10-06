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
	frameSamples = 960
	frameLen     = frameSamples * channels
	frameBytes   = frameLen * 2
	bitrate      = 64000
	holdFrames   = 15
	maxOpusBytes = 1400
)

var gateRMS = 500.0

var audioWatchInterval = time.Second

type audio struct {
	client *pulse.Client
	rec    *pulse.RecordStream
	play   *pulse.PlaybackStream
	tx     *capture
	rx     *receiver

	done       chan struct{}
	serverLost func() bool
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
	log    *slog.Logger
	enc    *opus.Encoder
	muted  atomic.Bool
	frames chan []byte

	buf  []int16
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

func (c *capture) Close() {}

func rms(pcm []int16) float64 {
	var sum float64
	for _, s := range pcm {
		f := float64(s)
		sum += f * f
	}
	return math.Sqrt(sum / float64(len(pcm)))
}
