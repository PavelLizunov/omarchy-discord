package voice

import (
	"context"
	"errors"
	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
)

func validQuality(q int) bool { return q == 360 || q == 720 || q == 1080 }
func (v *camera) setQuality(ctx context.Context, c dvoice.Conn, q int) error {
	if !validQuality(q) {
		return errors.New("choose 360, 720 or 1080 quality")
	}
	v.mu.Lock()
	old, ssrc, decoder := v.quality, v.ssrc, v.decoder
	v.quality = q
	v.mu.Unlock()
	if ssrc != 0 {
		if err := c.Gateway().Send(ctx, dvoice.OpcodeMediaSinkWants, v.sinkWants(ssrc)); err != nil {
			v.mu.Lock()
			v.quality = old
			v.mu.Unlock()
			return err
		}
	}
	if decoder != nil {
		decoder.quality.Store(int64(q))
	}
	return nil
}
func (s *Screen) SetVideoSettings(ctx context.Context, quality *int, volume *int) error {
	if quality != nil && !validQuality(*quality) {
		return errors.New("choose 360, 720 or 1080 quality")
	}
	if volume != nil && (*volume < 0 || *volume > 200) {
		return errors.New("stream volume must be 0 to 200 percent")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.conn == nil {
		return errors.New("screen is closed")
	}
	if quality != nil {
		if err := s.video.setQuality(ctx, s.conn, *quality); err != nil {
			return err
		}
	}
	if volume != nil {
		s.audio.volume.Store(int64(*volume))
	}
	return nil
}
func (e *Engine) SetVideoQuality(ctx context.Context, q int) error {
	e.opMu.Lock()
	defer e.opMu.Unlock()
	e.mu.Lock()
	v, c := e.video, e.conn
	e.mu.Unlock()
	if v == nil || c == nil {
		return errors.New("join voice before changing quality")
	}
	return v.setQuality(ctx, c, q)
}
