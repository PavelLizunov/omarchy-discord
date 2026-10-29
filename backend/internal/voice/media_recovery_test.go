package voice

import (
	"bytes"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/disgoorg/godave"
	"github.com/disgoorg/snowflake/v2"
	"github.com/hraban/opus"
	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
)

func TestOpusPacketDurationPreservesSamples(t *testing.T) {
	for _, ms := range []int{10, 20, 40, 60} {
		enc, err := opus.NewEncoder(sampleRate, channels, opus.AppAudio)
		if err != nil {
			t.Fatal(err)
		}
		dec, err := opus.NewDecoder(sampleRate, channels)
		if err != nil {
			t.Fatal(err)
		}
		reference, err := opus.NewDecoder(sampleRate, channels)
		if err != nil {
			t.Fatal(err)
		}
		u := &user{dec: dec, pcm: make([]int16, frameLen)}
		packetSamples := sampleRate * channels * ms / 1000
		var want []int16
		for i := 0; i < 4; i++ {
			pcm := make([]int16, packetSamples)
			for j := range pcm {
				pcm[j] = int16((j+i*17)%200*50 - 5000)
			}
			wire := make([]byte, maxOpusBytes)
			n, e := enc.Encode(pcm, wire)
			if e != nil {
				t.Fatal(e)
			}
			decoded := make([]int16, sampleRate*channels*120/1000)
			samples, e := reference.Decode(wire[:n], decoded)
			if e != nil {
				t.Fatal(e)
			}
			want = append(want, decoded[:samples*channels]...)
			u.push(frame{seq: uint16(i), opus: bytes.Clone(wire[:n])})
		}
		var got []int16
		for len(got) < len(want) {
			if !u.pop(slog.Default()) {
				t.Fatalf("%dms packet stopped prematurely", ms)
			}
			got = append(got, u.pcm...)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%dms sample %d lost: %d != %d", ms, i, got[i], want[i])
			}
		}
	}
}

func TestPredictedFrameWaitsForRepairedReference(t *testing.T) {
	a := h264Assembly{}
	now := time.Now()
	a.push(&dvoice.Packet{Sequence: 1, Timestamp: 1, Opus: []byte{0x7c, 0x81, 1}}, now)
	a.push(&dvoice.Packet{Sequence: 3, Timestamp: 1, Marker: true, Opus: []byte{0x7c, 0x41, 3}}, now)
	out, err := a.push(&dvoice.Packet{Sequence: 4, Timestamp: 2, Marker: true, Opus: []byte{0x61, 4}}, now)
	if err != nil || out != nil {
		t.Fatal("predicted frame jumped its missing reference", out, err)
	}
	out, err = a.push(&dvoice.Packet{Sequence: 2, Timestamp: 1, Opus: []byte{0x7c, 0x01, 2}}, now)
	if err != nil || out == nil {
		t.Fatal("reference did not repair", err)
	}
	a.accept(1)
	out, err = a.next()
	if err != nil || !bytes.Equal(out, []byte{0, 0, 0, 1, 0x61, 4}) {
		t.Fatal("held frame not drained", out, err)
	}
}

func TestPictureLossRequestsRateLimitedKeyframe(t *testing.T) {
	v := newCamera()
	v.ssrc = 42
	v.decoder = &cameraDecoder{}
	now := time.Now()
	v.assembly.push(&dvoice.Packet{Sequence: 1, Timestamp: 1, Opus: []byte{0x7c, 0x81, 1}}, now)
	if _, ok := v.requestKeyframe(now); ok {
		t.Fatal("healthy picture requested refresh")
	}
	ssrc, ok := v.requestKeyframe(now.Add(videoReorderAge + time.Millisecond))
	if !ok || ssrc != 42 || !v.awaitingKeyframe {
		t.Fatal("lost reference did not request IDR")
	}
	if _, ok := v.requestKeyframe(now.Add(videoReorderAge + 2*time.Millisecond)); ok {
		t.Fatal("PLI flood")
	}
	if _, ok := v.requestKeyframe(now.Add(2 * time.Second)); !ok {
		t.Fatal("lost refresh cannot retry")
	}
	v.decoder = nil
}

func TestAudioSourceRestartResetsSequenceAndDecoder(t *testing.T) {
	r := newReceiver(slog.Default(), func(snowflake.ID, bool) {})
	defer r.Close()
	for _, seq := range []uint16{30000, 30001} {
		if err := r.ReceiveOpusFrame(123, &dvoice.Packet{SSRC: 1, Sequence: seq, Opus: silenceFrame}); err != nil {
			t.Fatal(err)
		}
	}
	r.mu.Lock()
	u := r.users[123]
	u.primed = true
	u.next = 30000
	oldDecoder := u.dec
	r.mu.Unlock()
	for _, seq := range []uint16{1, 2} {
		if err := r.ReceiveOpusFrame(123, &dvoice.Packet{SSRC: 2, Sequence: seq, Opus: silenceFrame}); err != nil {
			t.Fatal(err)
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if u.dec == oldDecoder || len(u.q) != 2 || u.primed || u.q[0].seq != 1 {
		t.Fatal("new source audio rejected by stale sequence state")
	}
}

func TestPublishedVideoFramesAreNotCappedAtFiveFPS(t *testing.T) {
	d := &cameraDecoder{}
	reader, writer := io.Pipe()
	done := make(chan struct{})
	go func() { d.readImages(reader); close(done) }()
	for i := 0; i < 10; i++ {
		if _, err := writer.Write([]byte{0xff, 0xd8, byte(i), 0xff, 0xd9}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(40 * time.Millisecond)
	}
	writer.Close()
	<-done
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.sequence < 8 {
		t.Fatalf("video still behaves like a slideshow: %d frames", d.sequence)
	}
}

func TestEntireMissingPictureTriggersReferenceRecovery(t *testing.T) {
	a := h264Assembly{}
	now := time.Now()
	a.push(&dvoice.Packet{Sequence: 1, Timestamp: 1, Marker: true, Opus: []byte{0x65, 1}}, now)
	a.accept(1)
	a.push(&dvoice.Packet{Sequence: 4, Timestamp: 3, Marker: true, Opus: []byte{0x61, 3}}, now)
	a.accept(3)
	if a.dropped == 0 {
		t.Fatal("whole missing picture was silently fed to predictive decoder")
	}
}

func TestDroppedReferenceSuppressesPredictedFramesUntilIDR(t *testing.T) {
	v := newCamera()
	v.selected = 123
	v.ssrc = 42
	v.awaitingKeyframe = true
	v.decoder = &cameraDecoder{frames: make(chan []byte, 4)}
	ds := godave.NewNoopSession(slog.Default(), "123", nil)
	feed := func(seq uint16, timestamp uint32, nal byte) {
		v.receive(&dvoice.Packet{SSRC: 42, Sequence: seq, Timestamp: timestamp, Marker: true, Opus: []byte{nal, 1}}, ds)
	}
	feed(1, 1, 0x65)
	if len(v.decoder.frames) != 1 {
		t.Fatal("initial IDR blocked")
	}
	feed(2, 2, 0x61)
	if len(v.decoder.frames) != 2 {
		t.Fatal("healthy predicted picture blocked")
	}
	feed(4, 4, 0x61)
	if len(v.decoder.frames) != 2 || !v.awaitingKeyframe {
		t.Fatal("prediction after whole-picture loss reached decoder")
	}
	feed(5, 5, 0x61)
	if len(v.decoder.frames) != 2 {
		t.Fatal("corruption propagated while waiting for IDR")
	}
	feed(6, 6, 0x65)
	if len(v.decoder.frames) != 3 || v.awaitingKeyframe {
		t.Fatal("IDR did not restore decoding")
	}
}
