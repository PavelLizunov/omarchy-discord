package session

import (
	"encoding/json"

	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/ningen/v3"
)

type readyMergedMembers struct {
	MergedMembers [][]gateway.SupplementalMember `json:"merged_members"`
}

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
				break
			}
			mem := gateway.ConvertSupplementalMembers([]gateway.SupplementalMember{sm})[0]
			mem.User = *me
			n.Cabinet.MemberSet(guildID, &mem, false)
			break
		}
	}
}
