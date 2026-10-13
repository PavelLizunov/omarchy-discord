package voice

import (
	"bytes"
	"testing"
	"time"

	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
)

func TestH264AssemblyReorderPaddingRTXAndNoPartialDecrypt(t *testing.T) {
	now := time.Unix(1700000000, 0)
	cases := [][]*dvoice.Packet{
		{{Sequence: 10, Timestamp: 1, Opus: []byte{0x7c, 0x85, 1}}, {Sequence: 12, Timestamp: 1, Marker: true, Opus: []byte{0x7c, 0x45, 2}}, {Sequence: 11, Timestamp: 1}},
		{{Sequence: 12, Timestamp: 1, Marker: true, Opus: []byte{0x7c, 0x45, 2}}, {Sequence: 10, Timestamp: 1, Opus: []byte{0x7c, 0x85, 1}}, {Sequence: 11, Timestamp: 1, Opus: []byte{0x7c, 0x05, 3}}},
		{{Sequence: 65535, Timestamp: 1, Opus: []byte{0x7c, 0x85, 1}}, {Sequence: 1, Timestamp: 1, Marker: true, Opus: []byte{0x7c, 0x45, 2}}, {Sequence: 0, Timestamp: 1}},
	}
	for i, packets := range cases {
		a := h264Assembly{}
		var got []byte
		for j, p := range packets {
			out, e := a.push(p, now.Add(time.Duration(j)*time.Millisecond))
			if e != nil {
				t.Fatal(i, e)
			}
			if j < len(packets)-1 && len(out) > 0 {
				t.Fatal("incomplete unit released", i)
			}
			if out != nil {
				got = out
			}
		}
		expected := []byte{0, 0, 0, 1, 0x65, 1, 2}
		if i == 1 {
			expected = []byte{0, 0, 0, 1, 0x65, 1, 3, 2}
		}
		if !bytes.Equal(got, expected) {
			t.Fatal(i, got)
		}
		a.accept(1)
		out, _ := a.push(packets[len(packets)-1], now.Add(time.Second))
		if out != nil {
			t.Fatal("duplicate frame released")
		}
	}
}
func TestH264AssemblyMissingStartAndEndNeverReleased(t *testing.T) {
	at := time.Unix(1700000000, 0)
	a := h264Assembly{}
	if p, e := a.push(&dvoice.Packet{Sequence: 3, Timestamp: 1, Marker: true, Opus: []byte{0x7c, 0x45, 2}}, at); p != nil || e != nil {
		t.Fatal("middle fragment treated as frame", p, e)
	}
	a.push(&dvoice.Packet{Sequence: 4, Timestamp: 2, Opus: []byte{0x7c, 0x85, 2}}, at)
	if p, e := a.push(&dvoice.Packet{Sequence: 5, Timestamp: 2, Marker: true, Opus: []byte{0x7c, 0x05, 2}}, at); p != nil || e != nil {
		t.Fatal("missing FU end admitted", p, e)
	}
	a.push(&dvoice.Packet{Sequence: 6, Timestamp: 3, Marker: true, Opus: []byte{0x65, 1}}, at.Add(time.Second))
	if a.dropped != 2 || len(a.units) > 4 {
		t.Fatal("expired partial units retained", a)
	}
}

func TestH264MissingNACKBoundAndPaddingExclusion(t *testing.T) {
	a := h264Assembly{}
	at := time.Unix(1700000000, 0)
	a.push(&dvoice.Packet{Sequence: 10, Timestamp: 1, Opus: []byte{0x7c, 0x85, 1}}, at)
	a.push(&dvoice.Packet{Sequence: 13, Timestamp: 1, Marker: true, Opus: []byte{0x7c, 0x45, 2}}, at)
	a.push(&dvoice.Packet{Sequence: 11, Timestamp: 1}, at)
	m := a.missing(at)
	if len(m) != 1 || m[0] != 12 {
		t.Fatal("NACK guessed media or included probe", m)
	}
	for i := 1; i < 8; i++ {
		m = a.missing(at.Add(time.Duration(i) * 50 * time.Millisecond))
		if i >= 4 && len(m) > 0 {
			t.Fatal("unbounded NACK retry")
		}
	}
}

func TestH264AssemblyEqualTimeEvictionBound(t *testing.T) {
	a := h264Assembly{}
	at := time.Unix(1700000000, 0)
	for i := 0; i < 20; i++ {
		a.push(&dvoice.Packet{Sequence: 1, Timestamp: uint32(i + 1), Opus: []byte{0x7c, 0x85, 1}}, at)
	}
	if len(a.units) != 4 || a.dropped != 16 {
		t.Fatal("unit eviction bound", len(a.units), a.dropped)
	}
}

func TestH264LateRTXNeverReversesAcceptedFrame(t *testing.T) {
	at := time.Unix(1700000000, 0)
	a := h264Assembly{}
	a.push(&dvoice.Packet{Timestamp: 1, Sequence: 10, Opus: []byte{0x7c, 0x85, 1}}, at)
	a.push(&dvoice.Packet{Timestamp: 1, Sequence: 12, Marker: true, Opus: []byte{0x7c, 0x45, 2}}, at)
	f, e := a.push(&dvoice.Packet{Timestamp: 2, Sequence: 13, Marker: true, Opus: []byte{0x65, 2}}, at)
	if e != nil || f == nil {
		t.Fatal(e)
	}
	a.accept(2)
	f, e = a.push(&dvoice.Packet{Timestamp: 1, Sequence: 11, Opus: []byte{0x7c, 0x05, 1}}, at.Add(time.Millisecond))
	if e != nil || f != nil {
		t.Fatal("old repaired frame reversed decoder order", f, e)
	}
	a.accept(0xfffffff0)
	f, e = a.push(&dvoice.Packet{Timestamp: 10, Sequence: 14, Marker: true, Opus: []byte{0x65, 3}}, at)
	if f == nil || e != nil {
		t.Fatal("timestamp wrap rejected", f, e)
	}
}
func TestH264ExpiredGapCannotRestartNACK(t *testing.T) {
	at := time.Unix(1700000000, 0)
	a := h264Assembly{}
	a.push(&dvoice.Packet{Timestamp: 1, Sequence: 10, Opus: []byte{0x7c, 0x85, 1}}, at)
	a.push(&dvoice.Packet{Timestamp: 1, Sequence: 12, Marker: true, Opus: []byte{0x7c, 0x45, 2}}, at)
	for i := 1; i < 10; i++ {
		if missing := a.missing(at.Add(time.Duration(i) * time.Second)); len(missing) > 0 {
			t.Fatal("expired NACK repeated", missing)
		}
	}
	if a.bytes != 0 || len(a.units) != 0 {
		t.Fatal("expired buffer retained")
	}
}
