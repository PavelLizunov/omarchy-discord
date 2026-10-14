package voice

import (
	"errors"

	"github.com/disgoorg/snowflake/v2"
)

// UserAudio applies only to local playback, never to another user's account.
type UserAudio struct {
	Volume int  `json:"volume"`
	Muted  bool `json:"muted"`
}

func userAudio(levels map[snowflake.ID]UserAudio, id snowflake.ID) UserAudio {
	if level, ok := levels[id]; ok {
		return level
	}
	return UserAudio{Volume: 100}
}

// UserLevels is an immutable snapshot. Preferences survive reconnects in this
// signed-in backend session; they are not persisted across backend restarts.
func (e *Engine) UserLevels() map[string]UserAudio {
	e.opMu.Lock()
	defer e.opMu.Unlock()
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.audio != nil {
		e.audio.rx.mu.Lock()
		defer e.audio.rx.mu.Unlock()
	}
	out := make(map[string]UserAudio, len(e.userLevels))
	for id, level := range e.userLevels {
		out[id.String()] = level
	}
	return out
}

func (e *Engine) SetUserAudio(id snowflake.ID, volume *int, muted *bool) error {
	if id == 0 || (volume == nil && muted == nil) {
		return errors.New("participant and at least one audio setting are required")
	}
	if volume != nil && (*volume < 0 || *volume > 200) {
		return errors.New("participant volume must be between 0 and 200 percent")
	}
	e.opMu.Lock()
	defer e.opMu.Unlock()
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.audio == nil || e.st.Status != StatusConnected {
		return errors.New("join voice before changing participant audio")
	}
	r := e.audio.rx
	r.mu.Lock()
	defer r.mu.Unlock()
	if e.userLevels == nil {
		e.userLevels = make(map[snowflake.ID]UserAudio)
	}
	level := userAudio(e.userLevels, id)
	if volume != nil {
		level.Volume = *volume
	}
	if muted != nil {
		level.Muted = *muted
	}
	if _, exists := e.userLevels[id]; !exists && len(e.userLevels) >= 512 {
		return errors.New("participant audio preference limit reached")
	}
	if level.Volume == 100 && !level.Muted {
		delete(e.userLevels, id)
	} else {
		e.userLevels[id] = level
	}
	r.levels = e.userLevels
	// Do not play already mixed samples after a mute or volume change.
	r.pending = nil
	return nil
}
