package voice

import (
	"bytes"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/hraban/opus"
	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
)

func TestConcealmentUsesPacketDuration(t *testing.T) {
	for _, ms := range []int{10, 20, 40, 60} {
		enc, err := opus.NewEncoder(sampleRate, channels, opus.AppAudio)
		if err != nil {
			t.Fatal(err)
		}
		dec, err := opus.NewDecoder(sampleRate, channels)
		if err != nil {
			t.Fatal(err)
		}
		ref, err := opus.NewDecoder(sampleRate, channels)
		if err != nil {
			t.Fatal(err)
		}
		pcm := make([]int16, sampleRate*channels*ms/1000)
		for i := range pcm {
			pcm[i] = int16(i%200*50 - 5000)
		}
		wire := make([]byte, maxOpusBytes)
		n, err := enc.Encode(pcm, wire)
		if err != nil {
			t.Fatal(err)
		}
		wire = wire[:n]
		u := &user{dec: dec, pcm: make([]int16, frameLen)}
		u.push(frame{seq: 1, opus: bytes.Clone(wire)})
		u.push(frame{seq: 3, opus: bytes.Clone(wire)})
		decoded := make([]int16, sampleRate*channels*120/1000)
		n, err = ref.Decode(wire, decoded)
		if err != nil {
			t.Fatal(err)
		}
		want := append([]int16(nil), decoded[:n*channels]...)
		concealed := make([]int16, len(pcm))
		if err := ref.DecodePLC(concealed); err != nil {
			t.Fatal(err)
		}
		want = append(want, concealed...)
		n, err = ref.Decode(wire, decoded)
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, decoded[:n*channels]...)
		var got []int16
		for len(got) < len(want) {
			if !u.pop(slog.Default()) {
				t.Fatalf("%dms: playout stopped", ms)
			}
			got = append(got, u.pcm...)
		}
		if !slices.Equal(got[:len(want)], want) {
			t.Fatalf("%dms: packet loss changed playout duration", ms)
		}
	}
}

func TestH264RejectsMalformedAggregationAndFragments(t *testing.T) {
	for _, packets := range [][][]byte{
		{{24}},
		{{24, 0, 1, 0x65, 0}},
		{{24, 0, 1, 0x80}},
		{{0xe5, 1}},
		{{0x7c, 0xa5, 1}, {0x7c, 0x45, 2}},
		{{0x7c, 0x85, 1}, {0x5c, 0x45, 2}},
		{{0x7c, 0xc5, 1}},
	} {
		a := h264Assembly{}
		var result []byte
		var err error
		for i, payload := range packets {
			result, err = a.push(&dvoice.Packet{Sequence: uint16(i), Timestamp: 1, Marker: i == len(packets)-1, Opus: payload}, time.Now())
		}
		if result != nil || err == nil {
			t.Fatalf("malformed payload admitted: %x, err=%v", packets, err)
		}
	}
}
