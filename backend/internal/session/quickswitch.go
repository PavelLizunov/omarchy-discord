package session

import (
	"sort"
	"strings"
	"unicode"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/ningen/v3"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
)

// quickSwitchDefault is the default result cap for quick_switch.
const quickSwitchDefault = 20

// openable reports whether a channel type can be opened as a timeline: text,
// announcement, threads, DMs, group DMs. Voice, stage, categories, and forums
// are excluded.
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

// candidate is one switcher entry before scoring.
type candidate struct {
	ch        discord.Channel
	guildName string
	name      string // display name (DMs: recipient names)
}

// switchCandidates lists every openable channel: visible guild channels and
// unarchived threads (via ningen's permission-filtered Channels) and private
// channels. Threads additionally require View Channel on the parent, since the
// permission check on a thread itself sees no overwrites. n should be
// Offline(): the filter needs our own member per guild, which READY seeds
// (seedSelfMembers), so a miss must hide the guild rather than REST-fetch on
// every keystroke.
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

// dmName mirrors wireChannel's DM naming: the set name or the joined
// recipient display names.
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

// fuzzyScore scores query as a subsequence of text (both lowercased). 0 means
// no match. Each matched rune scores 2, consecutive matches +3, a match at a
// word start +2 (and the gap before it is free), a match at the very start
// +3; otherwise every skipped rune between matches costs 1, capped at 3.
// Deterministic and greedy (first occurrence wins).
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
	// Prefer shorter texts for equal matches so "dev" beats "devops-chat".
	return score - float64(len(t)-len(q))*0.01
}

func isWordBoundary(r rune) bool {
	return r == '-' || r == '_' || r == ' ' || r == '.' || r == '/' || unicode.IsPunct(r)
}

// unreadTier orders mentioned (2) > unread (1) > read (0).
func unreadTier(u ningen.UnreadIndication) int {
	switch u {
	case ningen.ChannelMentioned:
		return 2
	case ningen.ChannelUnread:
		return 1
	}
	return 0
}

// lastPreview is the newest cached message of a channel collapsed to one
// line, "" when nothing is cached.
func lastPreview(n *ningen.State, chID discord.ChannelID) string {
	msgs, err := n.Cabinet.Messages(chID)
	if err != nil || len(msgs) == 0 {
		return ""
	}
	return preview(&msgs[0])
}

// QuickSwitch ranks openable channels for the switcher. With a query, only
// fuzzy matches on the channel name (or, at half weight, the guild name) are
// returned; unread/mentioned entries come first, then by score, then by
// recency. With an empty query the unread set comes first, then everything
// else by recency. Cache-only: callers pass Offline().
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

// quickSwitch implements the quick_switch command.
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
