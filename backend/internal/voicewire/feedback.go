package voice

import (
	"encoding/binary"
	"github.com/disgoorg/snowflake/v2"
	"time"
)

// Native Discord assigns transport-wide congestion control to extension 5.
// WebRTC SDP extension numbering is different and must not be used here.
func transportSequence(profile int, extension []byte) (uint16, bool) {
	if profile != 0xbede {
		return 0, false
	}
	for off := 0; off < len(extension); {
		h := extension[off]
		off++
		if h == 0 {
			continue
		}
		id := h >> 4
		if id == 15 {
			return 0, false
		}
		n := int(h&15) + 1
		if off+n > len(extension) {
			return 0, false
		}
		if id == 5 && n == 2 {
			return binary.BigEndian.Uint16(extension[off : off+2]), true
		}
		off += n
	}
	return 0, false
}

type transportFeedback struct {
	base        uint16
	latest      uint16
	arrivals    map[uint16]int64
	last        time.Time
	count       byte
	initialized bool
}

// add batches one bounded transport-cc report per 50ms or full512-packet window. The
// receiver reports authenticated packet arrival, including padding probes,
// before media admission; a probe is never passed to a video decoder.
func (f *transportFeedback) add(sequence uint16, at time.Time, sender uint32) []byte {
	if f.arrivals == nil {
		f.arrivals = make(map[uint16]int64)
		if f.initialized {
			f.latest = f.base
		}
		if !f.initialized {
			f.base = sequence
			f.latest = sequence
			f.last = at
			f.initialized = true
		}
	}
	distance := int(uint16(sequence - f.base))
	if distance >= 512 && distance < 32768 {
		pending := f.report(sender)
		f.base = sequence
		f.latest = sequence
		f.arrivals = map[uint16]int64{sequence: at.UnixMicro()}
		f.last = at
		return pending
	}
	if distance >= 32768 {
		return nil
	} // Late packet already acknowledged.
	f.arrivals[sequence] = at.UnixMicro()
	if distance > int(uint16(f.latest-f.base)) {
		f.latest = sequence
	}
	if at.Sub(f.last) < 50*time.Millisecond {
		return nil
	}
	report := f.report(sender)
	f.base = f.latest + 1
	f.arrivals = nil
	f.last = at
	return report
}
func (f *transportFeedback) report(sender uint32) []byte {
	n := int(uint16(f.latest-f.base)) + 1
	if n < 1 || n > 513 {
		return nil
	}
	var first int64
	for i := 0; i < n; i++ {
		if a, ok := f.arrivals[f.base+uint16(i)]; ok {
			first = a
			break
		}
	}
	if first == 0 {
		return nil
	}
	reference := first / 64000
	previous := reference * 64000
	statuses := make([]byte, n)
	var deltas []byte
	for i := 0; i < n; i++ {
		at, ok := f.arrivals[f.base+uint16(i)]
		if !ok {
			continue
		}
		delta := (at - previous + 125) / 250
		if delta < -32768 || delta > 32767 {
			return nil
		}
		previous += delta * 250
		if delta >= 0 && delta <= 255 {
			statuses[i] = 1
			deltas = append(deltas, byte(delta))
		} else {
			statuses[i] = 2
			deltas = append(deltas, byte(uint16(int16(delta))>>8), byte(delta))
		}
	}
	p := make([]byte, 20)
	p[0] = 0x8f
	p[1] = 205
	binary.BigEndian.PutUint32(p[4:8], sender)
	// A transport-cc report spans a transport, not one media SSRC.
	binary.BigEndian.PutUint16(p[12:14], f.base)
	binary.BigEndian.PutUint16(p[14:16], uint16(n))
	p[16] = byte(reference >> 16)
	p[17] = byte(reference >> 8)
	p[18] = byte(reference)
	p[19] = f.count
	f.count++
	for i := 0; i < n; i += 7 {
		chunk := uint16(0xc000)
		for j := 0; j < 7 && i+j < n; j++ {
			chunk |= uint16(statuses[i+j]) << uint(12-j*2)
		}
		p = append(p, byte(chunk>>8), byte(chunk))
	}
	p = append(p, deltas...)
	for len(p)%4 != 0 {
		p = append(p, 0)
	}
	binary.BigEndian.PutUint16(p[2:4], uint16(len(p)/4-1))
	return p
}
func (u *udpConnImpl) feedbackArrival(p *Packet) {
	if p.Type == H264PayloadType || p.Type == H264PayloadType+1 {
		if u.reports == nil {
			u.reports = make(map[uint32]*receiverReport)
		}
		r := u.reports[p.SSRC]
		if r == nil && len(u.reports) < 32 {
			r = &receiverReport{}
			u.reports[p.SSRC] = r
		}
		if r != nil {
			if report := r.add(p, time.Now(), u.ssrc); report != nil {
				u.sendFeedback(report, "receiver_report_sent")
			}
		}
	}
	seq, ok := transportSequence(p.ExtensionID, p.Extension)
	if !ok {
		return
	}
	u.cameraStage("transport_sequence")
	report := u.feedback.add(seq, time.Now(), u.ssrc)
	if len(report) == 0 {
		return
	}
	u.sendFeedback(report, "feedback_sent")
}
func (u *udpConnImpl) sendFeedback(report []byte, stage string) {
	u.writeMu.Lock()
	defer u.writeMu.Unlock()
	u.connMu.Lock()
	conn := u.conn
	u.connMu.Unlock()
	a, ok := u.encrypter.(*AEADEncrypter)
	if !ok || conn == nil {
		return
	}
	_ = conn.SetWriteDeadline(time.Now().Add(time.Second))
	defer conn.SetWriteDeadline(time.Time{})
	if _, err := conn.Write(a.encryptHeader(report[:8], report[8:])); err != nil {
		u.cameraStage("feedback_error")
	} else {
		u.cameraStage(stage)
	}
}

// RFC3550 receiver state includes authenticated padding in sequence accounting.
type receiverReport struct {
	initialized   bool
	base          uint32
	highest       uint32
	received      uint32
	expectedPrior uint32
	receivedPrior uint32
	last          time.Time
	arrival       int64
	timestamp     uint32
	jitter        float64
}

func (r *receiverReport) add(p *Packet, at time.Time, sender uint32) []byte {
	if !r.initialized {
		r.initialized = true
		r.base = uint32(p.Sequence)
		r.highest = r.base
		r.last = at
	}
	extended := int64(r.highest) + int64(int16(p.Sequence-uint16(r.highest)))
	if extended > int64(r.highest) {
		r.highest = uint32(extended)
	}
	r.received++
	arrival := at.UnixMicro() * 90 / 1000
	if r.arrival != 0 {
		d := (arrival - r.arrival) - int64(int32(p.Timestamp-r.timestamp))
		if d < 0 {
			d = -d
		}
		r.jitter += (float64(d) - r.jitter) / 16
	}
	r.arrival = arrival
	r.timestamp = p.Timestamp
	if at.Sub(r.last) < time.Second {
		return nil
	}
	r.last = at
	expected := r.highest - r.base + 1
	lost := int64(expected) - int64(r.received)
	interval := expected - r.expectedPrior
	received := r.received - r.receivedPrior
	r.expectedPrior = expected
	r.receivedPrior = r.received
	fraction := uint32(0)
	if interval > received && interval > 0 {
		fraction = ((interval - received) << 8) / interval
	}
	if lost < -8388608 {
		lost = -8388608
	}
	if lost > 8388607 {
		lost = 8388607
	}
	wire := make([]byte, 32)
	wire[0] = 0x81
	wire[1] = 201
	binary.BigEndian.PutUint16(wire[2:4], 7)
	binary.BigEndian.PutUint32(wire[4:8], sender)
	binary.BigEndian.PutUint32(wire[8:12], p.SSRC)
	binary.BigEndian.PutUint32(wire[12:16], fraction<<24|uint32(lost)&0xffffff)
	binary.BigEndian.PutUint32(wire[16:20], r.highest)
	binary.BigEndian.PutUint32(wire[20:24], uint32(r.jitter))
	return wire
}

// RTX starts with the original RTP sequence. Ownership must come from opcode12,
// never from a numeric SSRC offset or the most recently speaking participant.
func (u *udpConnImpl) AnnounceVideo(d GatewayMessageDataVideo) {
	u.rtxMu.Lock()
	defer u.rtxMu.Unlock()
	if u.rtxSources == nil {
		u.rtxSources = make(map[uint32]uint32)
		u.rtxOwners = make(map[uint32]snowflake.ID)
	}
	for ssrc, owner := range u.rtxOwners {
		if owner == d.UserID {
			delete(u.rtxSources, ssrc)
			delete(u.rtxOwners, ssrc)
		}
	}
	for _, s := range d.Streams {
		if s.SSRC != 0 && s.RTXSSRC != 0 && len(u.rtxSources) < 512 {
			u.rtxSources[s.RTXSSRC] = s.SSRC
			u.rtxOwners[s.RTXSSRC] = d.UserID
		}
	}
	if d.VideoSSRC != 0 && d.RTXSSRC != 0 && len(u.rtxSources) < 512 {
		u.rtxSources[d.RTXSSRC] = d.VideoSSRC
		u.rtxOwners[d.RTXSSRC] = d.UserID
	}
}
