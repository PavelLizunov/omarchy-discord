package session

import (
	"sort"
	"strings"
	"unicode"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/ningen/v3"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
)

const quickSwitchDefault = 20

func openable(t discord.ChannelType) bool {
	switch t {
	case discord.GuildText, discord.GuildAnnouncement,
		discord.GuildPublicThread, discord.GuildPrivateThread, discord.GuildAnnouncementThread,
		discord.DirectMessage, discord.GroupDM:
		return true
	}
	return false
}

func isThread(t discord.ChannelType) bool {
	return t == discord.GuildPublicThread || t == discord.GuildPrivateThread || t == discord.GuildAnnouncementThread
}

func archived(ch *discord.Channel) bool {
	return ch.ThreadMetadata != nil && ch.ThreadMetadata.Archived
}

type candidate struct {
	ch        discord.Channel
	guildName string
	name      string
}

func switchCandidates(n *ningen.State) []candidate {
	var out []candidate
	if gs, err := n.Cabinet.Guilds(); err == nil {
		for _, g := range gs {
			chs, err := n.Channels(g.ID, AllowedChannelTypes)
			if err != nil {
				continue
			}
			for _, ch := range chs {
				if !openable(ch.Type) {
					continue
				}
				if isThread(ch.Type) {
					if archived(&ch) || (ch.ParentID.IsValid() && !n.HasPermissions(ch.ParentID, discord.PermissionViewChannel)) {
						continue
					}
				}
				out = append(out, candidate{ch: ch, guildName: g.Name, name: ch.Name})
			}
		}
	}
	if dms, err := n.PrivateChannels(); err == nil {
		for _, ch := range dms {
			if !openable(ch.Type) {
				continue
			}
			out = append(out, candidate{ch: ch, name: dmName(ch)})
		}
	}
	return out
}

func dmName(ch discord.Channel) string {
	if ch.Name != "" {
		return ch.Name
	}
	names := make([]string, 0, len(ch.DMRecipients))
	for _, u := range ch.DMRecipients {
		names = append(names, u.DisplayOrUsername())
	}
	return strings.Join(names, ", ")
}

func fuzzyScore(query, text string) float64 {
	q := []rune(strings.ToLower(strings.TrimSpace(query)))
	t := []rune(strings.ToLower(text))
	if len(q) == 0 || len(t) == 0 {
		return 0
	}
	score, qi, last := 0.0, 0, -1
	for ti := 0; ti < len(t) && qi < len(q); ti++ {
		if t[ti] != q[qi] {
			continue
		}
		score += 2
		switch {
		case ti == 0:
			score += 3
		case last == ti-1:
			score += 3
		case isWordBoundary(t[ti-1]):
			score += 2
		default:
			if last >= 0 {
				score -= min(float64(ti-last-1), 3)
			}
		}
		last, qi = ti, qi+1
	}
	if qi < len(q) {
		return 0
	}
	return score - float64(len(t)-len(q))*0.01
}

func isWordBoundary(r rune) bool {
	return r == '-' || r == '_' || r == ' ' || r == '.' || r == '/' || unicode.IsPunct(r)
}

func unreadTier(u ningen.UnreadIndication) int {
	switch u {
	case ningen.ChannelMentioned:
		return 2
	case ningen.ChannelUnread:
		return 1
	}
	return 0
}

func lastPreview(n *ningen.State, chID discord.ChannelID) string {
	msgs, err := n.Cabinet.Messages(chID)
	if err != nil || len(msgs) == 0 {
		return ""
	}
	return preview(&msgs[0])
}

func QuickSwitch(n *ningen.State, query string, limit int) []protocol.QuickSwitchEntry {
	if limit <= 0 {
		limit = quickSwitchDefault
	}
	type scored struct {
		candidate
		tier  int
		score float64
	}
	var rows []scored
	hasQuery := strings.TrimSpace(query) != ""
	for _, c := range switchCandidates(n) {
		s := 0.0
		if hasQuery {
			s = fuzzyScore(query, c.name)
			if c.guildName != "" {
				if gs := fuzzyScore(query, c.guildName) * 0.5; gs > s {
					s = gs
				}
			}
			if s <= 0 {
				continue
			}
		}
		rows = append(rows, scored{candidate: c, tier: unreadTier(n.ChannelIsUnread(c.ch.ID, unreadOpts)), score: s})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.tier != b.tier {
			return a.tier > b.tier
		}
		if a.score != b.score {
			return a.score > b.score
		}
		if a.ch.LastMessageID != b.ch.LastMessageID {
			return a.ch.LastMessageID > b.ch.LastMessageID
		}
		return a.ch.ID < b.ch.ID
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	out := make([]protocol.QuickSwitchEntry, 0, len(rows))
	for _, r := range rows {
		var gname *string
		if r.guildName != "" {
			g := r.guildName
			gname = &g
		}
		out = append(out, protocol.QuickSwitchEntry{
			Channel:            wireChannel(n, r.ch),
			GuildName:          gname,
			LastMessagePreview: lastPreview(n, r.ch.ID),
			Score:              r.score,
		})
	}
	return out
}

func (m *Manager) quickSwitch(req *protocol.Request) (any, *protocol.Error) {
	var p protocol.QuickSwitchParams
	if e := req.Params(&p); e != nil {
		return nil, e
	}
	n, e := m.cachedSession()
	if e != nil {
		return nil, e
	}
	return protocol.QuickSwitchResult{Entries: QuickSwitch(n.Offline(), p.Query, p.Limit)}, nil
}
