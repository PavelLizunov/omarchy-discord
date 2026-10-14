package voice

import (
	"context"
	"errors"

	"github.com/disgoorg/godave"
)

// OpenMedia opens a separately authorized Go Live transport without issuing a
// main voice-state update. Closing it must not disconnect the user's voice call.
func (c *connImpl) OpenMedia(ctx context.Context, state State) error {
	c.mediaOnly = true
	if state.GuildID == 0 || state.ChannelID == 0 || state.SessionID == "" || state.Token == "" || state.Endpoint == "" {
		return errors.New("stream handshake is incomplete")
	}
	state.MediaOnly = true
	c.stateMu.Lock()
	c.state = state
	c.stateMu.Unlock()
	ready, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.openedFunc = cancel
	if err := c.gateway.Open(ctx, state); err != nil {
		return err
	}
	select {
	case <-ready.Done():
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Media group differs from the enclosing voice channel for Go Live.
func mediaGroup(state State) godave.ChannelID {
	if state.MediaGroupID != 0 {
		return godave.ChannelID(state.MediaGroupID)
	}
	return godave.ChannelID(state.ChannelID)
}
