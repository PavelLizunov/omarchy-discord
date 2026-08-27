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

	dvoice "github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/snowflake/v2"
	"github.com/hraban/opus"
)

const (
	queueCap        = 6 // frames per user; ponytail: fixed depth, no drift compensation
	primeFrames     = 2 // buffered before a user's stream starts playing
	maxPLC          = 5 // consecutive concealed frames before a stream is considered ended
	silenceToStop   = 5 // silence frames (F8 FF FE) that end "speaking"
	speakingTimeout = 250 * time.Millisecond
)

var silenceFrame = []byte{0xF8, 0xFF, 0xFE}

// receiver is disgo's OpusFrameReceiver and the playback mixer: per-user
// decoder + jitter queue, int32 sum saturated to int16, one Pulse stream.
// Pulse's playback goroutine is the playback goroutine (read).
type receiver struct {
	log      *slog.Logger
	speaking func(snowflake.ID, bool)
	deafened atomic.Bool

	mu      sync.Mutex
	closed  bool
	users   map[snowflake.ID]*user
	mix     []int32
	frame   []int16
	pending []int16 // unread tail of frame
}

type user struct {
	dec      *opus.Decoder
	pcm      []int16
	q        []frame // sorted by RTP sequence
	next     uint16  // sequence expected next, valid while primed
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

// ReceiveOpusFrame runs on disgo's receiver goroutine.
func (r *receiver) ReceiveOpusFrame(userID snowflake.ID, p *dvoice.Packet) error {
	if userID == 0 || len(p.Opus) == 0 {
		return nil // SSRC not yet announced by a Speaking op
	}
	silent := bytes.Equal(p.Opus, silenceFrame)
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
		u.push(frame{seq: p.Sequence, opus: bytes.Clone(p.Opus)}) // disgo reuses its read buffer
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
	u := &user{dec: dec, pcm: make([]int16, frameLen)}
	u.timer = time.AfterFunc(speakingTimeout, func() { r.timeout(id) })
	r.users[id] = u
	return u, nil
}

// timeout clears speaking after 250 ms without packets.
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

// CleanupUser drops a participant who left the channel.
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

// Close clears speaking for every tracked user (mandatory on leave and
// disconnect) and stops mixing. Idempotent.
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

// read is the pulse.Int16Reader callback: always fills out.
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
	return filled, nil
}

// mixLocked renders one 20 ms frame from every user into r.frame.
func (r *receiver) mixLocked() {
	clear(r.mix)
	for _, u := range r.users {
		if !u.pop(r.log) {
			continue
		}
		for i, s := range u.pcm {
			r.mix[i] += int32(s)
		}
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

// push inserts by sequence, dropping late and duplicate frames; a full queue
// drops its oldest frame to keep latency bounded.
func (u *user) push(f frame) {
	if u.primed && int16(f.seq-u.next) < 0 {
		return // late
	}
	i := len(u.q)
	for i > 0 {
		d := int16(f.seq - u.q[i-1].seq)
		if d == 0 {
			return // duplicate
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

// take returns the next frame to render: (frame, false) to decode,
// (nil, true) for packet-loss concealment, (nil, false) for nothing.
func (u *user) take() (f *frame, plc bool) {
	if !u.primed {
		if len(u.q) < primeFrames {
			return nil, false
		}
		u.primed, u.next, u.plc = true, u.q[0].seq, 0
	}
	if len(u.q) == 0 {
		if u.plc >= maxPLC {
			u.primed = false // stream ended; re-prime on the next burst
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
		u.next = u.q[0].seq // too far ahead: jump
	}
	f = &u.q[0]
	u.q = u.q[1:]
	u.next = f.seq + 1
	u.plc = 0
	return f, false
}

// pop decodes the next frame (or PLC) into u.pcm; false means silence.
func (u *user) pop(log *slog.Logger) bool {
	f, plc := u.take()
	switch {
	case plc:
		if err := u.dec.DecodePLC(u.pcm); err != nil {
			return false
		}
		return true
	case f == nil:
		return false
	}
	n, err := u.dec.Decode(f.opus, u.pcm)
	if err != nil {
		log.Debug("voice: opus decode", "err", err)
		return false
	}
	clear(u.pcm[n*channels:])
	return true
}

// rxDriver replaces disgo's AudioReceiver, which busy-loops while the DAVE
// session is not ready (the whole time we are alone in the channel) and
// nil-derefs when closed before its goroutine starts.
type rxDriver struct {
	log    *slog.Logger
	rx     dvoice.OpusFrameReceiver
	conn   dvoice.Conn
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

func (d *rxDriver) loop(ctx context.Context) {
	pause := func() {
		select {
		case <-ctx.Done():
		case <-time.After(20 * time.Millisecond):
		}
	}
	for ctx.Err() == nil {
		if !d.conn.DAVE().Ready() {
			pause()
			continue
		}
		p, err := d.conn.UDP().ReadPacket()
		if errors.Is(err, net.ErrClosed) {
			return
		}
		if err != nil {
			d.log.Debug("voice: read packet", "err", err)
			pause() // never spin on a dead socket or a decrypt failure
			continue
		}
		if err := d.rx.ReceiveOpusFrame(d.conn.UserIDBySSRC(p.SSRC), p); err != nil {
			d.log.Warn("voice: receive frame", "err", err)
		}
	}
}

func (d *rxDriver) CleanupUser(id snowflake.ID) { d.rx.CleanupUser(id) }

// Close stops the loop; the OpusFrameReceiver is closed by its owner.
func (d *rxDriver) Close() {
	d.mu.Lock()
	cancel := d.cancel
	d.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
