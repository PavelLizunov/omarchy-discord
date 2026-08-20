package session

import (
	"errors"
	"sort"
	"strings"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/state/store"
	"github.com/diamondburned/ningen/v3"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
)

// AllowedChannelTypes mirrors dissent's gtkcord.AllowedChannelTypes: what the
// sidebar shows and what counts toward guild unread state.
var AllowedChannelTypes = []discord.ChannelType{
	discord.GuildText,
	discord.GuildCategory,
	discord.GuildPublicThread,
	discord.GuildPrivateThread,
	discord.GuildForum,
	discord.GuildAnnouncement,
	discord.GuildAnnouncementThread,
	discord.GuildVoice,
	discord.GuildStageVoice,
}

var unreadOpts = ningen.UnreadOpts{IncludeMutedCategories: true}

func snowflake(s discord.Snowflake) string { return s.String() }

func optSnowflake(s discord.Snowflake) *string {
	if !s.IsValid() {
		return nil
	}
	v := s.String()
	return &v
}

func unreadString(u ningen.UnreadIndication) string {
	switch u {
	case ningen.ChannelMentioned:
		return protocol.UnreadMentioned
	case ningen.ChannelUnread:
		return protocol.UnreadUnread
	}
	return protocol.UnreadRead
}

func channelType(t discord.ChannelType) string {
	switch t {
	case discord.GuildText:
		return "text"
	case discord.GuildAnnouncement:
		return "announcement"
	case discord.GuildCategory:
		return "category"
	case discord.GuildPublicThread, discord.GuildPrivateThread, discord.GuildAnnouncementThread:
		return "thread"
	case discord.GuildForum:
		return "forum"
	case discord.GuildVoice, discord.GuildStageVoice:
		return "voice"
	case discord.DirectMessage:
		return "dm"
	case discord.GroupDM:
		return "group_dm"
	}
	return "text"
}

func wireUser(u discord.User) protocol.User {
	return protocol.User{
		ID:          snowflake(discord.Snowflake(u.ID)),
		Username:    u.Username,
		DisplayName: u.DisplayOrUsername(),
		AvatarURL:   u.AvatarURL(),
	}
}

// mentionCount reads the per-channel mention count from ningen's read state.
func mentionCount(n *ningen.State, chID discord.ChannelID) int {
	if rs := n.ReadState.ReadState(chID); rs != nil {
		return rs.MentionCount
	}
	return 0
}

// lastReadMessageID reads the account's read marker from ningen's read state;
// nil when the channel has none.
func lastReadMessageID(n *ningen.State, chID discord.ChannelID) *string {
	if rs := n.ReadState.ReadState(chID); rs != nil {
		return optSnowflake(discord.Snowflake(rs.LastMessageID))
	}
	return nil
}

// Guilds lists the account's guilds in the user's configured order (guild
// folders, then legacy positions, then anything unlisted by name).
func Guilds(n *ningen.State) ([]protocol.Guild, error) {
	gs, err := n.Cabinet.Guilds()
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	order := map[discord.GuildID]int{}
	ready := n.Ready()
	if ready.UserSettings != nil {
		for _, f := range ready.UserSettings.GuildFolders {
			for _, id := range f.GuildIDs {
				if _, seen := order[id]; !seen {
					order[id] = len(order)
				}
			}
		}
		for _, id := range ready.UserSettings.GuildPositions {
			if _, seen := order[id]; !seen {
				order[id] = len(order)
			}
		}
	}
	sort.SliceStable(gs, func(i, j int) bool {
		oi, iok := order[gs[i].ID]
		oj, jok := order[gs[j].ID]
		switch {
		case iok && jok:
			return oi < oj
		case iok != jok:
			return iok
		}
		return strings.ToLower(gs[i].Name) < strings.ToLower(gs[j].Name)
	})

	out := make([]protocol.Guild, 0, len(gs))
	for i, g := range gs {
		var icon *string
		if u := g.IconURL(); u != "" {
			icon = &u
		}
		mentions := 0
		if chs, err := n.Cabinet.Channels(g.ID); err == nil {
			for _, ch := range chs {
				mentions += mentionCount(n, ch.ID)
			}
		}
		out = append(out, protocol.Guild{
			ID:           snowflake(discord.Snowflake(g.ID)),
			Name:         g.Name,
			IconURL:      icon,
			Unread:       unreadString(n.GuildIsUnread(g.ID, ningen.GuildUnreadOpts{UnreadOpts: unreadOpts, Types: AllowedChannelTypes})),
			MentionCount: mentions,
			Position:     i,
		})
	}
	return out, nil
}

// ErrUnknownGuild is returned by Channels for a guild not in the session.
var ErrUnknownGuild = errors.New("unknown guild")

// Channels lists a guild's visible channels in category-grouped display order.
func Channels(n *ningen.State, guildID discord.GuildID) ([]protocol.Channel, error) {
	if _, err := n.Cabinet.Guild(guildID); err != nil {
		return nil, ErrUnknownGuild
	}
	chs, err := n.Channels(guildID, AllowedChannelTypes)
	if err != nil {
		return nil, err
	}
	ordered := displayOrder(chs)
	out := make([]protocol.Channel, 0, len(ordered))
	for _, ch := range ordered {
		out = append(out, wireChannel(n, ch))
	}
	return out, nil
}

// displayOrder sorts like the official client: uncategorized channels first,
// then each category (by position) followed by its children; within a group,
// text-like channels precede voice, then by position, then by id.
func displayOrder(chs []discord.Channel) []discord.Channel {
	isCat := map[discord.ChannelID]bool{}
	for _, ch := range chs {
		if ch.Type == discord.GuildCategory {
			isCat[ch.ID] = true
		}
	}
	less := func(a, b discord.Channel) bool {
		av, bv := isVoice(a.Type), isVoice(b.Type)
		if av != bv {
			return !av
		}
		if a.Position != b.Position {
			return a.Position < b.Position
		}
		return a.ID < b.ID
	}
	var top, cats []discord.Channel
	children := map[discord.ChannelID][]discord.Channel{}
	for _, ch := range chs {
		switch {
		case ch.Type == discord.GuildCategory:
			cats = append(cats, ch)
		case isCat[ch.ParentID]:
			children[ch.ParentID] = append(children[ch.ParentID], ch)
		default:
			top = append(top, ch)
		}
	}
	sort.SliceStable(top, func(i, j int) bool { return less(top[i], top[j]) })
	sort.SliceStable(cats, func(i, j int) bool {
		if cats[i].Position != cats[j].Position {
			return cats[i].Position < cats[j].Position
		}
		return cats[i].ID < cats[j].ID
	})
	out := append([]discord.Channel{}, top...)
	for _, c := range cats {
		out = append(out, c)
		kids := children[c.ID]
		sort.SliceStable(kids, func(i, j int) bool { return less(kids[i], kids[j]) })
		out = append(out, kids...)
	}
	return out
}

func isVoice(t discord.ChannelType) bool {
	return t == discord.GuildVoice || t == discord.GuildStageVoice
}

func wireChannel(n *ningen.State, ch discord.Channel) protocol.Channel {
	c := protocol.Channel{
		ID:            snowflake(discord.Snowflake(ch.ID)),
		GuildID:       optSnowflake(discord.Snowflake(ch.GuildID)),
		Type:          channelType(ch.Type),
		Name:          ch.Name,
		Topic:         ch.Topic,
		ParentID:      optSnowflake(discord.Snowflake(ch.ParentID)),
		Position:      ch.Position,
		LastMessageID: optSnowflake(discord.Snowflake(ch.LastMessageID)),
		Unread:        unreadString(n.ChannelIsUnread(ch.ID, unreadOpts)),
		MentionCount:  mentionCount(n, ch.ID),
		Muted:         n.ChannelIsMuted(ch.ID, unreadOpts),
		Recipients:    make([]protocol.User, 0, len(ch.DMRecipients)),

		LastReadMessageID: lastReadMessageID(n, ch.ID),
	}
	if ch.Type == discord.DirectMessage || ch.Type == discord.GroupDM {
		names := make([]string, 0, len(ch.DMRecipients))
		for _, u := range ch.DMRecipients {
			c.Recipients = append(c.Recipients, wireUser(u))
			names = append(names, u.DisplayOrUsername())
		}
		if c.Name == "" {
			c.Name = strings.Join(names, ", ")
		}
	}
	return c
}

// DMs lists private channels, most recent message first.
func DMs(n *ningen.State) ([]protocol.Channel, error) {
	chs, err := n.PrivateChannels()
	if err != nil {
		return nil, err
	}
	out := make([]protocol.Channel, 0, len(chs))
	for _, ch := range chs {
		out = append(out, wireChannel(n, ch))
	}
	return out, nil
}

// UnreadDM returns the most recent unread DM channel id, or nil.
func UnreadDM(n *ningen.State) *string {
	chs, err := n.PrivateChannels()
	if err != nil {
		return nil
	}
	for _, ch := range chs {
		if n.ChannelIsUnread(ch.ID, unreadOpts) != ningen.ChannelRead {
			id := ch.ID.String()
			return &id
		}
	}
	return nil
}
