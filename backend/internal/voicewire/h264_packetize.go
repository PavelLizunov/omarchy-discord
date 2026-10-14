package voice

import (
	"bytes"
	"errors"
)

// PacketizeH264 fragments an already DAVE-encrypted Annex B access unit. It does
// not encrypt, choose an SSRC or transmit; those belong to an authorized sender.
func PacketizeH264(accessUnit []byte, payloadLimit int) ([][]byte, error) {
	if len(accessUnit) == 0 || len(accessUnit) > 4<<20 || payloadLimit < 256 || payloadLimit > 1200 {
		return nil, errors.New("invalid H264 access unit or payload limit")
	}
	start := func(b []byte) int {
		if len(b) >= 4 && bytes.Equal(b[:4], []byte{0, 0, 0, 1}) {
			return 4
		}
		if len(b) >= 3 && bytes.Equal(b[:3], []byte{0, 0, 1}) {
			return 3
		}
		return 0
	}
	if start(accessUnit) == 0 {
		return nil, errors.New("H264 Annex B start code required")
	}
	var packets [][]byte
	for len(accessUnit) > 0 {
		prefix := start(accessUnit)
		if prefix == 0 {
			return nil, errors.New("invalid H264 boundary")
		}
		end := prefix
		for end < len(accessUnit) && start(accessUnit[end:]) == 0 {
			end++
		}
		nal := accessUnit[prefix:end]
		if len(nal) == 0 || nal[0]&0x80 != 0 || nal[0]&31 == 0 || nal[0]&31 >= 24 {
			return nil, errors.New("invalid H264 NAL")
		}
		needed := 1
		if len(nal) > payloadLimit {
			needed = (len(nal) - 1 + payloadLimit - 3) / (payloadLimit - 2)
		}
		if len(packets)+needed > 512 {
			return nil, errors.New("H264 packet budget exceeded")
		}
		if len(nal) <= payloadLimit {
			packets = append(packets, append([]byte(nil), nal...))
		} else {
			header := nal[0]
			body := nal[1:]
			first := true
			for len(body) > 0 {
				count := min(payloadLimit-2, len(body))
				fragment := make([]byte, count+2)
				fragment[0], fragment[1] = (header&0xe0)|28, header&31
				if first {
					fragment[1] |= 0x80
				}
				if count == len(body) {
					fragment[1] |= 0x40
				}
				copy(fragment[2:], body[:count])
				packets = append(packets, fragment)
				first = false
				body = body[count:]
			}
		}
		accessUnit = accessUnit[end:]
	}
	return packets, nil
}
