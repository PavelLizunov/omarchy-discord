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

// memberDebounce coalesces bursts of GUILD_MEMBER_LIST_UPDATE ops into one
// member_list_update per channel.
const memberDebounce = 100 * time.Millisecond

// gatewaySendTimeout bounds one Op 14 write so a stalled gateway cannot pin a
// goroutine for the life of the process.
const gatewaySendTimeout = 10 * time.Second

// memberTracker is the manager's view of which users are shown where, so
// presence updates can be routed to the clients that display them and nothing
// else. It also owns the subscription registry: socket subscriptions are
// per connection, but the gateway request and the per-channel state behind
// them are shared, so they are refcounted here by subscribing client.
type memberTracker struct {
	mu sync.Mutex
	// pending holds the debounce timer per channel with a re-emit queued.
	pending map[discord.ChannelID]*time.Timer
	// users is the user set of the last emitted list per channel.
	users map[discord.ChannelID]map[discord.UserID]struct{}
	// dmUsers indexes private channels by recipient.
	dmUsers map[discord.UserID][]discord.ChannelID
	// subs holds the clients subscribed to each channel's member list; the
	// channel's state lives exactly as long as this set is non-empty. A nil
	// client (Handle called without a socket connection, i.e. tests) is a
	// valid key that only unsubscribe_members can remove.
	subs map[discord.ChannelID]map[socket.Client]struct{}
	// sent is the channel set last asked for per guild (Op 14), so a
	// re-subscribe of an already-requested set sends no gateway command.
	sent map[discord.GuildID]string
}

// reset drops everything, including the subscription registry: it is called
// when a *new* session is installed, whose lists belong to another account.
func (t *memberTracker) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopPendingLocked()
	t.users = map[discord.ChannelID]map[discord.UserID]struct{}{}
	t.dmUsers = map[discord.UserID][]discord.ChannelID{}
	t.subs = map[discord.ChannelID]map[socket.Client]struct{}{}
	t.sent = map[discord.GuildID]string{}
}

// resetLists drops the cached lists and the record of what was asked for, but
// keeps the subscriptions: a re-IDENTIFY wipes ningen's member lists, so
// everything must be requested and re-emitted again for the same subscribers.
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

// subscribe registers c as a subscriber of chID.
func (t *memberTracker) subscribe(chID discord.ChannelID, c socket.Client) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.subs[chID] == nil {
		t.subs[chID] = map[socket.Client]struct{}{}
	}
	t.subs[chID][c] = struct{}{}
}

// unsubscribe removes c; last reports that nobody displays chID any more, in
// which case its per-channel state is dropped with it.
func (t *memberTracker) unsubscribe(chID discord.ChannelID, c socket.Client) (last bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.releaseLocked(chID, c)
}

// dropClient releases every subscription of c (its connection ended) and
// returns the channels that lost their last subscriber.
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

// releaseLocked drops one subscriber and, when it was the last, forgets the
// channel's list state so nothing grows with every channel ever visited.
// Caller holds mu.
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

// subscribed lists every channel some client currently displays.
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

// markSent records key as the channel set requested for guildID; false means
// it is unchanged and no gateway command is needed.
func (t *memberTracker) markSent(guildID discord.GuildID, key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.sent[guildID] == key {
		return false
	}
	t.sent[guildID] = key
	return true
}

// clearSent forgets what was requested for guildID, so the next subscribe
// asks again (used when the gateway command failed).
func (t *memberTracker) clearSent(guildID discord.GuildID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.sent, guildID)
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

// memberListID computes the id Discord stamps on a channel's member list
// (GUILD_MEMBER_LIST_UPDATE.id), derived empirically against the live
// gateway (32 channels, 5 guilds): the list is "everyone" when the
// @everyone role has View Channel at guild level and no overwrite denies it;
// otherwise it is murmur3-32 of "allow:<id>,…,deny:<id>,…" with the allow
// and deny overwrite ids each sorted as strings (the client sorts in JS).
// ningen's ComputeListID keeps payload order and ignores the @everyone
// rule, so it matches only channels whose overwrites happen to arrive
// sorted — it must not be used for lookups.
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

// listChannel resolves the channel whose overwrites define the member list to
// display. A thread carries no overwrites of its own, so taking its (empty)
// set would compute the guild's public "everyone" list even for a thread under
// a private channel; Discord serves the parent's list, which is also the one
// to subscribe to.
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

// guildMemberList renders ningen's kept list for a guild channel. ok is false
// when ningen holds no list for the channel yet.
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

// listedChannels returns the subscribed channels of guildID whose member list
// is listID — the channels a GUILD_MEMBER_LIST_UPDATE with that id must be
// re-emitted for. It reads the manager's own registry: ningen's chunk state is
// not consulted (nothing is requested through it any more).
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

// memberRanges is the Op 14 `channels` map for guildID: the first range of the
// member list of every channel some client currently displays, threads mapped
// to the parent whose list Discord actually serves.
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

// rangesKey identifies a channel set for the send-once check.
func rangesKey(ranges map[discord.ChannelID][][2]int) string {
	ids := make([]string, 0, len(ranges))
	for id := range ranges {
		ids = append(ids, id.String())
	}
	sort.Strings(ids)
	return strings.Join(ids, ",")
}

// requestMemberLists sends the Op 14 GUILD_SUBSCRIBE that asks Discord for the
// first range of every member list the guild's subscribers display — exactly
// what the official client sends on channel open. Discord answers with a
// GUILD_MEMBER_LIST_UPDATE per list (and nothing at all for a range it already
// sent, which is why an existing list is re-emitted from the cache instead).
// The command is sent only when the displayed set actually changed: the gateway
// has a shared send budget (arikawa's limiter, 120 commands/min) and a client
// that re-subscribes on every channel change would otherwise burn it.
//
// ningen's RequestMemberList is deliberately not used: its chunk arithmetic
// panics ("makeslice: cap out of range") as soon as it already holds the list
// for the computed id and that list has fewer than 100 visible members — which
// is every second channel of a small guild.
func (m *Manager) requestMemberLists(n *ningen.State, guildID discord.GuildID) {
	if !guildID.IsValid() {
		return
	}
	ranges := m.memberRanges(n.Offline(), guildID)
	if len(ranges) == 0 {
		return // nothing displayed: the existing subscription is left alone
	}
	if !m.members.markSent(guildID, rangesKey(ranges)) {
		return // Discord already has exactly this set
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
			// Forget the request so the next subscribe retries it.
			m.members.clearSent(guildID)
			redact.Logf("session: member list subscribe for guild %s: %v", guildID, err)
		}
	})
}

// ClientClosed implements socket.ClientCloser: a dropped connection releases
// its member subscriptions, and a channel nobody displays any more loses its
// tracked users, its pending re-emit, and its place in the next Op 14. The
// gateway-side subscription is left as it is until the set is next sent.
func (m *Manager) ClientClosed(c socket.Client) {
	m.members.dropClient(c)
}

// installMemberHandlers wires member-list and presence gateway events.
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
			return // a RESUME keeps ningen's member lists
		}
		// A re-IDENTIFY wiped ningen's lists and its own request state: ask
		// again for every channel a client still displays, and drop the
		// tracked users so presence routing cannot use pre-reconnect data.
		m.members.resetLists()
		off := n.Offline()
		guilds := map[discord.GuildID]struct{}{}
		for _, chID := range m.members.subscribed() {
			ch, err := off.Cabinet.Channel(chID)
			if err != nil {
				continue
			}
			if !ch.GuildID.IsValid() {
				m.scheduleMemberList(n, chID) // DM lists are synthesized
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
	c := socket.ClientFromContext(ctx)
	if c != nil {
		c.SubscribeMembers(p.ChannelID)
	}
	m.members.subscribe(chID, c)
	if ch.GuildID.IsValid() {
		// Op 14 with a channel range, exactly what the official client sends
		// on channel open. The list arrives as GUILD_MEMBER_LIST_UPDATE; a
		// list we already hold is re-emitted at once because Discord does
		// not resend an unchanged range.
		_, missing := n.MemberState.GetMemberListDirect(ch.GuildID, memberListID(off, listChannel(off, ch)))
		if missing != nil {
			// Nothing to show yet: ask even if this exact set was asked for
			// before (the earlier request may have been lost to a reconnect).
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

// unsubscribeMembers implements the unsubscribe_members command; it is
// idempotent. When the last subscriber of a channel goes away its tracked
// users and pending re-emit are dropped, so nothing accumulates across a
// session's worth of channel switching; the channel simply stops appearing in
// the guild's Op 14 the next time one is sent (no command is sent for an
// unsubscribe — the gateway budget is better spent on lists we display).
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
