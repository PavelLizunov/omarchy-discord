package protocol

import "github.com/mattcalayo/omarchy-discord/backend/internal/redact"

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
	// LastReadMessageID is the account's read marker for the channel (null
	// when it has none), so the client can place the unread divider on open.
	LastReadMessageID *string `json:"last_read_message_id"`
	// Recipients is always an array: the DM/group-DM members, empty for guild
	// channels.
	Recipients []User `json:"recipients"`
	// MessageCount and MemberCount are Discord's approximate thread counters;
	// 0 for non-threads.
	MessageCount int `json:"message_count"`
	MemberCount  int `json:"member_count"`
	// Archived is true for an archived thread. list_channels reports every
	// cached thread including archived ones, while list_threads serves only
	// the active set, so the flag is what lets a client count them the same
	// way. Always false for non-threads.
	Archived bool `json:"archived"`
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

// Phase 2 parameter shapes: messaging, presence, media, upload, QR, config.
type (
	SendParams struct {
		ChannelID string `json:"channel_id"`
		Content   string `json:"content"`
		ReplyTo   string `json:"reply_to"`
		// ReplyMention defaults to true when absent.
		ReplyMention *bool `json:"reply_mention"`
	}
	EditParams struct {
		ChannelID string `json:"channel_id"`
		MessageID string `json:"message_id"`
		Content   string `json:"content"`
	}
	DeleteParams struct {
		ChannelID string `json:"channel_id"`
		MessageID string `json:"message_id"`
	}
	ReactParams struct {
		ChannelID string `json:"channel_id"`
		MessageID string `json:"message_id"`
		Emoji     string `json:"emoji"`
	}
	TypingParams struct {
		ChannelID string `json:"channel_id"`
	}
	SetPresenceParams struct {
		Status string `json:"status"`
	}
	FetchMediaParams struct {
		URL  string `json:"url"`
		Size int    `json:"size"`
	}
	UploadParams struct {
		ChannelID string   `json:"channel_id"`
		Paths     []string `json:"paths"`
		Content   string   `json:"content"`
		ReplyTo   string   `json:"reply_to"`
		Spoiler   bool     `json:"spoiler"`
	}
	SetConfigParams struct {
		// MediaCacheMB is the media cache size cap; nil leaves it unchanged.
		MediaCacheMB *int `json:"media_cache_mb"`
	}
)

// Phase 2 result shapes.
type (
	SendResult struct {
		MessageID string `json:"message_id"`
		Nonce     string `json:"nonce"`
	}
	FetchMediaResult struct {
		Cached bool `json:"cached"`
		// Path is present only on a hit.
		Path string `json:"path,omitempty"`
	}
)

// QRUser is the account that scanned a QR code (qr_scanned).
type QRUser struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	Discriminator string `json:"discriminator"`
	AvatarHash    string `json:"avatar_hash"`
}

// QR cancel reasons.
const (
	QRReasonDeclined  = "declined"
	QRReasonExpired   = "expired"
	QRReasonCancelled = "cancelled"
	QRReasonError     = "error"
)

// Phase 2 event shapes.
type (
	MediaReadyEvent struct {
		EventHeader
		URL   string `json:"url"`
		OK    bool   `json:"ok"`
		Path  string `json:"path"`
		Error string `json:"error"`
	}
	UploadProgressEvent struct {
		EventHeader
		UploadID   int64  `json:"upload_id"`
		Filename   string `json:"filename"`
		BytesSent  int64  `json:"bytes_sent"`
		BytesTotal int64  `json:"bytes_total"`
	}
	QRCodeEvent struct {
		EventHeader
		URL         string `json:"url"`
		Fingerprint string `json:"fingerprint"`
		ExpiresInMS int64  `json:"expires_in_ms"`
		// ImagePath is a PNG rendering of URL, written for the panel; it is
		// removed when the flow ends.
		ImagePath string `json:"image_path"`
	}
	QRScannedEvent struct {
		EventHeader
		User QRUser `json:"user"`
	}
	QRApprovedEvent struct {
		EventHeader
	}
	QRCancelledEvent struct {
		EventHeader
		Reason string `json:"reason"`
		Error  string `json:"error"`
	}
)

// NewMediaReady builds a media_ready event; errText is redacted.
func NewMediaReady(url string, ok bool, path, errText string) MediaReadyEvent {
	return MediaReadyEvent{EventHeader: header("media_ready"), URL: url, OK: ok, Path: path, Error: redact.Redact(errText)}
}

// NewUploadProgress builds an upload_progress event.
func NewUploadProgress(uploadID int64, filename string, sent, total int64) UploadProgressEvent {
	return UploadProgressEvent{EventHeader: header("upload_progress"), UploadID: uploadID, Filename: filename, BytesSent: sent, BytesTotal: total}
}

// NewQRCode builds a qr_code event.
func NewQRCode(url, fingerprint string, expiresInMS int64, imagePath string) QRCodeEvent {
	return QRCodeEvent{EventHeader: header("qr_code"), URL: url, Fingerprint: fingerprint, ExpiresInMS: expiresInMS, ImagePath: imagePath}
}

// NewQRScanned builds a qr_scanned event.
func NewQRScanned(u QRUser) QRScannedEvent {
	return QRScannedEvent{EventHeader: header("qr_scanned"), User: u}
}

// NewQRApproved builds a qr_approved event.
func NewQRApproved() QRApprovedEvent { return QRApprovedEvent{EventHeader: header("qr_approved")} }

// NewQRCancelled builds a qr_cancelled event; errText is redacted.
func NewQRCancelled(reason, errText string) QRCancelledEvent {
	return QRCancelledEvent{EventHeader: header("qr_cancelled"), Reason: reason, Error: redact.Redact(errText)}
}

// Phase 3 parameter shapes: quick switcher, threads, member list, emoji.
type (
	QuickSwitchParams struct {
		Query string `json:"query"`
		// Limit defaults to 20 when absent or non-positive.
		Limit int `json:"limit"`
	}
	ListThreadsParams struct {
		ChannelID string `json:"channel_id"`
	}
	SubscribeMembersParams struct {
		ChannelID string `json:"channel_id"`
	}
)

// QuickSwitchEntry is one switcher candidate.
type QuickSwitchEntry struct {
	Channel Channel `json:"channel"`
	// GuildName is null for DMs.
	GuildName *string `json:"guild_name"`
	// LastMessagePreview is the newest cached message collapsed to one line,
	// "" when nothing is cached (never fetched).
	LastMessagePreview string  `json:"last_message_preview"`
	Score              float64 `json:"score"`
}

// MemberGroup is one section of a member list: "online", "offline", or a
// hoisted role id.
type MemberGroup struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Member is one row of a member list.
type Member struct {
	User    MessageAuthor `json:"user"`
	GroupID string        `json:"group_id"`
	// Status is online, idle, dnd, or offline.
	Status string `json:"status"`
	// Activity is a one-line rendering of the primary activity, "" if none.
	Activity string `json:"activity"`
}

// Emoji is a custom guild emoji usable in reactions.
type Emoji struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Animated bool   `json:"animated"`
	// URL is the CDN URL (gif for animated); fetch_media accepts a size hint.
	URL string `json:"url"`
}

// GuildEmoji groups a guild's custom emoji.
type GuildEmoji struct {
	GuildID   string  `json:"guild_id"`
	GuildName string  `json:"guild_name"`
	Emoji     []Emoji `json:"emoji"`
}

// Phase 3 result shapes.
type (
	QuickSwitchResult struct {
		Entries []QuickSwitchEntry `json:"entries"`
	}
	ListThreadsResult struct {
		Threads []Channel `json:"threads"`
	}
	ListEmojiResult struct {
		Guilds []GuildEmoji `json:"guilds"`
	}
)

// Channel change kinds carried by channel_update.
const (
	ChannelChangeCreate = "create"
	ChannelChangeUpdate = "update"
	ChannelChangeDelete = "delete"
)

// Phase 3 event shapes.
type (
	ChannelUpdateEvent struct {
		EventHeader
		Change  string  `json:"change"`
		Channel Channel `json:"channel"`
	}
	MemberListUpdateEvent struct {
		EventHeader
		ChannelID string        `json:"channel_id"`
		GuildID   *string       `json:"guild_id"`
		Groups    []MemberGroup `json:"groups"`
		Members   []Member      `json:"members"`
	}
	PresenceUpdateEvent struct {
		EventHeader
		UserID   string `json:"user_id"`
		Status   string `json:"status"`
		Activity string `json:"activity"`
	}
)

// NewChannelUpdate builds a channel_update event.
func NewChannelUpdate(change string, ch Channel) ChannelUpdateEvent {
	return ChannelUpdateEvent{EventHeader: header("channel_update"), Change: change, Channel: ch}
}

// NewMemberListUpdate builds a member_list_update event; nil slices become
// empty arrays.
func NewMemberListUpdate(channelID string, guildID *string, groups []MemberGroup, members []Member) MemberListUpdateEvent {
	if groups == nil {
		groups = []MemberGroup{}
	}
	if members == nil {
		members = []Member{}
	}
	return MemberListUpdateEvent{EventHeader: header("member_list_update"), ChannelID: channelID, GuildID: guildID, Groups: groups, Members: members}
}

// NewPresenceUpdate builds a presence_update event.
func NewPresenceUpdate(userID, status, activity string) PresenceUpdateEvent {
	return PresenceUpdateEvent{EventHeader: header("presence_update"), UserID: userID, Status: status, Activity: activity}
}
