//go:build cgo

package main

import (
	"testing"

	"github.com/hraban/opus"
)

// Offline check of the send/decode path: a tone frame round-trips through
// libopus with audible energy.
func TestToneRoundTrip(t *testing.T) {
	enc, err := opus.NewEncoder(48000, 2, opus.AppVoIP)
	if err != nil {
		t.Fatal(err)
	}
	tp := &toneProvider{enc: enc, pcm: make([]int16, 1920), out: make([]byte, 1400)}
	var frame []byte
	for i := 0; i < 5; i++ { // let the encoder settle
		if frame, err = tp.ProvideOpusFrame(); err != nil {
			t.Fatal(err)
		}
	}
	if len(frame) < 10 {
		t.Fatalf("frame too small: %d bytes", len(frame))
	}
	dec, _ := opus.NewDecoder(48000, 2)
	pcm := make([]int16, 5760*2)
	n, err := dec.Decode(frame, pcm)
	if err != nil || n != 960 {
		t.Fatalf("decode: n=%d err=%v", n, err)
	}
	var peak int16
	for _, s := range pcm[:n*2] {
		if s > peak {
			peak = s
		}
	}
	if peak < 3000 {
		t.Fatalf("decoded tone too quiet: peak=%d", peak)
	}
}
