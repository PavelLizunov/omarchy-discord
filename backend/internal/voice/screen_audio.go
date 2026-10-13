package voice

import (
	"log/slog"
	"sync/atomic"

	"github.com/disgoorg/snowflake/v2"
	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
)

// Stream audio joins the existing call output, never a second playback/capture.
type screenAudio struct {
	rx      *receiver
	user    snowflake.ID
	packets atomic.Uint64
	errors  atomic.Uint64
}

func newScreenAudio(log *slog.Logger, user snowflake.ID) *screenAudio {
	r := newReceiver(log, func(snowflake.ID, bool) {})
	r.metrics = &audioMetrics{}
	return &screenAudio{rx: r, user: user}
}
func (a *screenAudio) receive(user snowflake.ID, p *dvoice.Packet) {
	if p.Type != 120 || user != a.user || user == 0 {
		return
	}
	if err := a.rx.ReceiveOpusFrame(user, p); err != nil {
		a.errors.Add(1)
		return
	}
	a.packets.Add(1)
}
func (e *Engine) SetScreenAudio(s *Screen) {
	e.opMu.Lock()
	defer e.opMu.Unlock()
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.audio == nil {
		return
	}
	r := e.audio.rx
	r.mu.Lock()
	defer r.mu.Unlock()
	var next *receiver
	if s != nil && s.audio != nil {
		next = s.audio.rx
	}
	if r.screen != next {
		r.pending = nil
		r.screen = next
	}
}
