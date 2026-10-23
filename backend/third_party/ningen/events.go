package ningen

import (
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/utils/json"
)

type readyEventExtras struct {
	Users             []discord.User        `json:"users,omitempty"`
	PrivateChannelsV2 []readyPrivateChannel `json:"private_channels,omitempty"`
}

type readyPrivateChannel struct {
	discord.Channel
	RecipientIDs []discord.UserID `json:"recipient_ids,omitempty"`
}

func (ch *readyPrivateChannel) UnmarshalJSON(data []byte) error {
	// Channel's promoted unmarshaler otherwise consumes the whole object,
	// leaving recipient_ids undecoded beside the embedded Channel.
	if err := json.Unmarshal(data, &ch.Channel); err != nil {
		return err
	}
	var recipients struct {
		IDs []discord.UserID `json:"recipient_ids"`
	}
	if err := json.Unmarshal(data, &recipients); err != nil {
		return err
	}
	ch.RecipientIDs = recipients.IDs
	return nil
}
