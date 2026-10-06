package protocol

import "github.com/mattcalayo/omarchy-discord/backend/internal/redact"

const (
	LifecycleStarting     = "starting"
	LifecycleLoggedOut    = "logged_out"
	LifecycleQRPending    = "qr_pending"
	LifecycleConnecting   = "connecting"
	LifecycleReady        = "ready"
	LifecycleReauthNeeded = "reauth_needed"
	LifecycleError        = "error"
)

const (
	UnreadRead      = "read"
	UnreadUnread    = "unread"
	UnreadMentioned = "mentioned"
)

type User struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

type State struct {
	ProtocolVersion   int        `json:"protocol_version"`
	BackendVersion    string     `json:"backend_version"`
	Lifecycle         string     `json:"lifecycle"`
	User              *User      `json:"user"`
	Presence          string     `json:"presence"`
	TotalMentionCount int        `json:"total_mention_count"`
	UnreadDMChannelID *string    `json:"unread_dm_channel_id"`
	Generation        int64      `json:"generation"`
	Error             string     `json:"error"`
	Voice             VoiceState `json:"voice"`
}

const (
	VoiceIdle       = "idle"
	VoiceConnecting = "connecting"
	VoiceConnected  = "connected"
	VoiceError      = "error"
)

type VoiceState struct {
	Status    string  `json:"status"`
	GuildID   *string `json:"guild_id"`
	ChannelID *string `json:"channel_id"`
	Muted     bool    `json:"muted"`
	Deafened  bool    `json:"deafened"`
	Error     string  `json:"error"`
}

func IdleVoice() VoiceState { return VoiceState{Status: VoiceIdle} }

type Guild struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	IconURL      *string `json:"icon_url"`
	Unread       string  `json:"unread"`
	MentionCount int     `json:"mention_count"`
	Position     int     `json:"position"`
}

type Channel struct {
	ID                string  `json:"id"`
	GuildID           *string `json:"guild_id"`
	Type              string  `json:"type"`
	Name              string  `json:"name"`
	Topic             string  `json:"topic"`
	ParentID          *string `json:"parent_id"`
	Position          int     `json:"position"`
	LastMessageID     *string `json:"last_message_id"`
	Unread            string  `json:"unread"`
	MentionCount      int     `json:"mention_count"`
	Muted             bool    `json:"muted"`
	LastReadMessageID *string `json:"last_read_message_id"`
	Recipients        []User  `json:"recipients"`
	MessageCount      int     `json:"message_count"`
	MemberCount       int     `json:"member_count"`
	Archived          bool    `json:"archived"`
}

type (
	LoginParams struct {
		Token string `json:"token"`
	}
	ListChannelsParams struct {
		GuildID string `json:"guild_id"`
	}
)

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
		User          User `json:"user"`
		KeyringStored bool `json:"keyring_stored"`
	}
	ListGuildsResult struct {
		Guilds []Guild `json:"guilds"`
	}
	ListChannelsResult struct {
		Channels []Channel `json:"channels"`
	}
)

type (
	StateChangedEvent struct {
		EventHeader
		State State `json:"state"`
	}
	GuildsSyncedEvent struct {
		EventHeader
		Generation int64     `json:"generation"`
		Guilds     []Guild   `json:"guilds"`
		DMs        []Channel `json:"dms"`
	}
)

func NewStateChanged(s State) StateChangedEvent {
	return StateChangedEvent{EventHeader: header("state_changed"), State: s}
}

func NewGuildsSynced(generation int64, guilds []Guild, dms []Channel) GuildsSyncedEvent {
	if guilds == nil {
		guilds = []Guild{}
	}
	if dms == nil {
		dms = []Channel{}
	}
	return GuildsSyncedEvent{EventHeader: header("guilds_synced"), Generation: generation, Guilds: guilds, DMs: dms}
}

func Hello() HelloResult {
	return HelloResult{ProtocolVersion: Version, BackendVersion: BackendVersion, Engine: Engine}
}

type MessageAuthor struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
	Bot         bool   `json:"bot"`
}

type ReplyTo struct {
	MessageID         string `json:"message_id"`
	AuthorDisplayName string `json:"author_display_name"`
	Preview           string `json:"preview"`
}

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

type Embed struct {
	Type         string `json:"type"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	URL          string `json:"url"`
	ImageURL     string `json:"image_url"`
	ThumbnailURL string `json:"thumbnail_url"`
	Color        int    `json:"color"`
}

type Reaction struct {
	Emoji string `json:"emoji"`
	Count int    `json:"count"`
	Me    bool   `json:"me"`
}

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

func NewMessageCreate(m Message, notify bool, channelName string) MessageCreateEvent {
	return MessageCreateEvent{EventHeader: header("message_create"), ChannelID: m.ChannelID, GuildID: m.GuildID, Message: m, Notify: notify, ChannelName: channelName}
}

func NewMessageUpdate(m Message) MessageUpdateEvent {
	return MessageUpdateEvent{EventHeader: header("message_update"), ChannelID: m.ChannelID, GuildID: m.GuildID, Message: m}
}

func NewMessageDelete(channelID string, guildID *string, messageID string) MessageDeleteEvent {
	return MessageDeleteEvent{EventHeader: header("message_delete"), ChannelID: channelID, GuildID: guildID, MessageID: messageID}
}

func NewTypingStart(channelID string, guildID *string, userID, displayName, timestamp string) TypingStartEvent {
	return TypingStartEvent{EventHeader: header("typing_start"), ChannelID: channelID, GuildID: guildID, UserID: userID, DisplayName: displayName, Timestamp: timestamp}
}

func NewReadStateChanged(channelID string, guildID *string, unread bool, mentions int, lastRead *string, total int) ReadStateChangedEvent {
	return ReadStateChangedEvent{EventHeader: header("read_state_changed"), ChannelID: channelID, GuildID: guildID, Unread: unread, MentionCount: mentions, LastReadMessageID: lastRead, TotalMentionCount: total}
}

type (
	SendParams struct {
		ChannelID    string `json:"channel_id"`
		Content      string `json:"content"`
		ReplyTo      string `json:"reply_to"`
		ReplyMention *bool  `json:"reply_mention"`
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
		MediaCacheMB *int `json:"media_cache_mb"`
	}
)

type (
	SendResult struct {
		MessageID string `json:"message_id"`
		Nonce     string `json:"nonce"`
	}
	FetchMediaResult struct {
		Cached bool   `json:"cached"`
		Path   string `json:"path,omitempty"`
	}
)

type QRUser struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	Discriminator string `json:"discriminator"`
	AvatarHash    string `json:"avatar_hash"`
}

const (
	QRReasonDeclined  = "declined"
	QRReasonExpired   = "expired"
	QRReasonCancelled = "cancelled"
	QRReasonError     = "error"
)

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
		ImagePath   string `json:"image_path"`
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

func NewMediaReady(url string, ok bool, path, errText string) MediaReadyEvent {
	return MediaReadyEvent{EventHeader: header("media_ready"), URL: url, OK: ok, Path: path, Error: redact.Redact(errText)}
}

func NewUploadProgress(uploadID int64, filename string, sent, total int64) UploadProgressEvent {
	return UploadProgressEvent{EventHeader: header("upload_progress"), UploadID: uploadID, Filename: filename, BytesSent: sent, BytesTotal: total}
}

func NewQRCode(url, fingerprint string, expiresInMS int64, imagePath string) QRCodeEvent {
	return QRCodeEvent{EventHeader: header("qr_code"), URL: url, Fingerprint: fingerprint, ExpiresInMS: expiresInMS, ImagePath: imagePath}
}

func NewQRScanned(u QRUser) QRScannedEvent {
	return QRScannedEvent{EventHeader: header("qr_scanned"), User: u}
}

func NewQRApproved() QRApprovedEvent { return QRApprovedEvent{EventHeader: header("qr_approved")} }

func NewQRCancelled(reason, errText string) QRCancelledEvent {
	return QRCancelledEvent{EventHeader: header("qr_cancelled"), Reason: reason, Error: redact.Redact(errText)}
}

type (
	QuickSwitchParams struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	ListThreadsParams struct {
		ChannelID string `json:"channel_id"`
	}
	SubscribeMembersParams struct {
		ChannelID string `json:"channel_id"`
	}
)

type QuickSwitchEntry struct {
	Channel            Channel `json:"channel"`
	GuildName          *string `json:"guild_name"`
	LastMessagePreview string  `json:"last_message_preview"`
	Score              float64 `json:"score"`
}

type MemberGroup struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type Member struct {
	User     MessageAuthor `json:"user"`
	GroupID  string        `json:"group_id"`
	Status   string        `json:"status"`
	Activity string        `json:"activity"`
}

type Emoji struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Animated bool   `json:"animated"`
	URL      string `json:"url"`
}

type GuildEmoji struct {
	GuildID   string  `json:"guild_id"`
	GuildName string  `json:"guild_name"`
	Emoji     []Emoji `json:"emoji"`
}

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

const (
	ChannelChangeCreate = "create"
	ChannelChangeUpdate = "update"
	ChannelChangeDelete = "delete"
)

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

func NewChannelUpdate(change string, ch Channel) ChannelUpdateEvent {
	return ChannelUpdateEvent{EventHeader: header("channel_update"), Change: change, Channel: ch}
}

func NewMemberListUpdate(channelID string, guildID *string, groups []MemberGroup, members []Member) MemberListUpdateEvent {
	if groups == nil {
		groups = []MemberGroup{}
	}
	if members == nil {
		members = []Member{}
	}
	return MemberListUpdateEvent{EventHeader: header("member_list_update"), ChannelID: channelID, GuildID: guildID, Groups: groups, Members: members}
}

func NewPresenceUpdate(userID, status, activity string) PresenceUpdateEvent {
	return PresenceUpdateEvent{EventHeader: header("presence_update"), UserID: userID, Status: status, Activity: activity}
}

type (
	VoiceJoinParams struct {
		GuildID   string `json:"guild_id"`
		ChannelID string `json:"channel_id"`
	}
	VoiceSetParams struct {
		Muted    *bool `json:"muted"`
		Deafened *bool `json:"deafened"`
	}
)

type VoiceChannelMembers struct {
	ChannelID string `json:"channel_id"`
	Users     []User `json:"users"`
}

type (
	VoiceMembersEvent struct {
		EventHeader
		GuildID  string                `json:"guild_id"`
		Channels []VoiceChannelMembers `json:"channels"`
	}
	VoiceSpeakingEvent struct {
		EventHeader
		UserID   string `json:"user_id"`
		Speaking bool   `json:"speaking"`
	}
)

func NewVoiceMembers(guildID string, channels []VoiceChannelMembers) VoiceMembersEvent {
	if channels == nil {
		channels = []VoiceChannelMembers{}
	}
	for i := range channels {
		if channels[i].Users == nil {
			channels[i].Users = []User{}
		}
	}
	return VoiceMembersEvent{EventHeader: header("voice_members"), GuildID: guildID, Channels: channels}
}

func NewVoiceSpeaking(userID string, speaking bool) VoiceSpeakingEvent {
	return VoiceSpeakingEvent{EventHeader: header("voice_speaking"), UserID: userID, Speaking: speaking}
}
