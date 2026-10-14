package voice

import (
	"bytes"
	"testing"
	"time"

	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
)

func TestH264OutboundPacketizerRoundtrip(t *testing.T) {
	frame := append([]byte{0, 0, 0, 1, 0x67, 0x42, 0x11, 0, 0, 0, 1, 0x65}, bytes.Repeat([]byte{0x55}, 3500)...)
	packets, err := dvoice.PacketizeH264(frame, 1000)
	if err != nil {
		t.Fatal(err)
	}
	var assembly h264Assembly
	var complete []byte
	for i, p := range packets {
		if len(p) > 1000 {
			t.Fatal("MTU exceeded")
		}
		complete, err = assembly.push(&dvoice.Packet{SSRC: 42, Type: dvoice.H264PayloadType, Sequence: uint16(65533 + i), Timestamp: 9000, Marker: i == len(packets)-1, Opus: p}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(complete, frame) {
		t.Fatal("encoded access unit changed")
	}
	packets[0][0] = 0
	if frame[4] != 0x67 {
		t.Fatal("packet aliases caller frame")
	}
	for _, bad := range [][]byte{nil, {0x65, 1, 2}, {0, 0, 0, 1}, {0, 0, 0, 1, 0x80}, {0, 0, 0, 1, 24}} {
		if _, err := dvoice.PacketizeH264(bad, 1000); err == nil {
			t.Fatal("invalid frame accepted")
		}
	}
	if _, err := dvoice.PacketizeH264(frame, 2); err == nil {
		t.Fatal("invalid MTU accepted")
	}
	huge := append([]byte{0, 0, 0, 1, 0x65}, bytes.Repeat([]byte{0x55}, 600000)...)
	if _, err := dvoice.PacketizeH264(huge, 256); err == nil {
		t.Fatal("packet budget ignored")
	}
}
