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
