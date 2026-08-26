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

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/redact"
)

// voiceStatus and voiceState mirror voice.Status / voice.State. They are
// duplicated here so this package does not depend on internal/voice; the
// integration that wires the real engine in swaps these for the real types.
type voiceStatus string

const (
	voiceStatusIdle       voiceStatus = "idle"
	voiceStatusConnecting voiceStatus = "connecting"
	voiceStatusConnected  voiceStatus = "connected"
	voiceStatusError      voiceStatus = "error"
)

type voiceState struct {
	Status    voiceStatus
	GuildID   discord.GuildID   // 0 when idle
	ChannelID discord.ChannelID // 0 when idle
	Muted     bool
	Deafened  bool
	Error     string // human-readable, "" unless Status is voiceStatusError
}

// voiceEngine is the one call the manager drives. Nil means voice is not
// available in this build/session and every voice command is refused.
type voiceEngine interface {
	Join(ctx context.Context, guildID discord.GuildID, channelID discord.ChannelID) error
	Leave(ctx context.Context) error
	SetMute(ctx context.Context, muted bool) error
	SetDeaf(ctx context.Context, deafened bool) error
	State() voiceState
	Close()
}

// voiceEvents are the engine's callbacks (voice.Events). They may run on any
// goroutine; both funnel into the manager's single writer.
type voiceEvents struct {
	State    func(voiceState)
	Speaking func(userID discord.UserID, speaking bool)
}

// idleVoice is the state of a session that is not in a call.
var idleVoice = voiceState{Status: voiceStatusIdle}

// wireVoice renders the engine state for protocol.State.
func wireVoice(v voiceState) protocol.VoiceState {
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

// onVoiceState is the engine's State callback: it records the new state and
// emits a state_changed with a fresh generation when anything changed.
func (m *Manager) onVoiceState(v voiceState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.voiceState == v {
		return
	}
	m.voiceState = v
	m.bump()
}

// onVoiceSpeaking is the engine's Speaking callback.
func (m *Manager) onVoiceSpeaking(userID discord.UserID, speaking bool) {
	m.push(protocol.NewVoiceSpeaking(userID.String(), speaking))
}

// liveVoice returns the engine, or the refusal every voice command answers
// when the daemon has no engine (build without audio support, no session).
func (m *Manager) liveVoice() (voiceEngine, *protocol.Error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.voice == nil {
		return nil, protocol.Errorf(protocol.CodeGatewayUnavailable, "voice is not available in this session")
	}
	return m.voice, nil
}

// voiceJoin implements voice_join.
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
	ch, err := n.Offline().Cabinet.Channel(chID)
	if err != nil || ch.GuildID != guildID {
		return nil, protocol.Errorf(protocol.CodeUnknownChannel, "channel %s is not a channel of guild %s", p.ChannelID, p.GuildID)
	}
	if ch.Type == discord.GuildStageVoice {
		return nil, protocol.Errorf(protocol.CodeInvalidArgument, "stage channels are not supported")
	}
	if ch.Type != discord.GuildVoice {
		return nil, protocol.Errorf(protocol.CodeInvalidArgument, "channel %s is not a voice channel", p.ChannelID)
	}
	if err := v.Join(ctx, guildID, chID); err != nil {
		return nil, protocol.Errorf(protocol.CodeDiscordError, "voice join failed: %v", err)
	}
	return protocol.EmptyResult{}, nil
}

// voiceLeave implements voice_leave. Leaving when idle is a no-op for the
// engine, so it is not an error here either.
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

// voiceSet implements voice_set; absent fields are left unchanged and an
// empty request is a no-op.
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

// voiceUser renders one occupant. The guild member (nick, avatar) is preferred
// over the user the voice state carries; an uncached user degrades to its id.
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

// VoiceMembers builds the voice_members payload for one guild from the
// cabinet's voice states. Only channels this account can see and that hold at
// least one user appear; channels are ordered by id and occupants by display
// name, so an unchanged guild always renders the same event.
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

// pushVoiceMembersLocked emits one voice_members per guild that currently has
// somebody in voice — the post-guilds_synced seed. Guilds with nobody in voice
// are skipped: their empty list is what a client starts from anyway, and the
// per-guild event is sent as soon as that changes. Caller holds mu.
func (m *Manager) pushVoiceMembersLocked(n *ningen.State) {
	guilds, err := n.Cabinet.Guilds()
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		redact.Logf("session: guilds for voice members: %v", err)
		return
	}
	for _, g := range guilds {
		if ev := VoiceMembers(n, g.ID); len(ev.Channels) > 0 {
			m.push(ev)
		}
	}
}

// installVoiceHandlers keeps voice occupancy in sync: one voice_members for
// the affected guild on every VOICE_STATE_UPDATE. The handler is sync so the
// cabinet already holds the update it reports.
func (m *Manager) installVoiceHandlers(n *ningen.State) {
	addSyncHandler(n, "voice_state_update", func(ev *gateway.VoiceStateUpdateEvent) {
		m.mu.Lock()
		live := m.n == n && m.lifecycle == protocol.LifecycleReady
		m.mu.Unlock()
		if !live || !ev.GuildID.IsValid() {
			return // DM and group-DM calls are out of scope
		}
		m.push(VoiceMembers(n.Offline(), ev.GuildID))
	})
}
