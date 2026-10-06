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

	"github.com/mattcalayo/omarchy-discord/backend/internal/panics"
	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/redact"
	"github.com/mattcalayo/omarchy-discord/backend/internal/socket"
)

const memberDebounce = 100 * time.Millisecond

const gatewaySendTimeout = 10 * time.Second

type memberTracker struct {
	mu      sync.Mutex
	pending map[discord.ChannelID]*time.Timer
	users   map[discord.ChannelID]map[discord.UserID]struct{}
	dmUsers map[discord.UserID][]discord.ChannelID
	subs    map[discord.ChannelID]map[socket.Client]struct{}
	sent    map[discord.GuildID]string
}

func (t *memberTracker) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopPendingLocked()
	t.users = map[discord.ChannelID]map[discord.UserID]struct{}{}
	t.dmUsers = map[discord.UserID][]discord.ChannelID{}
	t.subs = map[discord.ChannelID]map[socket.Client]struct{}{}
	t.sent = map[discord.GuildID]string{}
}

func (t *memberTracker) resetLists() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopPendingLocked()
	t.users = map[discord.ChannelID]map[discord.UserID]struct{}{}
	t.sent = map[discord.GuildID]string{}
}

func (t *memberTracker) stopPendingLocked() {
	for _, tm := range t.pending {
		tm.Stop()
	}
	t.pending = map[discord.ChannelID]*time.Timer{}
}

func (t *memberTracker) subscribe(chID discord.ChannelID, c socket.Client) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.subs[chID] == nil {
		t.subs[chID] = map[socket.Client]struct{}{}
	}
	t.subs[chID][c] = struct{}{}
}

func (t *memberTracker) unsubscribe(chID discord.ChannelID, c socket.Client) (last bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.releaseLocked(chID, c)
}

func (t *memberTracker) dropClient(c socket.Client) []discord.ChannelID {
	t.mu.Lock()
	defer t.mu.Unlock()
	var gone []discord.ChannelID
	for chID := range t.subs {
		if t.releaseLocked(chID, c) {
			gone = append(gone, chID)
		}
	}
	return gone
}

func (t *memberTracker) releaseLocked(chID discord.ChannelID, c socket.Client) bool {
	subs, ok := t.subs[chID]
	if !ok {
		return false
	}
	if _, ok := subs[c]; !ok {
		return false
	}
	delete(subs, c)
	if len(subs) > 0 {
		return false
	}
	delete(t.subs, chID)
	delete(t.users, chID)
	if tm, ok := t.pending[chID]; ok {
		tm.Stop()
		delete(t.pending, chID)
	}
	return true
}

func (t *memberTracker) subscribed() []discord.ChannelID {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]discord.ChannelID, 0, len(t.subs))
	for chID := range t.subs {
		out = append(out, chID)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (t *memberTracker) markSent(guildID discord.GuildID, key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.sent[guildID] == key {
		return false
	}
	t.sent[guildID] = key
	return true
}

func (t *memberTracker) clearSent(guildID discord.GuildID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.sent, guildID)
}

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

func memberListID(n *ningen.State, ch *discord.Channel) string {
	var allows, denies []discord.Snowflake
	for _, ow := range ch.Overwrites {
		switch {
		case ow.Allow.Has(discord.PermissionViewChannel):
			allows = append(allows, ow.ID)
		case ow.Deny.Has(discord.PermissionViewChannel):
			denies = append(denies, ow.ID)
		}
	}
	if len(denies) == 0 {
		if everyone, err := n.Cabinet.Role(ch.GuildID, discord.RoleID(ch.GuildID)); err == nil && everyone.Permissions.Has(discord.PermissionViewChannel) {
			return "everyone"
		}
		if len(allows) == 0 {
			return "everyone"
		}
	}
	sort.Slice(allows, func(i, j int) bool { return allows[i].String() < allows[j].String() })
	sort.Slice(denies, func(i, j int) bool { return denies[i].String() < denies[j].String() })
	var sorted []discord.Overwrite
	for _, id := range allows {
		sorted = append(sorted, discord.Overwrite{ID: id, Allow: discord.PermissionViewChannel})
	}
	for _, id := range denies {
		sorted = append(sorted, discord.Overwrite{ID: id, Deny: discord.PermissionViewChannel})
	}
	return member.ComputeListID(sorted)
}

func listChannel(n *ningen.State, ch *discord.Channel) *discord.Channel {
	if !isThread(ch.Type) || !ch.ParentID.IsValid() {
		return ch
	}
	parent, err := n.Cabinet.Channel(ch.ParentID)
	if err != nil {
		return ch
	}
	return parent
}

func guildMemberList(n *ningen.State, ch *discord.Channel) (groups []protocol.MemberGroup, members []protocol.Member, ok bool) {
	list, err := n.MemberState.GetMemberListDirect(ch.GuildID, memberListID(n, listChannel(n, ch)))
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

func (m *Manager) scheduleMemberList(n *ningen.State, chID discord.ChannelID) {
	m.members.mu.Lock()
	defer m.members.mu.Unlock()
	if _, queued := m.members.pending[chID]; queued {
		return
	}
	m.members.pending[chID] = time.AfterFunc(m.memberDebounce, func() {
		defer panics.Recover("session: member list debounce")
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

func (m *Manager) listedChannels(n *ningen.State, guildID discord.GuildID, listID string) []discord.ChannelID {
	var out []discord.ChannelID
	for _, chID := range m.members.subscribed() {
		ch, err := n.Cabinet.Channel(chID)
		if err != nil || ch.GuildID != guildID {
			continue
		}
		if memberListID(n, listChannel(n, ch)) == listID {
			out = append(out, chID)
		}
	}
	return out
}

func (m *Manager) memberRanges(n *ningen.State, guildID discord.GuildID) map[discord.ChannelID][][2]int {
	ranges := map[discord.ChannelID][][2]int{}
	for _, chID := range m.members.subscribed() {
		ch, err := n.Cabinet.Channel(chID)
		if err != nil || ch.GuildID != guildID {
			continue
		}
		ranges[listChannel(n, ch).ID] = [][2]int{{0, 99}}
	}
	return ranges
}

func rangesKey(ranges map[discord.ChannelID][][2]int) string {
	ids := make([]string, 0, len(ranges))
	for id := range ranges {
		ids = append(ids, id.String())
	}
	sort.Strings(ids)
	return strings.Join(ids, ",")
}

func (m *Manager) requestMemberLists(n *ningen.State, guildID discord.GuildID) {
	if !guildID.IsValid() {
		return
	}
	ranges := m.memberRanges(n.Offline(), guildID)
	if len(ranges) == 0 {
		return
	}
	if !m.members.markSent(guildID, rangesKey(ranges)) {
		return
	}
	panics.Go("session: guild subscribe", func() {
		ctx, cancel := context.WithTimeout(context.Background(), gatewaySendTimeout)
		defer cancel()
		err := n.SendGateway(ctx, &gateway.GuildSubscribeCommand{
			GuildID:    guildID,
			Channels:   ranges,
			Typing:     true,
			Activities: true,
		})
		if err != nil {
			m.members.clearSent(guildID)
			redact.Logf("session: member list subscribe for guild %s: %v", guildID, err)
		}
	})
}

func (m *Manager) ClientClosed(c socket.Client) {
	m.members.dropClient(c)
}

func (m *Manager) installMemberHandlers(n *ningen.State) {
	m.members.reset()
	live := func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		return m.n == n && m.everReady
	}
	addSyncHandler(n, "member_connected", func(ev *ningen.ConnectedEvent) {
		if !live() {
			return
		}
		m.members.refreshDMIndex(n.Offline())
		if _, ready := ev.Event.(*gateway.ReadyEvent); !ready {
			return
		}
		m.members.resetLists()
		off := n.Offline()
		guilds := map[discord.GuildID]struct{}{}
		for _, chID := range m.members.subscribed() {
			ch, err := off.Cabinet.Channel(chID)
			if err != nil {
				continue
			}
			if !ch.GuildID.IsValid() {
				m.scheduleMemberList(n, chID)
				continue
			}
			guilds[ch.GuildID] = struct{}{}
		}
		for guildID := range guilds {
			m.requestMemberLists(n, guildID)
		}
	})
	dmChanged := func(ch discord.Channel) {
		if !ch.GuildID.IsValid() && live() {
			m.members.refreshDMIndex(n.Offline())
		}
	}
	addSyncHandler(n, "member_channel_create", func(ev *gateway.ChannelCreateEvent) { dmChanged(ev.Channel) })
	addSyncHandler(n, "member_channel_update", func(ev *gateway.ChannelUpdateEvent) { dmChanged(ev.Channel) })
	addSyncHandler(n, "member_channel_delete", func(ev *gateway.ChannelDeleteEvent) { dmChanged(ev.Channel) })
	addSyncHandler(n, "member_list_update", func(ev *gateway.GuildMemberListUpdateEvent) {
		if !live() {
			return
		}
		for _, chID := range m.listedChannels(n.Offline(), ev.GuildID, ev.ID) {
			m.scheduleMemberList(n, chID)
		}
	})
	addSyncHandler(n, "presence_update", func(ev *gateway.PresenceUpdateEvent) {
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
	c := socket.ClientFromContext(ctx)
	if c != nil {
		c.SubscribeMembers(p.ChannelID)
	}
	m.members.subscribe(chID, c)
	if ch.GuildID.IsValid() {
		_, missing := n.MemberState.GetMemberListDirect(ch.GuildID, memberListID(off, listChannel(off, ch)))
		if missing != nil {
			m.members.clearSent(ch.GuildID)
		}
		m.requestMemberLists(n, ch.GuildID)
		if missing != nil {
			return protocol.EmptyResult{}, nil
		}
	}
	m.scheduleMemberList(n, chID)
	return protocol.EmptyResult{}, nil
}

func (m *Manager) unsubscribeMembers(ctx context.Context, req *protocol.Request) (any, *protocol.Error) {
	var p protocol.SubscribeMembersParams
	if e := req.Params(&p); e != nil {
		return nil, e
	}
	sf, e := parseSnowflake(p.ChannelID, "channel_id")
	if e != nil {
		return nil, e
	}
	c := socket.ClientFromContext(ctx)
	if c != nil {
		c.UnsubscribeMembers(p.ChannelID)
	}
	m.members.unsubscribe(discord.ChannelID(sf), c)
	return protocol.EmptyResult{}, nil
}
