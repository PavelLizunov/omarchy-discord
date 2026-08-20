package protocol

// Lifecycle values carried in State.Lifecycle.
const (
	LifecycleStarting     = "starting"
	LifecycleLoggedOut    = "logged_out"
	LifecycleQRPending    = "qr_pending"
	LifecycleConnecting   = "connecting"
	LifecycleReady        = "ready"
	LifecycleReauthNeeded = "reauth_needed"
	LifecycleError        = "error"
)

// Unread indications on guilds and channels.
const (
	UnreadRead      = "read"
	UnreadUnread    = "unread"
	UnreadMentioned = "mentioned"
)

// User is the wire shape of a Discord user.
type User struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

// State is the session state object.
type State struct {
	ProtocolVersion   int     `json:"protocol_version"`
	BackendVersion    string  `json:"backend_version"`
	Lifecycle         string  `json:"lifecycle"`
	User              *User   `json:"user"`
	Presence          string  `json:"presence"`
	TotalMentionCount int     `json:"total_mention_count"`
	UnreadDMChannelID *string `json:"unread_dm_channel_id"`
	Generation        int64   `json:"generation"`
	Error             string  `json:"error"`
}

// Guild is the wire shape of a guild.
type Guild struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	IconURL      *string `json:"icon_url"`
	Unread       string  `json:"unread"`
	MentionCount int     `json:"mention_count"`
	Position     int     `json:"position"`
}

// Channel is the wire shape of a guild channel or DM.
type Channel struct {
	ID            string  `json:"id"`
	GuildID       *string `json:"guild_id"`
	Type          string  `json:"type"`
	Name          string  `json:"name"`
	Topic         string  `json:"topic"`
	ParentID      *string `json:"parent_id"`
	Position      int     `json:"position"`
	LastMessageID *string `json:"last_message_id"`
	Unread        string  `json:"unread"`
	MentionCount  int     `json:"mention_count"`
	Muted         bool    `json:"muted"`
	// Recipients is always an array: the DM/group-DM members, empty for guild
	// channels.
	Recipients []User `json:"recipients"`
}

// Request parameter shapes.
type (
	LoginParams struct {
		Token string `json:"token"`
	}
	ListChannelsParams struct {
		GuildID string `json:"guild_id"`
	}
)

// Result shapes.
type (
	HelloResult struct {
		ProtocolVersion int    `json:"protocol_version"`
		BackendVersion  string `json:"backend_version"`
		Engine          string `json:"engine"`
	}
	PingResult struct {
		Pong bool `json:"pong"`
	}
	EmptyResult struct{}
	LoginResult struct {
		User User `json:"user"`
		// KeyringStored is false when the token could not be persisted; the
		// session is still live for this process.
		KeyringStored bool `json:"keyring_stored"`
	}
	ListGuildsResult struct {
		Guilds []Guild `json:"guilds"`
	}
	ListChannelsResult struct {
		Channels []Channel `json:"channels"`
	}
)

// Event shapes.
type (
	StateChangedEvent struct {
		EventHeader
		State State `json:"state"`
	}
	GuildsSyncedEvent struct {
		EventHeader
		// Generation is the session state generation at which this structure
		// was taken; clients discard structure older than what they have seen.
		Generation int64     `json:"generation"`
		Guilds     []Guild   `json:"guilds"`
		DMs        []Channel `json:"dms"`
	}
)

// NewStateChanged wraps a state in its event envelope.
func NewStateChanged(s State) StateChangedEvent {
	return StateChangedEvent{EventHeader: header("state_changed"), State: s}
}

// NewGuildsSynced wraps a structure snapshot in its event envelope. Nil slices
// are normalized to empty arrays so QML never sees null.
func NewGuildsSynced(generation int64, guilds []Guild, dms []Channel) GuildsSyncedEvent {
	if guilds == nil {
		guilds = []Guild{}
	}
	if dms == nil {
		dms = []Channel{}
	}
	return GuildsSyncedEvent{EventHeader: header("guilds_synced"), Generation: generation, Guilds: guilds, DMs: dms}
}

// Hello is the canonical hello result.
func Hello() HelloResult {
	return HelloResult{ProtocolVersion: Version, BackendVersion: BackendVersion, Engine: Engine}
}

// MessageAuthor is the author object on a message.
type MessageAuthor struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
	Bot         bool   `json:"bot"`
}

// ReplyTo describes the message a reply references.
type ReplyTo struct {
	MessageID         string `json:"message_id"`
	AuthorDisplayName string `json:"author_display_name"`
	Preview           string `json:"preview"`
}

// Attachment is a file attached to a message.
type Attachment struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        uint64 `json:"size"`
	URL         string `json:"url"`
	ProxyURL    string `json:"proxy_url"`
	Width       uint   `json:"width"`
	Height      uint   `json:"height"`
	Spoiler     bool   `json:"spoiler"`
}

// Embed is the rendering subset of a Discord embed.
type Embed struct {
	Type         string `json:"type"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	URL          string `json:"url"`
	ImageURL     string `json:"image_url"`
	ThumbnailURL string `json:"thumbnail_url"`
	// Color is the 0xRRGGBB integer, 0 when the embed has none.
	Color int `json:"color"`
}

// Reaction is one emoji's reaction summary.
type Reaction struct {
	// Emoji is the unicode emoji or "name:id" for custom emoji.
	Emoji string `json:"emoji"`
	Count int    `json:"count"`
	Me    bool   `json:"me"`
}

// Message is the wire shape of a message.
type Message struct {
	ID              string        `json:"id"`
	ChannelID       string        `json:"channel_id"`
	GuildID         *string       `json:"guild_id"`
	Author          MessageAuthor `json:"author"`
	Content         string        `json:"content"`
	Timestamp       string        `json:"timestamp"`
	EditedTimestamp *string       `json:"edited_timestamp"`
	Nonce           string        `json:"nonce"`
	ReplyTo         *ReplyTo      `json:"reply_to"`
	Attachments     []Attachment  `json:"attachments"`
	Embeds          []Embed       `json:"embeds"`
	Reactions       []Reaction    `json:"reactions"`
	MentionsSelf    bool          `json:"mentions_self"`
	System          bool          `json:"system"`
}

// Message command parameter shapes.
type (
	OpenChannelParams struct {
		ChannelID string `json:"channel_id"`
	}
	CloseChannelParams struct {
		ChannelID string `json:"channel_id"`
	}
	HistoryParams struct {
		ChannelID string `json:"channel_id"`
		BeforeID  string `json:"before_id"`
		Limit     int    `json:"limit"`
	}
	AckParams struct {
		ChannelID string `json:"channel_id"`
		MessageID string `json:"message_id"`
	}
)

// Message command result shapes.
type (
	OpenChannelResult struct {
		Channel  Channel   `json:"channel"`
		Messages []Message `json:"messages"`
		HasMore  bool      `json:"has_more"`
	}
	HistoryResult struct {
		Messages []Message `json:"messages"`
		HasMore  bool      `json:"has_more"`
	}
)

// Message and read-state event shapes.
type (
	MessageCreateEvent struct {
		EventHeader
		ChannelID   string  `json:"channel_id"`
		GuildID     *string `json:"guild_id"`
		Message     Message `json:"message"`
		Notify      bool    `json:"notify"`
		ChannelName string  `json:"channel_name"`
	}
	MessageUpdateEvent struct {
		EventHeader
		ChannelID string  `json:"channel_id"`
		GuildID   *string `json:"guild_id"`
		Message   Message `json:"message"`
	}
	MessageDeleteEvent struct {
		EventHeader
		ChannelID string  `json:"channel_id"`
		GuildID   *string `json:"guild_id"`
		MessageID string  `json:"message_id"`
	}
	TypingStartEvent struct {
		EventHeader
		ChannelID   string  `json:"channel_id"`
		GuildID     *string `json:"guild_id"`
		UserID      string  `json:"user_id"`
		DisplayName string  `json:"display_name"`
		Timestamp   string  `json:"timestamp"`
	}
	ReadStateChangedEvent struct {
		EventHeader
		ChannelID         string  `json:"channel_id"`
		GuildID           *string `json:"guild_id"`
		Unread            bool    `json:"unread"`
		MentionCount      int     `json:"mention_count"`
		LastReadMessageID *string `json:"last_read_message_id"`
		TotalMentionCount int     `json:"total_mention_count"`
	}
)

// NewMessageCreate wraps a message in its event envelope.
func NewMessageCreate(m Message, notify bool, channelName string) MessageCreateEvent {
	return MessageCreateEvent{EventHeader: header("message_create"), ChannelID: m.ChannelID, GuildID: m.GuildID, Message: m, Notify: notify, ChannelName: channelName}
}

// NewMessageUpdate wraps a full updated message in its event envelope.
func NewMessageUpdate(m Message) MessageUpdateEvent {
	return MessageUpdateEvent{EventHeader: header("message_update"), ChannelID: m.ChannelID, GuildID: m.GuildID, Message: m}
}

// NewMessageDelete builds a message_delete event.
func NewMessageDelete(channelID string, guildID *string, messageID string) MessageDeleteEvent {
	return MessageDeleteEvent{EventHeader: header("message_delete"), ChannelID: channelID, GuildID: guildID, MessageID: messageID}
}

// NewTypingStart builds a typing_start event.
func NewTypingStart(channelID string, guildID *string, userID, displayName, timestamp string) TypingStartEvent {
	return TypingStartEvent{EventHeader: header("typing_start"), ChannelID: channelID, GuildID: guildID, UserID: userID, DisplayName: displayName, Timestamp: timestamp}
}

// NewReadStateChanged builds a read_state_changed event.
func NewReadStateChanged(channelID string, guildID *string, unread bool, mentions int, lastRead *string, total int) ReadStateChangedEvent {
	return ReadStateChangedEvent{EventHeader: header("read_state_changed"), ChannelID: channelID, GuildID: guildID, Unread: unread, MentionCount: mentions, LastReadMessageID: lastRead, TotalMentionCount: total}
}
