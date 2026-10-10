package voice

import (
	"log/slog"
	"math"
	"sync"
	"testing"

	"github.com/disgoorg/snowflake/v2"
	"github.com/hraban/opus"
	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
)

func TestParticipantMixerIsolation(t *testing.T) {
	enc, err := opus.NewEncoder(sampleRate, channels, opus.AppVoIP)
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, maxOpusBytes)
	n, err := enc.Encode(tone(5000), data)
	if err != nil {
		t.Fatal(err)
	}
	render := func(levels map[snowflake.ID]UserAudio, ids ...snowflake.ID) []int16 {
		r := newReceiver(slog.Default(), func(snowflake.ID, bool) {})
		r.levels = levels
		defer r.Close()
		for _, id := range ids {
			for seq := uint16(0); seq < 4; seq++ {
				if err := r.ReceiveOpusFrame(id, &dvoice.Packet{Sequence: seq, Opus: data[:n]}); err != nil {
					t.Fatal(err)
				}
			}
		}
		out := make([]int16, frameLen*2)
		r.read(out)
		return out
	}
	baseline := render(nil, 2)
	muted := render(map[snowflake.ID]UserAudio{1: {Volume: 100, Muted: true}}, 1, 2)
	for i := range baseline {
		if muted[i] != baseline[i] {
			t.Fatal("muted participant changed another user's output")
		}
	}
	half := render(map[snowflake.ID]UserAudio{2: {Volume: 50}}, 2)
	if ratio := rms(half) / rms(baseline); math.Abs(ratio-.5) > .002 {
		t.Fatal("gain", ratio)
	}
	zero := render(map[snowflake.ID]UserAudio{2: {Volume: 0}}, 2)
	if rms(zero) != 0 {
		t.Fatal("zero volume audible")
	}
	both := render(nil, 1, 2)
	if rms(both) < rms(baseline)*1.9 {
		t.Fatal("default gain not preserved")
	}
}

func TestParticipantAudioValidationSnapshotAndConcurrentMix(t *testing.T) {
	r := newReceiver(slog.Default(), func(snowflake.ID, bool) {})
	defer r.Close()
	e := &Engine{st: State{Status: StatusConnected}, audio: &audio{rx: r}}
	v, m := 50, true
	if err := e.SetUserAudio(7, &v, &m); err != nil {
		t.Fatal(err)
	}
	s := e.UserLevels()
	s["7"] = UserAudio{Volume: 200}
	if e.UserLevels()["7"].Volume != 50 {
		t.Fatal("mutable snapshot")
	}
	for _, bad := range []int{-1, 201} {
		if e.SetUserAudio(7, &bad, nil) == nil {
			t.Fatal("invalid gain")
		}
	}
	if e.SetUserAudio(0, &v, nil) == nil || e.SetUserAudio(7, nil, nil) == nil {
		t.Fatal("invalid identity/empty accepted")
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			v := i % 201
			if err := e.SetUserAudio(7, &v, nil); err != nil {
				t.Error(err)
			}
			e.UserLevels()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			r.read(make([]int16, frameLen))
		}
	}()
	wg.Wait()
	e.st.Status = StatusIdle
	if e.SetUserAudio(7, &v, nil) == nil {
		t.Fatal("idle accepted")
	}
}

func TestParticipantSettingsSurviveRejoin(t *testing.T) {
	h := newHarness(t)
	if err := h.join(1, 2, 0); err != nil {
		t.Fatal(err)
	}
	v, m := 35, true
	if err := h.e.SetUserAudio(8, &v, &m); err != nil {
		t.Fatal(err)
	}
	if err := h.join(1, 3, 1); err != nil {
		t.Fatal(err)
	}
	if level := h.e.UserLevels()["8"]; level.Volume != 35 || !level.Muted {
		t.Fatal(level)
	}
	if h.e.audio.rx.levels[8].Volume != 35 {
		t.Fatal("new mixer lost preferences")
	}
}
