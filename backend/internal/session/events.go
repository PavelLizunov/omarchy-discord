package session

import (
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/ningen/v3"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/socket"
)

func (m *Manager) installMessageHandlers(n *ningen.State) {
	live := func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		return m.n == n && m.everReady
	}
	route := func(chID discord.ChannelID, all bool, ev any) {
		m.push(socket.Routed{ChannelID: chID.String(), All: all, Event: ev})
	}
	updateFromCache := func(chID discord.ChannelID, msgID discord.MessageID) {
		off := n.Offline()
		msg, err := off.Cabinet.Message(chID, msgID)
		if err != nil {
			return
		}
		route(chID, false, protocol.NewMessageUpdate(WireMessage(off, msg)))
	}

	addSyncHandler(n, "message_create", func(ev *gateway.MessageCreateEvent) {
		if !live() {
			return
		}
		off := n.Offline()
		notify := off.MessageMentions(&ev.Message).Has(ningen.MessageNotifies)
		wire := WireMessage(off, &ev.Message)
		route(ev.ChannelID, notify, protocol.NewMessageCreate(wire, notify, channelName(off, ev.ChannelID)))
	})
	addSyncHandler(n, "message_update", func(ev *gateway.MessageUpdateEvent) {
		if !live() {
			return
		}
		off := n.Offline()
		msg, err := off.Cabinet.Message(ev.ChannelID, ev.ID)
		if err != nil {
			if !ev.Author.ID.IsValid() {
				return
			}
			msg = &ev.Message
		}
		route(ev.ChannelID, false, protocol.NewMessageUpdate(WireMessage(off, msg)))
	})
	addSyncHandler(n, "message_delete", func(ev *gateway.MessageDeleteEvent) {
		if !live() {
			return
		}
		route(ev.ChannelID, false, protocol.NewMessageDelete(ev.ChannelID.String(), optSnowflake(discord.Snowflake(ev.GuildID)), ev.ID.String()))
	})
	addSyncHandler(n, "message_delete_bulk", func(ev *gateway.MessageDeleteBulkEvent) {
		if !live() {
			return
		}
		for _, id := range ev.IDs {
			route(ev.ChannelID, false, protocol.NewMessageDelete(ev.ChannelID.String(), optSnowflake(discord.Snowflake(ev.GuildID)), id.String()))
		}
	})
	addSyncHandler(n, "reaction_add", func(ev *gateway.MessageReactionAddEvent) {
		if live() {
			updateFromCache(ev.ChannelID, ev.MessageID)
		}
	})
	addSyncHandler(n, "reaction_remove", func(ev *gateway.MessageReactionRemoveEvent) {
		if live() {
			updateFromCache(ev.ChannelID, ev.MessageID)
		}
	})
	addSyncHandler(n, "reaction_remove_all", func(ev *gateway.MessageReactionRemoveAllEvent) {
		if !live() {
			return
		}
		if msg, err := n.Cabinet.Message(ev.ChannelID, ev.MessageID); err == nil && len(msg.Reactions) > 0 {
			cpy := *msg
			cpy.Reactions = []discord.Reaction{}
			n.Cabinet.MessageSet(&cpy, true)
		}
		updateFromCache(ev.ChannelID, ev.MessageID)
	})
	addSyncHandler(n, "reaction_remove_emoji", func(ev *gateway.MessageReactionRemoveEmojiEvent) {
		if live() {
			updateFromCache(ev.ChannelID, ev.MessageID)
		}
	})
	addSyncHandler(n, "typing_start", func(ev *gateway.TypingStartEvent) {
		if !live() {
			return
		}
		off := n.Offline()
		name := typerName(off, ev)
		route(ev.ChannelID, false, protocol.NewTypingStart(
			ev.ChannelID.String(), optSnowflake(discord.Snowflake(ev.GuildID)),
			ev.UserID.String(), name, wireTime(ev.Timestamp.Time())))
	})
}

func typerName(n *ningen.State, ev *gateway.TypingStartEvent) string {
	if ev.Member != nil {
		if ev.Member.Nick != "" {
			return ev.Member.Nick
		}
		if ev.Member.User.ID.IsValid() {
			return ev.Member.User.DisplayOrUsername()
		}
	}
	if ev.GuildID.IsValid() {
		if mem, err := n.Cabinet.Member(ev.GuildID, ev.UserID); err == nil {
			return displayName(n, ev.GuildID, mem.User)
		}
		return ""
	}
	if ch, err := n.Cabinet.Channel(ev.ChannelID); err == nil {
		for _, u := range ch.DMRecipients {
			if u.ID == ev.UserID {
				return u.DisplayOrUsername()
			}
		}
	}
	return ""
}
