package voice

import (
	"context"
	"errors"
	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
	"time"
)

func validQuality(q int) bool { return q == 360 || q == 720 || q == 1080 }
func (v *camera) setQuality(ctx context.Context, c dvoice.Conn, q int) error {
	if !validQuality(q) {
		return errors.New("choose 360, 720 or 1080 quality")
	}
	v.mu.Lock()
	old, ssrc, decoder := v.quality, v.ssrc, v.decoder
	v.mu.Unlock()
	var replacement *cameraDecoder
	if decoder != nil && old != q {
		var err error
		replacement, err = newCameraDecoderQuality(q)
		if err != nil {
			return err
		}
	}
	v.mu.Lock()
	v.quality = q
	v.mu.Unlock()
	if ssrc != 0 {
		if err := c.Gateway().Send(ctx, dvoice.OpcodeMediaSinkWants, v.sinkWants(ssrc)); err != nil {
			v.mu.Lock()
			v.quality = old
			v.mu.Unlock()
			if replacement != nil {
				replacement.close()
			}
			return err
		}
	}
	if replacement != nil {
		v.mu.Lock()
		if v.decoder == decoder {
			v.decoder = replacement
			v.awaitingKeyframe = true
			v.lastPLI = time.Time{}
			replacement = nil
		}
		v.mu.Unlock()
		if replacement != nil {
			replacement.close()
		} else {
			decoder.close()
		}
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
