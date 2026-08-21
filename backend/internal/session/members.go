package session

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/ningen/v3"
	"github.com/diamondburned/ningen/v3/states/member"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/socket"
)

// memberDebounce coalesces bursts of GUILD_MEMBER_LIST_UPDATE ops into one
// member_list_update per channel.
const memberDebounce = 100 * time.Millisecond

// memberTracker is the manager's view of which users are shown where, so
// presence updates can be routed to the clients that display them and nothing
// else.
type memberTracker struct {
	mu sync.Mutex
	// pending holds the debounce timer per channel with a re-emit queued.
	pending map[discord.ChannelID]*time.Timer
	// users is the user set of the last emitted list per channel.
	users map[discord.ChannelID]map[discord.UserID]struct{}
	// dmUsers indexes private channels by recipient.
	dmUsers map[discord.UserID][]discord.ChannelID
}

func (t *memberTracker) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, tm := range t.pending {
		tm.Stop()
	}
	t.pending = map[discord.ChannelID]*time.Timer{}
	t.users = map[discord.ChannelID]map[discord.UserID]struct{}{}
	t.dmUsers = map[discord.UserID][]discord.ChannelID{}
}

// refreshDMIndex rebuilds the recipient → DM index from the cache.
func (t *memberTracker) refreshDMIndex(n *ningen.State) {
	idx := map[discord.UserID][]discord.ChannelID{}
	if chs, err := n.Cabinet.PrivateChannels(); err == nil {
		for _, ch := range chs {
			for _, u := range ch.DMRecipients {
				idx[u.ID] = append(idx[u.ID], ch.ID)
			}
		}
	}
	t.mu.Lock()
	t.dmUsers = idx
	t.mu.Unlock()
}

// channelsFor returns the DM channels (open-set keys) and member-list
// channels (member-sub keys) that currently display the user.
func (t *memberTracker) channelsFor(uid discord.UserID) (dms, lists []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, id := range t.dmUsers[uid] {
		dms = append(dms, id.String())
	}
	for chID, users := range t.users {
		if _, ok := users[uid]; ok {
			lists = append(lists, chID.String())
		}
	}
	sort.Strings(lists)
	return dms, lists
}

func presenceStatus(s discord.Status) string {
	switch s {
	case discord.OnlineStatus, discord.IdleStatus, discord.DoNotDisturbStatus:
		return string(s)
	}
	return "offline"
}

// activityLine renders the first activity the way the official client does.
func activityLine(acts []discord.Activity) string {
	for _, a := range acts {
		switch a.Type {
		case discord.CustomActivity:
			parts := []string{}
			if a.Emoji != nil && a.Emoji.Name != "" && !a.Emoji.ID.IsValid() {
				parts = append(parts, a.Emoji.Name)
			}
			if a.State != "" {
				parts = append(parts, a.State)
			}
			if s := strings.Join(parts, " "); s != "" {
				return s
			}
		case discord.GameActivity:
			return "Playing " + a.Name
		case discord.StreamingActivity:
			return "Streaming " + a.Name
		case discord.ListeningActivity:
			return "Listening to " + a.Name
		case discord.WatchingActivity:
			return "Watching " + a.Name
		case discord.CompetingActivity:
			return "Competing in " + a.Name
		}
	}
	return ""
}

func wireMember(n *ningen.State, guildID discord.GuildID, u discord.User, nick, groupID string, p *discord.Presence) protocol.Member {
	name := u.DisplayOrUsername()
	if nick != "" {
		name = nick
	}
	status, activity := "offline", ""
	if p != nil {
		status, activity = presenceStatus(p.Status), activityLine(p.Activities)
	}
	return protocol.Member{
		User:     protocol.MessageAuthor{ID: u.ID.String(), Username: u.Username, DisplayName: name, AvatarURL: sizedURL(u.AvatarURL(), avatarSize), Bot: u.Bot},
		GroupID:  groupID,
		Status:   status,
		Activity: activity,
	}
}

func groupName(n *ningen.State, guildID discord.GuildID, id string) string {
	switch id {
	case "online":
		return "Online"
	case "offline":
		return "Offline"
	}
	if sf, err := discord.ParseSnowflake(id); err == nil {
		if r, err := n.Cabinet.Role(guildID, discord.RoleID(sf)); err == nil {
			return r.Name
		}
	}
	return id
}

// guildMemberList renders ningen's kept list for a guild channel. ok is false
// when ningen holds no list for the channel yet.
func guildMemberList(n *ningen.State, ch *discord.Channel) (groups []protocol.MemberGroup, members []protocol.Member, ok bool) {
	list, err := n.MemberState.GetMemberList(ch.GuildID, ch.ID)
	if err != nil {
		return nil, nil, false
	}
	list.ViewGroups(func(gs []gateway.GuildMemberListGroup) {
		for _, g := range gs {
			groups = append(groups, protocol.MemberGroup{ID: g.ID, Name: groupName(n, ch.GuildID, g.ID), Count: int(g.Count)})
		}
	})
	list.ViewItems(func(items []gateway.GuildMemberListOpItem) {
		group := ""
		for _, it := range items {
			switch {
			case it.Group != nil:
				group = it.Group.ID
			case it.Member != nil:
				p := it.Member.Presence
				members = append(members, wireMember(n, ch.GuildID, it.Member.User, it.Member.Nick, group, &p))
			}
		}
	})
	return groups, members, true
}

// dmMemberList synthesizes a list for a DM / group DM from its recipients and
// the global presence store; no gateway request is involved.
func dmMemberList(n *ningen.State, ch *discord.Channel) ([]protocol.MemberGroup, []protocol.Member) {
	var online, offline []protocol.Member
	for _, u := range ch.DMRecipients {
		p, _ := n.PresenceStore.Presence(0, u.ID)
		mem := wireMember(n, 0, u, "", "offline", p)
		if mem.Status != "offline" {
			mem.GroupID = "online"
			online = append(online, mem)
		} else {
			offline = append(offline, mem)
		}
	}
	byName := func(ms []protocol.Member) {
		sort.SliceStable(ms, func(i, j int) bool {
			return strings.ToLower(ms[i].User.DisplayName) < strings.ToLower(ms[j].User.DisplayName)
		})
	}
	byName(online)
	byName(offline)
	groups := []protocol.MemberGroup{}
	if len(online) > 0 {
		groups = append(groups, protocol.MemberGroup{ID: "online", Name: "Online", Count: len(online)})
	}
	if len(offline) > 0 {
		groups = append(groups, protocol.MemberGroup{ID: "offline", Name: "Offline", Count: len(offline)})
	}
	return groups, append(online, offline...)
}

// MemberList builds the member_list_update payload for a channel. ok is false
// when nothing can be said yet (guild list not received).
func MemberList(n *ningen.State, ch *discord.Channel) (ev protocol.MemberListUpdateEvent, ok bool) {
	var groups []protocol.MemberGroup
	var members []protocol.Member
	if ch.GuildID.IsValid() {
		groups, members, ok = guildMemberList(n, ch)
		if !ok {
			return ev, false
		}
	} else {
		groups, members = dmMemberList(n, ch)
	}
	return protocol.NewMemberListUpdate(ch.ID.String(), optSnowflake(discord.Snowflake(ch.GuildID)), groups, members), true
}

// emitMemberList pushes the current list for chID to its subscribers and
// records the displayed users for presence routing.
func (m *Manager) emitMemberList(n *ningen.State, chID discord.ChannelID) {
	off := n.Offline()
	ch, err := off.Cabinet.Channel(chID)
	if err != nil {
		return
	}
	ev, ok := MemberList(off, ch)
	if !ok {
		return
	}
	users := make(map[discord.UserID]struct{}, len(ev.Members))
	for _, mem := range ev.Members {
		if sf, err := discord.ParseSnowflake(mem.User.ID); err == nil {
			users[discord.UserID(sf)] = struct{}{}
		}
	}
	m.members.mu.Lock()
	m.members.users[chID] = users
	m.members.mu.Unlock()
	m.push(socket.Routed{Members: []string{chID.String()}, Event: ev})
}

// scheduleMemberList queues a debounced re-emit for chID.
func (m *Manager) scheduleMemberList(n *ningen.State, chID discord.ChannelID) {
	m.members.mu.Lock()
	defer m.members.mu.Unlock()
	if _, queued := m.members.pending[chID]; queued {
		return
	}
	m.members.pending[chID] = time.AfterFunc(m.memberDebounce, func() {
		m.members.mu.Lock()
		delete(m.members.pending, chID)
		m.members.mu.Unlock()
		m.mu.Lock()
		live := m.n == n
		m.mu.Unlock()
		if live {
			m.emitMemberList(n, chID)
		}
	})
}

// requestedChannels lists the guild channels that share list ID listID and
// have had a member-list chunk requested through ningen — i.e. the channels
// some client subscribed.
func requestedChannels(n *ningen.State, guildID discord.GuildID, listID string) []discord.ChannelID {
	chs, err := n.Cabinet.Channels(guildID)
	if err != nil {
		return nil
	}
	var out []discord.ChannelID
	for _, ch := range chs {
		if n.MemberState.GetMemberListChunk(guildID, ch.ID) < 0 {
			continue
		}
		if member.ComputeListID(ch.Overwrites) == listID {
			out = append(out, ch.ID)
		}
	}
	return out
}

// installMemberHandlers wires member-list and presence gateway events.
func (m *Manager) installMemberHandlers(n *ningen.State) {
	m.members.reset()
	live := func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		return m.n == n && m.everReady
	}
	n.AddSyncHandler(func(*ningen.ConnectedEvent) {
		if live() {
			m.members.refreshDMIndex(n.Offline())
		}
	})
	dmChanged := func(ch discord.Channel) {
		if !ch.GuildID.IsValid() && live() {
			m.members.refreshDMIndex(n.Offline())
		}
	}
	n.AddSyncHandler(func(ev *gateway.ChannelCreateEvent) { dmChanged(ev.Channel) })
	n.AddSyncHandler(func(ev *gateway.ChannelUpdateEvent) { dmChanged(ev.Channel) })
	n.AddSyncHandler(func(ev *gateway.ChannelDeleteEvent) { dmChanged(ev.Channel) })
	n.AddSyncHandler(func(ev *gateway.GuildMemberListUpdateEvent) {
		if !live() {
			return
		}
		for _, chID := range requestedChannels(n.Offline(), ev.GuildID, ev.ID) {
			m.scheduleMemberList(n, chID)
		}
	})
	n.AddSyncHandler(func(ev *gateway.PresenceUpdateEvent) {
		if !live() {
			return
		}
		dms, lists := m.members.channelsFor(ev.User.ID)
		if len(dms) == 0 && len(lists) == 0 {
			return
		}
		m.push(socket.Routed{Open: dms, Members: lists, Event: protocol.NewPresenceUpdate(
			ev.User.ID.String(), presenceStatus(ev.Status), activityLine(ev.Activities))})
	})
}

// subscribeMembers implements the subscribe_members command.
func (m *Manager) subscribeMembers(ctx context.Context, req *protocol.Request) (any, *protocol.Error) {
	var p protocol.SubscribeMembersParams
	if e := req.Params(&p); e != nil {
		return nil, e
	}
	sf, e := parseSnowflake(p.ChannelID, "channel_id")
	if e != nil {
		return nil, e
	}
	chID := discord.ChannelID(sf)
	n, e := m.cachedSession()
	if e != nil {
		return nil, e
	}
	off := n.Offline()
	ch, err := off.Cabinet.Channel(chID)
	if err != nil {
		return nil, protocol.Errorf(protocol.CodeUnknownChannel, "channel %s is not visible to this account", p.ChannelID)
	}
	if ch.GuildID.IsValid() && !off.HasPermissions(chID, discord.PermissionViewChannel) {
		return nil, protocol.Errorf(protocol.CodeForbidden, "no permission to view channel %s", p.ChannelID)
	}
	if c := socket.ClientFromContext(ctx); c != nil {
		c.SubscribeMembers(p.ChannelID)
	}
	if ch.GuildID.IsValid() {
		// Op 14 with a channel range, exactly what the official client sends
		// on channel open. The list arrives as GUILD_MEMBER_LIST_UPDATE; a
		// list ningen already holds is re-emitted at once because Discord
		// does not resend an unchanged range.
		n.MemberState.RequestMemberList(ch.GuildID, chID, 0)
		if _, err := n.MemberState.GetMemberList(ch.GuildID, chID); err != nil {
			return protocol.EmptyResult{}, nil
		}
	}
	m.scheduleMemberList(n, chID)
	return protocol.EmptyResult{}, nil
}

// unsubscribeMembers implements the unsubscribe_members command; it is
// idempotent (the gateway subscription is retained like the guild subscribe).
func (m *Manager) unsubscribeMembers(ctx context.Context, req *protocol.Request) (any, *protocol.Error) {
	var p protocol.SubscribeMembersParams
	if e := req.Params(&p); e != nil {
		return nil, e
	}
	if _, e := parseSnowflake(p.ChannelID, "channel_id"); e != nil {
		return nil, e
	}
	if c := socket.ClientFromContext(ctx); c != nil {
		c.UnsubscribeMembers(p.ChannelID)
	}
	return protocol.EmptyResult{}, nil
}
