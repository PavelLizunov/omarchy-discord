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
			panics.Go("voice: self state", func() { e.onSelfState(gen, ev.GuildID, ev.ChannelID) })
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

func (e *Engine) onSelfState(gen uint64, guildID discord.GuildID, channelID discord.ChannelID) {
	e.update(func() func() {
		if gen != e.gen || guildID != e.st.GuildID || (e.st.Status != StatusConnecting && e.st.Status != StatusConnected) {
			return nil
		}
		if !channelID.IsValid() {
			if e.st.Status == StatusConnecting {
				return nil
			}
			e.log.Warn("voice: disconnected by server")
			after := e.detachLocked()
			e.st.Status, e.st.Error = StatusError, "disconnected from the voice channel"
			return after
		}
		e.st.ChannelID = channelID
		return nil
	})
}

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

func toVoiceServer(ev *gateway.VoiceServerUpdateEvent) dgateway.EventVoiceServerUpdate {
	u := dgateway.EventVoiceServerUpdate{Token: ev.Token, GuildID: snowflake.ID(ev.GuildID)}
	if ev.Endpoint != "" {
		endpoint := ev.Endpoint
		u.Endpoint = &endpoint
	}
	return u
}
