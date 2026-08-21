package session

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/state/store"
	"github.com/diamondburned/arikawa/v3/utils/httputil"
	"github.com/diamondburned/ningen/v3"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/socket"
)

const (
	// openTail is how many messages open_channel returns.
	openTail = 50
	// historyDefault / historyMax bound the history page size.
	historyDefault = 50
	historyMax     = 100
	// avatarSize is the CDN size hint appended to avatar URLs.
	avatarSize = 64
	// previewRunes caps reply previews.
	previewRunes = 120
)

// wireTime formats a Discord timestamp as RFC 3339 UTC with milliseconds.
func wireTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z07:00")
}

func sizedURL(u string, size int) string {
	if u == "" {
		return ""
	}
	if strings.Contains(u, "?") {
		return u + fmt.Sprintf("&size=%d", size)
	}
	return u + fmt.Sprintf("?size=%d", size)
}

// displayName resolves guild nick > global display name > username using only
// the cache (never REST: member fetches are rate-limited and, for user
// accounts, a flag risk).
func displayName(n *ningen.State, guildID discord.GuildID, u discord.User) string {
	if guildID.IsValid() {
		if m, err := n.Cabinet.Member(guildID, u.ID); err == nil && m.Nick != "" {
			return m.Nick
		}
	}
	return u.DisplayOrUsername()
}

func wireAuthor(n *ningen.State, guildID discord.GuildID, u discord.User) protocol.MessageAuthor {
	return protocol.MessageAuthor{
		ID:          u.ID.String(),
		Username:    u.Username,
		DisplayName: displayName(n, guildID, u),
		AvatarURL:   sizedURL(u.AvatarURL(), avatarSize),
		Bot:         u.Bot,
	}
}

// preview flattens a message into one short plain-text line for reply quotes.
// Markdown is left as-is (collapsed whitespace only) — parsing it is the QML
// renderer's job and ningen's discordmd can panic on hostile input.
func preview(msg *discord.Message) string {
	text := strings.Join(strings.Fields(msg.Content), " ")
	if text == "" {
		switch {
		case len(msg.Attachments) > 0:
			names := make([]string, 0, len(msg.Attachments))
			for _, a := range msg.Attachments {
				names = append(names, a.Filename)
			}
			text = strings.Join(names, ", ")
		case len(msg.Embeds) > 0:
			text = "[embed]"
		case len(msg.Stickers) > 0:
			text = "[sticker]"
		}
	}
	if r := []rune(text); len(r) > previewRunes {
		text = string(r[:previewRunes-1]) + "…"
	}
	return text
}

// isSystem reports whether a message type renders as a system line rather
// than user content.
func isSystem(t discord.MessageType) bool {
	switch t {
	case discord.DefaultMessage, discord.InlinedReplyMessage, discord.ChatInputCommandMessage, discord.ContextMenuCommand:
		return false
	}
	return true
}

// systemLine renders a non-default message type as one plain-text line that
// includes the actor's display name, so QML can show it without a header.
func systemLine(n *ningen.State, msg *discord.Message) string {
	who := displayName(n, msg.GuildID, msg.Author)
	target := ""
	if len(msg.Mentions) > 0 {
		target = displayName(n, msg.GuildID, msg.Mentions[0].User)
	}
	switch msg.Type {
	case discord.RecipientAddMessage:
		return who + " added " + target + " to the group."
	case discord.RecipientRemoveMessage:
		if len(msg.Mentions) > 0 && msg.Mentions[0].ID == msg.Author.ID {
			return who + " left the group."
		}
		return who + " removed " + target + " from the group."
	case discord.CallMessage:
		if msg.Call.EndedTimestamp != nil {
			return who + " started a call that has ended."
		}
		return who + " started a call."
	case discord.ChannelNameChangeMessage:
		return who + " changed the channel name to " + msg.Content + "."
	case discord.ChannelIconChangeMessage:
		return who + " changed the channel icon."
	case discord.ChannelPinnedMessage:
		return who + " pinned a message."
	case discord.GuildMemberJoinMessage:
		return who + " joined the server."
	case discord.NitroBoostMessage:
		return who + " boosted the server!"
	case discord.NitroTier1Message:
		return who + " boosted the server! The server reached Tier 1."
	case discord.NitroTier2Message:
		return who + " boosted the server! The server reached Tier 2."
	case discord.NitroTier3Message:
		return who + " boosted the server! The server reached Tier 3."
	case discord.ChannelFollowAddMessage:
		return who + " followed " + msg.Content + " into this channel."
	case discord.ThreadCreatedMessage:
		return who + " started a thread: " + msg.Content + "."
	case discord.ThreadStarterMessage:
		return who + " started a thread from a message."
	case discord.GuildInviteReminderMessage:
		return "Invite your friends to the server."
	case discord.AutoModerationActionMessage:
		return "AutoMod took an action on a message."
	case discord.StageStartMessage:
		return who + " started a stage: " + msg.Content + "."
	case discord.StageEndMessage:
		return who + " ended the stage."
	case discord.StageSpeakerMessage:
		return who + " is now a speaker."
	case discord.StageTopicMessage:
		return who + " changed the stage topic to " + msg.Content + "."
	case discord.GuildDiscoveryDisqualifiedMessage:
		return "The server is no longer eligible for Server Discovery."
	case discord.GuildDiscoveryRequalifiedMessage:
		return "The server is eligible for Server Discovery again."
	case discord.GuildDiscoveryGracePeriodInitialWarning, discord.GuildDiscoveryGracePeriodFinalWarning:
		return "The server is at risk of losing Server Discovery."
	case discord.RoleSubscriptionPurchaseMessage:
		return who + " purchased a role subscription."
	case discord.GuildApplicationPremiumSubscriptionMessage:
		return who + " upgraded an app to premium."
	}
	return fmt.Sprintf("%s sent a system message (type %d).", who, msg.Type)
}

// replyTo resolves the reply reference from the inline referenced message or
// the cache; an unknown target keeps the id with empty name/preview.
func replyTo(n *ningen.State, msg *discord.Message) *protocol.ReplyTo {
	ref := msg.Reference
	if ref == nil || !ref.MessageID.IsValid() || ref.Type == discord.MessageReferenceTypeForward {
		return nil
	}
	out := &protocol.ReplyTo{MessageID: ref.MessageID.String()}
	target := msg.ReferencedMessage
	if target == nil {
		chID := ref.ChannelID
		if !chID.IsValid() {
			chID = msg.ChannelID
		}
		target, _ = n.Cabinet.Message(chID, ref.MessageID)
	}
	if target != nil {
		guildID := target.GuildID
		if !guildID.IsValid() {
			guildID = msg.GuildID
		}
		out.AuthorDisplayName = displayName(n, guildID, target.Author)
		out.Preview = preview(target)
	}
	return out
}

// WireMessage maps a cached/gateway message to the wire shape. n should be an
// offline state: every lookup is cache-only.
func WireMessage(n *ningen.State, msg *discord.Message) protocol.Message {
	m := protocol.Message{
		ID:          msg.ID.String(),
		ChannelID:   msg.ChannelID.String(),
		GuildID:     optSnowflake(discord.Snowflake(msg.GuildID)),
		Author:      wireAuthor(n, msg.GuildID, msg.Author),
		Content:     msg.Content,
		Timestamp:   wireTime(msg.Timestamp.Time()),
		Nonce:       msg.Nonce,
		ReplyTo:     replyTo(n, msg),
		Attachments: make([]protocol.Attachment, 0, len(msg.Attachments)),
		Embeds:      make([]protocol.Embed, 0, len(msg.Embeds)),
		Reactions:   make([]protocol.Reaction, 0, len(msg.Reactions)),
		System:      isSystem(msg.Type),
	}
	if msg.EditedTimestamp.IsValid() {
		t := wireTime(msg.EditedTimestamp.Time())
		m.EditedTimestamp = &t
	}
	if m.System {
		m.Content = systemLine(n, msg)
	}
	for _, a := range msg.Attachments {
		m.Attachments = append(m.Attachments, protocol.Attachment{
			ID:          a.ID.String(),
			Filename:    a.Filename,
			ContentType: a.ContentType,
			Size:        a.Size,
			URL:         string(a.URL),
			ProxyURL:    string(a.Proxy),
			Width:       a.Width,
			Height:      a.Height,
			Spoiler:     strings.HasPrefix(a.Filename, "SPOILER_"),
		})
	}
	for _, e := range msg.Embeds {
		w := protocol.Embed{Type: string(e.Type), Title: e.Title, Description: e.Description, URL: string(e.URL)}
		if e.Image != nil {
			w.ImageURL = string(e.Image.URL)
		}
		if e.Thumbnail != nil {
			w.ThumbnailURL = string(e.Thumbnail.URL)
		}
		if e.Color > 0 {
			w.Color = int(e.Color)
		}
		m.Embeds = append(m.Embeds, w)
	}
	for _, r := range msg.Reactions {
		m.Reactions = append(m.Reactions, protocol.Reaction{Emoji: string(r.Emoji.APIString()), Count: r.Count, Me: r.Me})
	}
	m.MentionsSelf = n.MessageMentions(msg).Has(ningen.MessageMentions)
	return m
}

// wireMessages maps messages into an ascending (oldest→newest) wire slice.
func wireMessages(n *ningen.State, msgs []discord.Message) []protocol.Message {
	sorted := append([]discord.Message(nil), msgs...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	out := make([]protocol.Message, 0, len(sorted))
	for i := range sorted {
		out = append(out, WireMessage(n, &sorted[i]))
	}
	return out
}

// channelName is the sidebar name of a channel (DMs: recipient names).
func channelName(n *ningen.State, chID discord.ChannelID) string {
	ch, err := n.Cabinet.Channel(chID)
	if err != nil {
		return ""
	}
	return wireChannel(n, *ch).Name
}

func parseSnowflake(s, field string) (discord.Snowflake, *protocol.Error) {
	sf, err := discord.ParseSnowflake(s)
	if err != nil || !sf.IsValid() {
		return 0, protocol.Errorf(protocol.CodeInvalidArgument, "%s must be a snowflake string", field)
	}
	return sf, nil
}

// discordError maps a REST failure to a protocol error.
func discordError(err error) *protocol.Error {
	var herr *httputil.HTTPError
	if errors.As(err, &herr) {
		switch herr.Status {
		case http.StatusForbidden:
			return protocol.Errorf(protocol.CodeForbidden, "discord denied the action: %v", err)
		case http.StatusNotFound:
			return protocol.Errorf(protocol.CodeUnknownChannel, "channel is not accessible: %v", err)
		case http.StatusTooManyRequests:
			return protocol.Errorf(protocol.CodeRateLimited, "rate limited: %v", err)
		}
	}
	return protocol.Errorf(protocol.CodeDiscordError, "%v", err)
}

// Message fetchers; tests replace them to avoid the network.
func fetchTail(ctx context.Context, n *ningen.State, chID discord.ChannelID, limit uint) ([]discord.Message, error) {
	// Cache-aware: fills the cabinet on a cold channel.
	return n.WithContext(ctx).Messages(chID, limit)
}

func fetchBefore(ctx context.Context, n *ningen.State, chID discord.ChannelID, before discord.MessageID, limit uint) ([]discord.Message, error) {
	// Plain REST: deep pages never enter the cache.
	return n.Client.WithContext(ctx).MessagesBefore(chID, before, limit)
}

// openChannel implements the open_channel command.
func (m *Manager) openChannel(ctx context.Context, req *protocol.Request) (any, *protocol.Error) {
	var p protocol.OpenChannelParams
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
	if ch.GuildID.IsValid() {
		if !off.HasPermissions(chID, discord.PermissionViewChannel) {
			return nil, protocol.Errorf(protocol.CodeForbidden, "no permission to view channel %s", p.ChannelID)
		}
		n.MemberState.Subscribe(ch.GuildID)
	}
	// Register the open before fetching the tail so a message that lands
	// during the fetch is routed to this connection rather than lost; the
	// client dedupes by id if it also shows up in the tail. A failed open
	// rolls the registration back unless the channel was already open.
	c := socket.ClientFromContext(ctx)
	wasOpen := false
	if c != nil {
		wasOpen = c.HasOpen(p.ChannelID)
		c.OpenChannel(p.ChannelID)
	}
	rollback := func() {
		if c != nil && !wasOpen {
			c.CloseChannel(p.ChannelID)
		}
	}
	msgs, err := m.fetchTail(ctx, n, chID, openTail)
	if err != nil {
		rollback()
		return nil, m.restError(n, err, false)
	}
	if len(msgs) == 0 && ch.Type == discord.DirectMessage {
		rollback()
		return nil, protocol.Errorf(protocol.CodeEmptyDMRefused, "refusing to open a DM with no history; send a message from the official client first")
	}
	// The cache-aware fetch can return more than asked for (a "tiny" channel
	// hands back its whole store); keep the newest openTail so has_more keeps
	// meaning "a full page came back".
	if len(msgs) > openTail {
		sort.SliceStable(msgs, func(i, j int) bool { return msgs[i].ID > msgs[j].ID })
		msgs = msgs[:openTail]
	}
	return protocol.OpenChannelResult{
		Channel:  wireChannel(off, *ch),
		Messages: wireMessages(off, msgs),
		HasMore:  len(msgs) == openTail,
	}, nil
}

// closeChannel implements the close_channel command.
func (m *Manager) closeChannel(ctx context.Context, req *protocol.Request) (any, *protocol.Error) {
	var p protocol.CloseChannelParams
	if e := req.Params(&p); e != nil {
		return nil, e
	}
	if _, e := parseSnowflake(p.ChannelID, "channel_id"); e != nil {
		return nil, e
	}
	c := socket.ClientFromContext(ctx)
	if c == nil || !c.CloseChannel(p.ChannelID) {
		return nil, protocol.Errorf(protocol.CodeChannelNotOpen, "channel %s is not open", p.ChannelID)
	}
	return protocol.EmptyResult{}, nil
}

// history implements the history command: the page is served from the cache
// when it holds a full page older than before_id, else from REST without
// touching the cache.
func (m *Manager) history(ctx context.Context, req *protocol.Request) (any, *protocol.Error) {
	var p protocol.HistoryParams
	if e := req.Params(&p); e != nil {
		return nil, e
	}
	chSF, e := parseSnowflake(p.ChannelID, "channel_id")
	if e != nil {
		return nil, e
	}
	beforeSF, e := parseSnowflake(p.BeforeID, "before_id")
	if e != nil {
		return nil, e
	}
	limit := p.Limit
	switch {
	case limit <= 0:
		limit = historyDefault
	case limit > historyMax:
		limit = historyMax
	}
	c := socket.ClientFromContext(ctx)
	if c == nil || !c.HasOpen(p.ChannelID) {
		return nil, protocol.Errorf(protocol.CodeChannelNotOpen, "channel %s is not open", p.ChannelID)
	}
	n, e := m.cachedSession()
	if e != nil {
		return nil, e
	}
	chID, before := discord.ChannelID(chSF), discord.MessageID(beforeSF)
	off := n.Offline()
	ch, err := off.Cabinet.Channel(chID)
	if err != nil {
		return nil, protocol.Errorf(protocol.CodeUnknownChannel, "channel %s is not visible to this account", p.ChannelID)
	}

	var page []discord.Message
	if cached, err := off.Cabinet.Messages(chID); err == nil || errors.Is(err, store.ErrNotFound) {
		for _, msg := range cached { // latest → oldest
			if msg.ID < before {
				page = append(page, msg)
				if len(page) == limit {
					break
				}
			}
		}
	}
	if len(page) < limit {
		page, err = m.fetchBefore(ctx, n, chID, before, uint(limit))
		if err != nil {
			return nil, m.restError(n, err, false)
		}
		for i := range page {
			page[i].GuildID = ch.GuildID
		}
	}
	return protocol.HistoryResult{Messages: wireMessages(off, page), HasMore: len(page) >= limit}, nil
}

// ack implements the ack command. The message must be cached: ningen only
// sends the REST ack for cached, non-self messages, and we would rather say so
// than silently move the local marker.
func (m *Manager) ack(req *protocol.Request) (any, *protocol.Error) {
	var p protocol.AckParams
	if e := req.Params(&p); e != nil {
		return nil, e
	}
	chSF, e := parseSnowflake(p.ChannelID, "channel_id")
	if e != nil {
		return nil, e
	}
	msgSF, e := parseSnowflake(p.MessageID, "message_id")
	if e != nil {
		return nil, e
	}
	n, e := m.cachedSession()
	if e != nil {
		return nil, e
	}
	chID, msgID := discord.ChannelID(chSF), discord.MessageID(msgSF)
	if _, err := n.Cabinet.Channel(chID); err != nil {
		return nil, protocol.Errorf(protocol.CodeUnknownChannel, "channel %s is not visible to this account", p.ChannelID)
	}
	if _, err := n.Cabinet.Message(chID, msgID); err != nil {
		return nil, protocol.Errorf(protocol.CodeUnknownMessage, "message %s is not cached; open the channel first", p.MessageID)
	}
	n.ReadState.MarkRead(chID, msgID)
	return protocol.EmptyResult{}, nil
}
