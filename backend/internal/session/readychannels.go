package session

import (
	"encoding/json"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/ningen/v3"
)

func seedPrivateChannels(n *ningen.State, ready *gateway.ReadyEvent) {
	var extra struct {
		Users    []discord.User    `json:"users"`
		Channels []json.RawMessage `json:"private_channels"`
	}
	if err := json.Unmarshal(ready.RawEventBody, &extra); err != nil {
		return
	}
	users := make(map[discord.UserID]discord.User, len(extra.Users))
	for _, user := range extra.Users {
		users[user.ID] = user
	}
	for _, raw := range extra.Channels {
		var channel discord.Channel
		if err := json.Unmarshal(raw, &channel); err != nil {
			continue
		}
		if channel.Type != discord.DirectMessage && channel.Type != discord.GroupDM {
			continue
		}
		// Channel.UnmarshalJSON consumes embedded fields, so decode recipient_ids separately.
		var recipients struct {
			IDs []discord.UserID `json:"recipient_ids"`
		}
		if err := json.Unmarshal(raw, &recipients); err != nil {
			continue
		}
		if len(channel.DMRecipients) == 0 {
			for _, id := range recipients.IDs {
				user, ok := users[id]
				if !ok {
					if presence, _ := n.Cabinet.Presence(0, id); presence != nil {
						user = presence.User
					}
				}
				user.ID = id
				channel.DMRecipients = append(channel.DMRecipients, user)
			}
		}
		_ = n.Cabinet.ChannelSet(&channel, true)
	}
}
