package session

import (
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/ningen/v3"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
)

// Emojis lists the custom emoji of every guild, in guild display order,
// filtered to what the account can use there: available, and (when the emoji
// is role-restricted and our membership is cached) one of our roles. Nitro
// cross-guild gating is not modelled; Discord's 400 on react surfaces it.
// Guilds with no usable emoji are omitted.
func Emojis(n *ningen.State) ([]protocol.GuildEmoji, error) {
	gs, err := orderedGuilds(n)
	if err != nil {
		return nil, err
	}
	me, _ := n.Cabinet.Me()
	out := []protocol.GuildEmoji{}
	for _, g := range gs {
		emojis, err := n.Cabinet.Emojis(g.ID)
		if err != nil || len(emojis) == 0 {
			continue
		}
		var roles map[discord.RoleID]bool
		if me != nil {
			if mem, err := n.Cabinet.Member(g.ID, me.ID); err == nil {
				roles = map[discord.RoleID]bool{}
				for _, r := range mem.RoleIDs {
					roles[r] = true
				}
			}
		}
		list := make([]protocol.Emoji, 0, len(emojis))
		for _, e := range emojis {
			if !e.Available || !e.ID.IsValid() || !roleAllowed(e.RoleIDs, roles) {
				continue
			}
			list = append(list, protocol.Emoji{ID: e.ID.String(), Name: e.Name, Animated: e.Animated, URL: e.EmojiURL()})
		}
		if len(list) == 0 {
			continue
		}
		out = append(out, protocol.GuildEmoji{GuildID: g.ID.String(), GuildName: g.Name, Emoji: list})
	}
	return out, nil
}

// roleAllowed reports whether an emoji restricted to want is usable with the
// cached role set have (nil have = membership unknown = allowed).
func roleAllowed(want []discord.RoleID, have map[discord.RoleID]bool) bool {
	if len(want) == 0 || have == nil {
		return true
	}
	for _, r := range want {
		if have[r] {
			return true
		}
	}
	return false
}

// listEmoji implements the list_emoji command.
func (m *Manager) listEmoji() (any, *protocol.Error) {
	n, e := m.cachedSession()
	if e != nil {
		return nil, e
	}
	gs, err := Emojis(n.Offline())
	if err != nil {
		return nil, protocol.Errorf(protocol.CodeInternalError, "%v", err)
	}
	return protocol.ListEmojiResult{Guilds: gs}, nil
}
