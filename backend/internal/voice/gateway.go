package voice

import (
	"context"
	"errors"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	ddiscord "github.com/disgoorg/disgo/discord"
	dgateway "github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/snowflake/v2"

	"github.com/mattcalayo/omarchy-discord/backend/internal/panics"
)

// installHandlers forwards arikawa's two voice events to disgo. Sync handlers
// keep Discord's order (state update first, then server update).
func (e *Engine) installHandlers() {
	e.n.AddSyncHandler(func(ev *gateway.VoiceStateUpdateEvent) {
		defer panics.Recover("voice: voice_state_update")
		e.mu.Lock()
		mgr, self, gen := e.mgr, e.selfID, e.gen
		e.mu.Unlock()
		if mgr != nil {
			mgr.HandleVoiceStateUpdate(toVoiceState(ev))
		}
		if self.IsValid() && ev.UserID == self {
			// Off the dispatch goroutine: Join blocks on this very event.
			panics.Go("voice: self state", func() { e.onSelfState(gen, ev.ChannelID) })
		}
	})
	e.n.AddSyncHandler(func(ev *gateway.VoiceServerUpdateEvent) {
		defer panics.Recover("voice: voice_server_update")
		e.mu.Lock()
		mgr := e.mgr
		e.mu.Unlock()
		if mgr != nil {
			mgr.HandleVoiceServerUpdate(toVoiceServer(ev))
		}
	})
}

// onSelfState tracks our own voice state: the server moved us or dropped us
// (channel 0 while we did not leave).
func (e *Engine) onSelfState(gen uint64, channelID discord.ChannelID) {
	e.update(func() func() {
		if gen != e.gen || e.st.Status == StatusIdle || e.st.Status == StatusError {
			return nil
		}
		if !channelID.IsValid() {
			e.log.Warn("voice: disconnected by server")
			after := e.detachLocked()
			e.st.Status, e.st.Error = StatusError, "disconnected from the voice channel"
			return after
		}
		e.st.ChannelID = channelID
		return nil
	})
}

// stateUpdate is disgo's StateUpdateFunc: op 4 over the main gateway.
func (e *Engine) stateUpdate(ctx context.Context, guildID snowflake.ID, channelID *snowflake.ID, mute, deaf bool) error {
	cmd := &gateway.UpdateVoiceStateCommand{GuildID: discord.GuildID(guildID), SelfMute: mute, SelfDeaf: deaf}
	if channelID != nil {
		cmd.ChannelID = discord.ChannelID(*channelID)
	}
	gw := e.n.Gateway()
	if gw == nil {
		return errors.New("voice: main gateway not connected")
	}
	e.log.Info("voice: op 4", "guild", cmd.GuildID, "channel", cmd.ChannelID, "self_mute", mute, "self_deaf", deaf)
	return gw.Send(ctx, cmd)
}

// toVoiceState converts arikawa → disgo; ChannelID 0 → nil (left).
func toVoiceState(ev *gateway.VoiceStateUpdateEvent) dgateway.EventVoiceStateUpdate {
	u := dgateway.EventVoiceStateUpdate{VoiceState: ddiscord.VoiceState{
		GuildID: snowflake.ID(ev.GuildID), UserID: snowflake.ID(ev.UserID), SessionID: ev.SessionID,
		GuildDeaf: ev.Deaf, GuildMute: ev.Mute, SelfDeaf: ev.SelfDeaf, SelfMute: ev.SelfMute,
		SelfStream: ev.SelfStream, SelfVideo: ev.SelfVideo, Suppress: ev.Suppress,
	}}
	if ev.ChannelID.IsValid() {
		id := snowflake.ID(ev.ChannelID)
		u.ChannelID = &id
	}
	return u
}

// toVoiceServer converts arikawa → disgo; Endpoint "" → nil (no server yet).
func toVoiceServer(ev *gateway.VoiceServerUpdateEvent) dgateway.EventVoiceServerUpdate {
	u := dgateway.EventVoiceServerUpdate{Token: ev.Token, GuildID: snowflake.ID(ev.GuildID)}
	if ev.Endpoint != "" {
		endpoint := ev.Endpoint
		u.Endpoint = &endpoint
	}
	return u
}
