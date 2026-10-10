package session

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/state/store"
	"github.com/diamondburned/ningen/v3"
	dsnowflake "github.com/disgoorg/snowflake/v2"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/redact"
	"github.com/mattcalayo/omarchy-discord/backend/internal/voice"
)

type voiceEngine interface {
	Join(ctx context.Context, guildID discord.GuildID, channelID discord.ChannelID) error
	Leave(ctx context.Context) error
	SetMute(ctx context.Context, muted bool) error
	SetDeaf(ctx context.Context, deafened bool) error
	State() voice.State
	Close()
}

var idleVoice = voice.State{Status: voice.StatusIdle}

func (m *Manager) voiceCameras(ctx context.Context, req *protocol.Request, watch bool) (any, *protocol.Error) {
	v, e := m.liveVoice()
	if e != nil {
		return nil, e
	}
	c, ok := v.(interface {
		Cameras() voice.CameraSnapshot
		WatchCamera(context.Context, dsnowflake.ID, uint64) error
	})
	if !ok {
		return nil, protocol.Errorf(protocol.CodeGatewayUnavailable, "camera viewing unavailable")
	}
	if watch {
		var p struct {
			UserID   string `json:"user_id"`
			Revision uint64 `json:"revision"`
		}
		if e = req.Params(&p); e != nil {
			return nil, e
		}
		var id dsnowflake.ID
		if p.UserID != "" {
			sf, err := parseSnowflake(p.UserID, "user_id")
			if err != nil {
				return nil, err
			}
			id = dsnowflake.ID(sf)
		}
		if err := c.WatchCamera(ctx, id, p.Revision); err != nil {
			return nil, protocol.Errorf(protocol.CodeDiscordError, "%v", err)
		}
	}
	return c.Cameras(), nil
}

func wireVoice(v voice.State) protocol.VoiceState {
	status := string(v.Status)
	if status == "" {
		status = protocol.VoiceIdle
	}
	return protocol.VoiceState{
		Status:    status,
		GuildID:   optSnowflake(discord.Snowflake(v.GuildID)),
		ChannelID: optSnowflake(discord.Snowflake(v.ChannelID)),
		Muted:     v.Muted,
		Deafened:  v.Deafened,
		Error:     redact.Redact(v.Error),
	}
}

func (m *Manager) onVoiceState(v voice.State) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.voiceState == v {
		return
	}
	m.voiceState = v
	m.bump()
}

func (m *Manager) onVoiceSpeaking(userID discord.UserID, speaking bool) {
	m.push(protocol.NewVoiceSpeaking(userID.String(), speaking))
}

func (m *Manager) liveVoice() (voiceEngine, *protocol.Error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.voice == nil {
		return nil, protocol.Errorf(protocol.CodeGatewayUnavailable, "voice is not available in this session")
	}
	return m.voice, nil
}

func (m *Manager) voiceJoin(ctx context.Context, req *protocol.Request) (any, *protocol.Error) {
	var p protocol.VoiceJoinParams
	if e := req.Params(&p); e != nil {
		return nil, e
	}
	gsf, e := parseSnowflake(p.GuildID, "guild_id")
	if e != nil {
		return nil, e
	}
	csf, e := parseSnowflake(p.ChannelID, "channel_id")
	if e != nil {
		return nil, e
	}
	v, e := m.liveVoice()
	if e != nil {
		return nil, e
	}
	n, e := m.cachedSession()
	if e != nil {
		return nil, e
	}
	guildID, chID := discord.GuildID(gsf), discord.ChannelID(csf)
	off := n.Offline()
	ch, err := off.Cabinet.Channel(chID)
	if err != nil || ch.GuildID != guildID {
		return nil, protocol.Errorf(protocol.CodeUnknownChannel, "channel %s is not a channel of guild %s", p.ChannelID, p.GuildID)
	}
	if ch.Type == discord.GuildStageVoice {
		return nil, protocol.Errorf(protocol.CodeInvalidArgument, "stage channels are not supported")
	}
	if ch.Type != discord.GuildVoice {
		return nil, protocol.Errorf(protocol.CodeInvalidArgument, "channel %s is not a voice channel", p.ChannelID)
	}
	if !off.HasPermissions(chID, discord.PermissionViewChannel|discord.PermissionConnect) {
		return nil, protocol.Errorf(protocol.CodeForbidden, "no permission to join channel %s", p.ChannelID)
	}
	if err := v.Join(ctx, guildID, chID); err != nil {
		return nil, protocol.Errorf(protocol.CodeDiscordError, "voice join failed: %v", err)
	}
	return protocol.EmptyResult{}, nil
}

func (m *Manager) voiceLeave(ctx context.Context) (any, *protocol.Error) {
	v, e := m.liveVoice()
	if e != nil {
		return nil, e
	}
	if err := v.Leave(ctx); err != nil {
		return nil, protocol.Errorf(protocol.CodeDiscordError, "voice leave failed: %v", err)
	}
	return protocol.EmptyResult{}, nil
}

func (m *Manager) voiceSet(ctx context.Context, req *protocol.Request) (any, *protocol.Error) {
	var p protocol.VoiceSetParams
	if e := req.Params(&p); e != nil {
		return nil, e
	}
	v, e := m.liveVoice()
	if e != nil {
		return nil, e
	}
	if p.Muted != nil {
		if err := v.SetMute(ctx, *p.Muted); err != nil {
			return nil, protocol.Errorf(protocol.CodeDiscordError, "voice mute failed: %v", err)
		}
	}
	if p.Deafened != nil {
		if err := v.SetDeaf(ctx, *p.Deafened); err != nil {
			return nil, protocol.Errorf(protocol.CodeDiscordError, "voice deafen failed: %v", err)
		}
	}
	return protocol.EmptyResult{}, nil
}

func voiceUser(n *ningen.State, guildID discord.GuildID, vs discord.VoiceState) protocol.User {
	mem := vs.Member
	if mem == nil {
		mem, _ = n.Cabinet.Member(guildID, vs.UserID)
	}
	if mem == nil {
		return protocol.User{ID: vs.UserID.String()}
	}
	u := wireUser(mem.User)
	if mem.Nick != "" {
		u.DisplayName = mem.Nick
	}
	return u
}

func VoiceMembers(n *ningen.State, guildID discord.GuildID) protocol.VoiceMembersEvent {
	states, err := n.Cabinet.VoiceStates(guildID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		redact.Logf("session: voice states for guild %s: %v", guildID, err)
	}
	byChannel := map[discord.ChannelID][]protocol.User{}
	for _, vs := range states {
		if !vs.ChannelID.IsValid() || !n.HasPermissions(vs.ChannelID, discord.PermissionViewChannel) {
			continue
		}
		byChannel[vs.ChannelID] = append(byChannel[vs.ChannelID], voiceUser(n, guildID, vs))
	}
	ids := make([]discord.ChannelID, 0, len(byChannel))
	for chID := range byChannel {
		ids = append(ids, chID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	channels := make([]protocol.VoiceChannelMembers, 0, len(ids))
	for _, chID := range ids {
		users := byChannel[chID]
		sort.Slice(users, func(i, j int) bool {
			a, b := strings.ToLower(users[i].DisplayName), strings.ToLower(users[j].DisplayName)
			if a != b {
				return a < b
			}
			return users[i].ID < users[j].ID
		})
		channels = append(channels, protocol.VoiceChannelMembers{ChannelID: chID.String(), Users: users})
	}
	return protocol.NewVoiceMembers(guildID.String(), channels)
}

func allVoiceMembers(n *ningen.State) []protocol.VoiceMembersEvent {
	guilds, err := n.Cabinet.Guilds()
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		redact.Logf("session: guilds for voice members: %v", err)
		return nil
	}
	evs := make([]protocol.VoiceMembersEvent, 0, len(guilds))
	for _, g := range guilds {
		evs = append(evs, VoiceMembers(n, g.ID))
	}
	return evs
}

func (m *Manager) pushVoiceMembersLocked(n *ningen.State) {
	for _, ev := range allVoiceMembers(n) {
		m.push(ev)
	}
}

func (m *Manager) installVoiceHandlers(n *ningen.State) {
	addSyncHandler(n, "voice_state_update", func(ev *gateway.VoiceStateUpdateEvent) {
		m.mu.Lock()
		live := m.n == n && m.lifecycle == protocol.LifecycleReady
		m.mu.Unlock()
		if !live || !ev.GuildID.IsValid() {
			return
		}
		m.push(VoiceMembers(n.Offline(), ev.GuildID))
	})
}
