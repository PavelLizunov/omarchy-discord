package session

import (
	"context"
	"time"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
)

type guildActionParams struct {
	GuildID   string `json:"guild_id"`
	Confirmed bool   `json:"confirmed"`
	Muted     *bool  `json:"muted"`
}

type guildCounts struct {
	Online  *uint64 `json:"approximate_presence_count"`
	Members *uint64 `json:"approximate_member_count"`
}

type guildStatsResult struct {
	GuildID     string  `json:"guild_id"`
	OnlineCount uint64  `json:"online_count"`
	MemberCount *uint64 `json:"member_count"`
	Approximate bool    `json:"approximate"`
}

func (m *Manager) guildAction(ctx context.Context, req *protocol.Request) (any, *protocol.Error) {
	var p guildActionParams
	if e := req.Params(&p); e != nil {
		return nil, e
	}
	sf, e := parseSnowflake(p.GuildID, "guild_id")
	if e != nil {
		return nil, e
	}
	if req.Command == "leave_guild" && !p.Confirmed {
		return nil, protocol.Errorf(protocol.CodeInvalidArgument, "leaving a server requires confirmation")
	}
	n, e := m.liveSession()
	if e != nil {
		return nil, e
	}
	id := discord.GuildID(sf)
	g, err := n.Cabinet.Guild(id)
	if err != nil {
		return nil, protocol.Errorf(protocol.CodeUnknownGuild, "server is not in this session")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	switch req.Command {
	case "guild_settings":
		return struct {
			Muted bool `json:"muted"`
		}{n.MutedState.Guild(id, false)}, nil
	case "set_guild_mute":
		if p.Muted == nil {
			return nil, protocol.Errorf(protocol.CodeInvalidArgument, "muted is required")
		}
		if err := m.rest.muteGuild(ctx, n, id, *p.Muted); err != nil {
			return nil, m.restError(n, err, false)
		}
		settings := n.MutedState.GuildSettings(id)
		settings.Muted = *p.Muted
		settings.MuteConfig = nil
		n.State.Session.Handler.Call(&gateway.UserGuildSettingsUpdateEvent{UserGuildSetting: settings})
		return protocol.EmptyResult{}, nil
	case "guild_stats":
		stats, err := m.rest.guildStats(ctx, n, id)
		if err != nil {
			return nil, m.restError(n, err, false)
		}
		if stats == nil || stats.Online == nil {
			return nil, protocol.Errorf(protocol.CodeInternalError, "server counts unavailable")
		}
		return guildStatsResult{GuildID: p.GuildID, OnlineCount: *stats.Online, MemberCount: stats.Members, Approximate: true}, nil
	case "leave_guild":
		me, err := n.Cabinet.Me()
		if err != nil {
			return nil, protocol.Errorf(protocol.CodeInternalError, "current user unavailable")
		}
		if g.OwnerID == me.ID {
			return nil, protocol.Errorf(protocol.CodeInvalidArgument, "server owners must transfer ownership before leaving")
		}
		if err := m.rest.leaveGuild(ctx, n, id); err != nil {
			return nil, m.restError(n, err, false)
		}
		return protocol.EmptyResult{}, nil
	case "mark_guild_read":
		channels, err := n.Channels(id, AllowedChannelTypes)
		if err != nil {
			return nil, protocol.Errorf(protocol.CodeInternalError, "server channels unavailable")
		}
		count := 0
		for _, ch := range channels {
			if count >= 500 {
				return nil, protocol.Errorf(protocol.CodeInvalidArgument, "too many channels; some channels were marked read")
			}
			if !ch.LastMessageID.IsValid() {
				continue
			}
			if e := m.acknowledgeRead(ctx, n, ch.ID, ch.LastMessageID); e != nil {
				return nil, e
			}
			count++
		}
		return struct {
			Channels int `json:"channels"`
		}{count}, nil
	}
	return nil, protocol.Errorf(protocol.CodeUnknownCommand, "unknown server action")
}
