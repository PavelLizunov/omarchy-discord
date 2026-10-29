package voice

import (
	"errors"
	"time"

	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
	"github.com/pion/rtp/codecs"
)

// One selected stream: bounded jitter/RTX repair, no retained recording.
const videoReorderPackets = 512
const videoReorderUnits = 32
const videoReorderAge = 500 * time.Millisecond

type h264AccessUnit struct {
	packets map[uint16]*dvoice.Packet
	bytes   int
	at      time.Time
	end     uint16
	marked  bool
}
type h264Assembly struct {
	units              map[uint32]*h264AccessUnit
	padding            map[uint16]time.Time
	finished           []uint32
	bytes              int
	dropped            uint64
	reordered          uint64
	duplicates         uint64
	completed          uint64
	lastNACK           time.Time
	nackRounds         int
	candidateTimestamp uint32
	acceptedTimestamp  uint32
	hasAccepted        bool
	acceptedEnd        uint16
}

func (a *h264Assembly) reset() { *a = h264Assembly{} }
func (a *h264Assembly) push(p *dvoice.Packet, now time.Time) ([]byte, error) {
	if a.units == nil {
		a.units = make(map[uint32]*h264AccessUnit)
		a.padding = make(map[uint16]time.Time)
	}
	a.expire(now)
	if len(p.Opus) == 0 {
		if len(a.padding) < videoReorderPackets {
			a.padding[p.Sequence] = now
		}
		for timestamp, u := range a.units {
			if u.marked {
				out, err := a.complete(timestamp, u)
				if out != nil || err != nil {
					return out, err
				}
			}
		}
		return nil, nil
	}
	if a.hasAccepted && int32(p.Timestamp-a.acceptedTimestamp) <= 0 {
		a.duplicates++
		return nil, nil
	}
	for _, timestamp := range a.finished {
		if timestamp == p.Timestamp {
			a.duplicates++
			return nil, nil
		}
	}
	u := a.units[p.Timestamp]
	if u == nil {
		if len(a.units) >= videoReorderUnits {
			var oldest uint32
			at := now
			found := false
			for t, unit := range a.units {
				if !found || unit.at.Before(at) {
					found = true
					oldest = t
					at = unit.at
				}
			}
			a.bytes -= a.units[oldest].bytes
			delete(a.units, oldest)
			a.dropped++
		}
		u = &h264AccessUnit{packets: make(map[uint16]*dvoice.Packet), at: now}
		a.units[p.Timestamp] = u
	}
	if _, exists := u.packets[p.Sequence]; exists {
		a.duplicates++
		return nil, nil
	}
	if len(u.packets) >= videoReorderPackets || a.bytes+len(p.Opus) > cameraFrameLimit {
		a.bytes -= u.bytes
		delete(a.units, p.Timestamp)
		a.dropped++
		return nil, errors.New("video reorder limit reached")
	}
	copyPacket := *p
	copyPacket.Opus = append([]byte(nil), p.Opus...)
	u.packets[p.Sequence] = &copyPacket
	u.bytes += len(p.Opus)
	a.bytes += len(p.Opus)
	if p.Marker {
		u.end = p.Sequence
		u.marked = true
	}
	return a.complete(p.Timestamp, u)
}
func (a *h264Assembly) complete(timestamp uint32, u *h264AccessUnit) ([]byte, error) {
	if !u.marked {
		return nil, nil
	}
	// A complete predicted picture cannot jump over an earlier reference
	// picture still being repaired. An IDR can safely replace the old GOP.
	keyframe := false
	for _, packet := range u.packets {
		if h264PacketIDR(packet.Opus) {
			keyframe = true
			break
		}
	}
	if !keyframe {
		for older := range a.units {
			if int32(older-timestamp) < 0 {
				return nil, nil
			}
		}
	}
	// The earliest start before the marker wins; FU continuations are not starts.
	var start uint16
	found := false
	distance := uint16(0)
	for seq, packet := range u.packets {
		if !h264PacketStart(packet.Opus) {
			continue
		}
		d := uint16(u.end - seq)
		if d < videoReorderPackets && (!found || d > distance) {
			start = seq
			distance = d
			found = true
		}
	}
	if !found {
		return nil, nil
	}
	var frame []byte
	var depacketizer codecs.H264Packet
	fragmentOpen := false
	var fragmentHeader byte
	for offset := uint16(0); offset <= distance; offset++ {
		seq := start + offset
		packet := u.packets[seq]
		if packet == nil {
			if _, ok := a.padding[seq]; ok {
				continue
			}
			a.reordered++
			return nil, nil
		}
		payload := packet.Opus
		if payload[0]&0x80 != 0 {
			return nil, errors.New("invalid H264 forbidden bit")
		}
		typ := payload[0] & 31
		if typ == 28 {
			if len(payload) < 3 || payload[1]&0x20 != 0 || payload[1]&31 == 0 || payload[1]&31 > 23 {
				return nil, errors.New("invalid H264 fragment")
			}
			begin, end := payload[1]&128 != 0, payload[1]&64 != 0
			if begin && end {
				return nil, errors.New("invalid H264 fragment flags")
			}
			if begin {
				if fragmentOpen {
					return nil, errors.New("overlapping H264 fragments")
				}
				fragmentOpen = true
				fragmentHeader = payload[0]&0x60 | payload[1]&31
			} else if !fragmentOpen {
				return nil, nil
			} else if fragmentHeader != payload[0]&0x60|payload[1]&31 {
				return nil, errors.New("inconsistent H264 fragment header")
			}
			if end {
				fragmentOpen = false
			}
		} else if fragmentOpen {
			return nil, errors.New("missing H264 fragment end")
		}
		if typ == 24 {
			if err := validateH264Aggregation(payload); err != nil {
				return nil, err
			}
		}
		nal, err := depacketizer.Unmarshal(payload)
		if err != nil {
			return nil, err
		}
		if len(frame)+len(nal) > cameraFrameLimit {
			return nil, errors.New("camera frame exceeds limit")
		}
		frame = append(frame, nal...)
	}
	if fragmentOpen {
		return nil, nil
	}
	a.candidateTimestamp = timestamp
	// Marker can arrive before an earlier NAL. Keep the candidate until DAVE
	// authentication succeeds; later primary/RTX packets can complete it.
	return frame, nil
}
func (a *h264Assembly) expire(now time.Time) {
	for timestamp, u := range a.units {
		if now.Sub(u.at) > videoReorderAge {
			a.bytes -= u.bytes
			delete(a.units, timestamp)
			a.dropped++
		}
	}
	for sequence, at := range a.padding {
		if now.Sub(at) > videoReorderAge {
			delete(a.padding, sequence)
		}
	}
}
func (a *h264Assembly) accept(timestamp uint32) {
	if unit := a.units[timestamp]; unit != nil && unit.marked {
		if a.hasAccepted {
			// A whole lost access unit has no map entry to expire. Detect its
			// missing sequence range before accepting the next picture.
			distance := uint16(unit.end - a.acceptedEnd)
			if distance > 0 && distance < 32768 {
				for i := uint16(1); i <= distance; i++ {
					seq := a.acceptedEnd + i
					if unit.packets[seq] == nil {
						if _, padding := a.padding[seq]; !padding {
							a.dropped++
							break
						}
					}
				}
			}
		}
		a.acceptedEnd = unit.end
	}
	a.acceptedTimestamp = timestamp
	a.hasAccepted = true
	for older, u := range a.units {
		if int32(older-timestamp) < 0 {
			a.bytes -= u.bytes
			delete(a.units, older)
			a.dropped++
		}
	}
	if u := a.units[timestamp]; u != nil {
		a.bytes -= u.bytes
		delete(a.units, timestamp)
	}
	a.finished = append(a.finished, timestamp)
	if len(a.finished) > 4 {
		a.finished = a.finished[1:]
	}
	a.completed++
}

// Bound retransmission requests to four NACK rounds per 500ms assembly window.
func (a *h264Assembly) missing(now time.Time) []uint16 {
	a.expire(now)
	if !a.lastNACK.IsZero() && now.Sub(a.lastNACK) < 50*time.Millisecond {
		return nil
	}
	if a.nackRounds >= 4 {
		if now.Sub(a.lastNACK) < videoReorderAge {
			return nil
		}
		a.nackRounds = 0
	}
	var missing []uint16
	for _, u := range a.units {
		if !u.marked {
			continue
		}
		var start uint16
		var distance uint16
		found := false
		for seq, p := range u.packets {
			d := uint16(u.end - seq)
			if d < videoReorderPackets && h264PacketStart(p.Opus) && (!found || d > distance) {
				start = seq
				distance = d
				found = true
			}
		}
		if !found {
			continue
		}
		for i := uint16(0); i <= distance && len(missing) < 16; i++ {
			seq := start + i
			if u.packets[seq] == nil {
				if _, probe := a.padding[seq]; !probe {
					missing = append(missing, seq)
				}
			}
		}
	}
	if len(missing) > 0 {
		a.lastNACK = now
		a.nackRounds++
	}
	return missing
}

// Pion permits a trailing STAP-A length byte; retain strict frame admission
// before authenticated DAVE reconstruction rather than silently truncating it.
func validateH264Aggregation(payload []byte) error {
	if len(payload) < 4 {
		return errors.New("empty H264 aggregation")
	}
	for offset := 1; offset < len(payload); {
		if offset+2 > len(payload) {
			return errors.New("invalid H264 aggregation")
		}
		n := int(payload[offset])<<8 | int(payload[offset+1])
		offset += 2
		if n == 0 || offset+n > len(payload) {
			return errors.New("invalid H264 aggregation")
		}
		if payload[offset]&0x80 != 0 || payload[offset]&31 == 0 || payload[offset]&31 >= 24 {
			return errors.New("invalid aggregated H264 NAL")
		}
		offset += n
	}
	return nil
}

func h264PacketStart(payload []byte) bool {
	if len(payload) == 0 {
		return false
	}
	typ := payload[0] & 31
	return typ >= 1 && typ <= 24 || typ == 28 && len(payload) >= 3 && payload[1]&128 != 0
}

func h264PacketIDR(payload []byte) bool {
	if len(payload) == 0 {
		return false
	}
	switch payload[0] & 31 {
	case 5:
		return true
	case 28:
		return len(payload) > 1 && payload[1]&31 == 5 && payload[1]&128 != 0
	case 24:
		for offset := 1; offset+2 <= len(payload); {
			n := int(payload[offset])<<8 | int(payload[offset+1])
			offset += 2
			if n == 0 || offset+n > len(payload) {
				return false
			}
			if payload[offset]&31 == 5 {
				return true
			}
			offset += n
		}
	}
	return false
}

func h264IDR(frame []byte) bool {
	for i := 0; i+3 < len(frame); i++ {
		if frame[i] == 0 && frame[i+1] == 0 && frame[i+2] == 1 && frame[i+3]&31 == 5 {
			return true
		}
	}
	return false
}

// Drain completed pictures held behind a repaired reference in timestamp order.
func (a *h264Assembly) next() ([]byte, error) {
	var timestamp uint32
	found := false
	for candidate := range a.units {
		if !found || int32(candidate-timestamp) < 0 {
			timestamp = candidate
			found = true
		}
	}
	if !found {
		return nil, nil
	}
	return a.complete(timestamp, a.units[timestamp])
}
