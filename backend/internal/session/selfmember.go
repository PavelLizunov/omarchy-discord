package session

import (
	"encoding/json"

	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/ningen/v3"
)

// readyMergedMembers is the slice of READY that arikawa does not model: with
// capabilities, Discord sends our own membership of every guild as
// `merged_members`, index-aligned with `guilds` (READY_SUPPLEMENTAL's
// `merged_members`, which arikawa does parse, only carries friends).
type readyMergedMembers struct {
	MergedMembers [][]gateway.SupplementalMember `json:"merged_members"`
}

// seedSelfMembers caches our own member for every guild from READY's
// merged_members. Without it the member store never holds us, so every
// permission check (ningen's channel filter, ChannelIsUnread) misses: it
// fails outright through Offline() and REST-fetches per guild otherwise.
// Must run before anything reads permissions for the new cache, i.e. at the
// top of the ConnectedEvent handler for a READY.
func seedSelfMembers(n *ningen.State, ready *gateway.ReadyEvent) {
	me, err := n.Cabinet.Me()
	if err != nil || len(ready.RawEventBody) == 0 {
		return
	}
	var extra readyMergedMembers
	if err := json.Unmarshal(ready.RawEventBody, &extra); err != nil {
		return
	}
	for i, sms := range extra.MergedMembers {
		if i >= len(ready.Guilds) {
			break
		}
		guildID := ready.Guilds[i].ID
		for _, sm := range sms {
			if sm.UserID != me.ID {
				continue
			}
			if _, err := n.Cabinet.Member(guildID, me.ID); err == nil {
				break // a fuller member (GUILD_CREATE members) wins
			}
			mem := gateway.ConvertSupplementalMembers([]gateway.SupplementalMember{sm})[0]
			mem.User = *me
			n.Cabinet.MemberSet(guildID, &mem, false)
			break
		}
	}
}
