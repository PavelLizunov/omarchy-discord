package voice

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/hraban/opus"
	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
)

const (
	queueCap        = 6
	primeFrames     = 2
	maxPLC          = 5
	silenceToStop   = 5
	speakingTimeout = 250 * time.Millisecond
)

var silenceFrame = []byte{0xF8, 0xFF, 0xFE}

type receiver struct {
	log         *slog.Logger
	speaking    func(snowflake.ID, bool)
	deafened    atomic.Bool
	metrics     *audioMetrics
	testSamples atomic.Int64

	mu      sync.Mutex
	closed  bool
	users   map[snowflake.ID]*user
	levels  map[snowflake.ID]UserAudio
	mix     []int32
	frame   []int16
	pending []int16
	screen  *screenAudio
}

type user struct {
	dec      *opus.Decoder
	pcm      []int16
	decoded  []int16
	pending  []int16
	q        []frame
	ssrc     uint32
	next     uint16
	primed   bool
	plc      int
	speaking bool
	silence  int
	timer    *time.Timer
}

type frame struct {
	seq  uint16
	opus []byte
}

func newReceiver(log *slog.Logger, speaking func(snowflake.ID, bool)) *receiver {
	return &receiver{
		log:      log,
		speaking: speaking,
		users:    map[snowflake.ID]*user{},
		mix:      make([]int32, frameLen),
		frame:    make([]int16, frameLen),
	}
}

func (r *receiver) ReceiveOpusFrame(userID snowflake.ID, p *dvoice.Packet) error {
	if userID == 0 || len(p.Opus) == 0 {
		return nil
	}
	silent := bytes.Equal(p.Opus, silenceFrame)
	if !silent && r.metrics != nil {
		r.metrics.receivedAt.Store(time.Now().UnixMilli())
	}
	var emit *bool
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	u, err := r.userLocked(userID)
	if err != nil {
		r.mu.Unlock()
		return err
	}
	if u.ssrc != p.SSRC {
		// A restarted source has a new sequence space. Old next/PLC state
		// otherwise rejects valid audio as late until sequence wraparound.
		u.ssrc = p.SSRC
		u.q, u.pending = nil, nil
		u.primed, u.plc = false, 0
		if decoder, err := opus.NewDecoder(sampleRate, channels); err == nil {
			u.dec = decoder
		}
	}
	if silent {
		u.silence++
		if u.silence >= silenceToStop && u.speaking {
			u.speaking = false
			emit = new(bool)
		}
	} else {
		u.silence = 0
		if !u.speaking {
			u.speaking = true
			on := true
			emit = &on
		}
	}
	u.timer.Reset(speakingTimeout)
	if !r.deafened.Load() {
		u.push(frame{seq: p.Sequence, opus: bytes.Clone(p.Opus)})
	}
	r.mu.Unlock()
	if emit != nil {
		r.speaking(userID, *emit)
	}
	return nil
}

func (r *receiver) userLocked(id snowflake.ID) (*user, error) {
	if u, ok := r.users[id]; ok {
		return u, nil
	}
	dec, err := opus.NewDecoder(sampleRate, channels)
	if err != nil {
		return nil, err
	}
	u := &user{dec: dec, pcm: make([]int16, frameLen), decoded: make([]int16, sampleRate*channels*120/1000)}
	u.timer = time.AfterFunc(speakingTimeout, func() { r.timeout(id) })
	r.users[id] = u
	return u, nil
}

func (r *receiver) timeout(id snowflake.ID) {
	r.mu.Lock()
	u := r.users[id]
	fire := !r.closed && u != nil && u.speaking
	if fire {
		u.speaking = false
	}
	r.mu.Unlock()
	if fire {
		r.speaking(id, false)
	}
}

func (r *receiver) CleanupUser(id snowflake.ID) {
	r.mu.Lock()
	u := r.users[id]
	delete(r.users, id)
	fire := false
	if u != nil {
		u.timer.Stop()
		fire = u.speaking && !r.closed
	}
	r.mu.Unlock()
	if fire {
		r.speaking(id, false)
	}
}

func (r *receiver) Close() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	var clear []snowflake.ID
	for id, u := range r.users {
		u.timer.Stop()
		if u.speaking {
			clear = append(clear, id)
		}
	}
	r.users = map[snowflake.ID]*user{}
	r.mu.Unlock()
	for _, id := range clear {
		r.speaking(id, false)
	}
}

func (r *receiver) read(out []int16) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	filled := 0
	for filled < len(out) {
		if len(r.pending) == 0 {
			r.mixLocked()
			r.pending = r.frame
		}
		k := copy(out[filled:], r.pending)
		r.pending = r.pending[k:]
		filled += k
	}
	for i := range out {
		if remaining := r.testSamples.Load(); remaining > 0 {
			out[i] = saturate(int32(out[i]) + int32(testSample(remaining)))
			r.testSamples.Add(-1)
		}
	}
	if r.metrics != nil && len(out) > 0 && rms(out) > 0 {
		r.metrics.outputAt.Store(time.Now().UnixMilli())
	}
	return filled, nil
}

func (r *receiver) mixLocked() {
	clear(r.mix)
	for id, u := range r.users {
		if !u.pop(r.log, r.metrics) {
			continue
		}
		level := userAudio(r.levels, id)
		if level.Muted || r.deafened.Load() {
			continue
		}
		for i, s := range u.pcm {
			r.mix[i] += int32(s) * int32(level.Volume) / 100
		}
	}
	if r.screen != nil {
		stream := r.screen.rx
		stream.mu.Lock()
		if !stream.closed {
			stream.mixLocked()
			for id := range stream.users {
				level := userAudio(r.levels, id)
				if !level.Muted && !r.deafened.Load() {
					for i, s := range stream.frame {
						r.mix[i] += int32(s) * int32(level.Volume) / 100 * int32(r.screen.volume.Load()) / 100
					}
				}
			}
		}
		stream.mu.Unlock()
	}
	for i, s := range r.mix {
		r.frame[i] = saturate(s)
	}
}

func saturate(s int32) int16 {
	if s > 32767 {
		return 32767
	}
	if s < -32768 {
		return -32768
	}
	return int16(s)
}

func (u *user) push(f frame) {
	if u.primed && int16(f.seq-u.next) < 0 {
		return
	}
	i := len(u.q)
	for i > 0 {
		d := int16(f.seq - u.q[i-1].seq)
		if d == 0 {
			return
		}
		if d > 0 {
			break
		}
		i--
	}
	u.q = append(u.q, frame{})
	copy(u.q[i+1:], u.q[i:])
	u.q[i] = f
	if len(u.q) > queueCap {
		u.q = u.q[1:]
		if u.primed {
			u.next = u.q[0].seq
		}
	}
}

func (u *user) take() (f *frame, plc bool) {
	if !u.primed {
		if len(u.q) < primeFrames {
			return nil, false
		}
		u.primed, u.next, u.plc = true, u.q[0].seq, 0
	}
	if len(u.q) == 0 {
		if u.plc >= maxPLC {
			u.primed = false
			return nil, false
		}
		u.plc++
		u.next++
		return nil, true
	}
	if gap := int16(u.q[0].seq - u.next); gap > 0 {
		if gap <= queueCap {
			u.plc++
			u.next++
			return nil, true
		}
		u.next = u.q[0].seq
	}
	f = &u.q[0]
	u.q = u.q[1:]
	u.next = f.seq + 1
	u.plc = 0
	return f, false
}

func (u *user) pop(log *slog.Logger, metrics ...*audioMetrics) bool {
	var m *audioMetrics
	if len(metrics) > 0 {
		m = metrics[0]
	}
	// Packet duration and the playback block size are independent. Preserve
	// long packets and combine short ones instead of dropping samples.
	clear(u.pcm)
	filled := 0
	for filled < len(u.pcm) {
		if len(u.pending) > 0 {
			n := copy(u.pcm[filled:], u.pending)
			filled += n
			u.pending = u.pending[n:]
			continue
		}
		f, plc := u.take()
		if f == nil && !plc {
			break
		}
		if len(u.decoded) == 0 {
			u.decoded = make([]int16, sampleRate*channels*120/1000)
		}
		if plc {
			if err := u.dec.DecodePLC(u.decoded[:frameLen:frameLen]); err != nil {
				break
			}
			u.pending = u.decoded[:frameLen]
			continue
		}
		n, err := u.dec.Decode(f.opus, u.decoded)
		if err != nil {
			log.Debug("voice: opus decode", "err", err)
			if m != nil {
				m.decodeErrors.Add(1)
			}
			break
		}
		if m != nil {
			m.decodedAt.Store(time.Now().UnixMilli())
		}
		u.pending = u.decoded[:n*channels]
	}
	return filled > 0
}

type rxDriver struct {
	log    *slog.Logger
	rx     dvoice.OpusFrameReceiver
	conn   dvoice.Conn
	video  *camera
	cancel context.CancelFunc
	mu     sync.Mutex
}

func (d *rxDriver) Open() {
	ctx, cancel := context.WithCancel(context.Background())
	d.mu.Lock()
	d.cancel = cancel
	d.mu.Unlock()
	go d.loop(ctx)
}

func (d *rxDriver) loop(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	pause := func() {
		select {
		case <-ctx.Done():
		case <-time.After(20 * time.Millisecond):
		}
	}
	feedbackDone := make(chan struct{})
	go func() {
		defer close(feedbackDone)
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				videoFeedback(d.video, d.conn.UDP(), now)
			}
		}
	}()
	defer func() { cancel(); <-feedbackDone }()
	for ctx.Err() == nil {
		p, err := d.conn.UDP().ReadPacket()
		if errors.Is(err, net.ErrClosed) {
			return
		}
		if err != nil {
			d.log.Debug("voice: read packet", "err", err)
			if r, ok := d.rx.(*receiver); ok && r.metrics != nil {
				r.metrics.receiveErrors.Add(1)
			}
			// A rejected media packet has already been consumed. Sleeping for
			// 20 ms here delays every other speaker on this shared transport.
			var networkError net.Error
			if errors.As(err, &networkError) {
				pause()
			}
			continue
		}
		if p.Type == dvoice.H264PayloadType {
			if d.video != nil {
				d.video.receive(p, d.conn.DAVE())
			}
			continue
		}
		if err := d.rx.ReceiveOpusFrame(d.conn.UserIDBySSRC(p.SSRC), p); err != nil {
			d.log.Warn("voice: receive frame", "err", err)
		}
	}
}

func (d *rxDriver) CleanupUser(id snowflake.ID) { d.rx.CleanupUser(id) }

func (d *rxDriver) Close() {
	d.mu.Lock()
	cancel := d.cancel
	d.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
