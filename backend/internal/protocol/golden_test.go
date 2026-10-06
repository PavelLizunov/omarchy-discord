package protocol

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden fixtures")

func str(s string) *string { return &s }

type typedResponse[T any] struct {
	Type   string `json:"type"`
	V      int    `json:"v"`
	ID     int64  `json:"id"`
	OK     bool   `json:"ok"`
	Result *T     `json:"result,omitempty"`
	Err    *Error `json:"error,omitempty"`
}

var goldens = []struct {
	name  string
	value any
	rt    any
}{
	{"response_hello", OKResponse(1, Hello()), &typedResponse[HelloResult]{}},
	{"response_ping", OKResponse(2, PingResult{Pong: true}), &typedResponse[PingResult]{}},
	{"response_get_state", OKResponse(3, State{
		ProtocolVersion: 1, BackendVersion: BackendVersion, Lifecycle: LifecycleReady,
		User:     &User{ID: "183627919046737920", Username: "m", DisplayName: "m", AvatarURL: "https://cdn.discordapp.com/avatars/183627919046737920/a.png"},
		Presence: "online", TotalMentionCount: 3, UnreadDMChannelID: str("1049931213073821696"), Generation: 7, Error: "",
		Voice: VoiceState{Status: VoiceConnected, GuildID: str("1000000000000000001"), ChannelID: str("1000000000000000012"), Muted: true, Deafened: false, Error: ""},
	}), &typedResponse[State]{}},
	{"response_get_state_logged_out", OKResponse(3, State{
		ProtocolVersion: 1, BackendVersion: BackendVersion, Lifecycle: LifecycleLoggedOut,
		User: nil, Presence: "", TotalMentionCount: 0, UnreadDMChannelID: nil, Generation: 1, Error: "", Voice: IdleVoice(),
	}), &typedResponse[State]{}},
	{"response_login", OKResponse(4, LoginResult{User: User{ID: "183627919046737920", Username: "m", DisplayName: "m", AvatarURL: "https://cdn.discordapp.com/avatars/183627919046737920/a.png"}, KeyringStored: true}), &typedResponse[LoginResult]{}},
	{"response_logout", OKResponse(5, EmptyResult{}), &typedResponse[EmptyResult]{}},
	{"response_list_guilds", OKResponse(6, ListGuildsResult{Guilds: []Guild{
		{ID: "1000000000000000001", Name: "Omarchy", IconURL: str("https://cdn.discordapp.com/icons/1000000000000000001/abc.png"), Unread: UnreadMentioned, MentionCount: 2, Position: 0},
		{ID: "1000000000000000002", Name: "Quiet", IconURL: nil, Unread: UnreadRead, MentionCount: 0, Position: 1},
	}}), &typedResponse[ListGuildsResult]{}},
	{"response_list_channels", OKResponse(7, ListChannelsResult{Channels: []Channel{
		{ID: "1000000000000000010", GuildID: str("1000000000000000001"), Type: "category", Name: "General", Topic: "", ParentID: nil, Position: 0, LastMessageID: nil, Unread: UnreadRead, MentionCount: 0, Muted: false, Recipients: []User{}},
		{ID: "1000000000000000011", GuildID: str("1000000000000000001"), Type: "text", Name: "general", Topic: "chat", ParentID: str("1000000000000000010"), Position: 0, LastMessageID: str("1000000000000000099"), Unread: UnreadMentioned, MentionCount: 2, Muted: false, Recipients: []User{}, LastReadMessageID: str("1000000000000000090")},
	}}), &typedResponse[ListChannelsResult]{}},
	{"response_list_dms", OKResponse(8, ListChannelsResult{Channels: []Channel{
		{ID: "1049931213073821696", GuildID: nil, Type: "dm", Name: "ada", Topic: "", ParentID: nil, Position: 0, LastMessageID: str("1049931302442426390"), Unread: UnreadUnread, MentionCount: 0, Muted: false,
			Recipients: []User{{ID: "2000000000000000001", Username: "ada", DisplayName: "ada", AvatarURL: "https://cdn.discordapp.com/avatars/2000000000000000001/b.png"}}},
	}}), &typedResponse[ListChannelsResult]{}},
	{"response_error", ErrResponse(9, &Error{Code: CodeUnknownChannel, Message: "channel is not accessible"}), &typedResponse[struct{}]{}},
	{"response_invalid_request", ErrResponse(0, &Error{Code: CodeInvalidRequest, Message: "malformed request"}), &typedResponse[struct{}]{}},
	{"event_state_changed", NewStateChanged(State{
		ProtocolVersion: 1, BackendVersion: BackendVersion, Lifecycle: LifecycleConnecting,
		User: nil, Presence: "", TotalMentionCount: 0, UnreadDMChannelID: nil, Generation: 2, Error: "", Voice: IdleVoice(),
	}), &StateChangedEvent{}},
	{"event_guilds_synced", NewGuildsSynced(7,
		[]Guild{{ID: "1000000000000000001", Name: "Omarchy", IconURL: nil, Unread: UnreadUnread, MentionCount: 0, Position: 0}},
		[]Channel{{ID: "1049931213073821696", GuildID: nil, Type: "dm", Name: "ada", Topic: "", ParentID: nil, Position: 0, LastMessageID: str("1049931302442426390"), Unread: UnreadRead, MentionCount: 0, Muted: false,
			Recipients: []User{{ID: "2000000000000000001", Username: "ada", DisplayName: "ada", AvatarURL: ""}}, LastReadMessageID: str("1049931302442426390")}},
	), &GuildsSyncedEvent{}},
	{"event_guilds_synced_empty", NewGuildsSynced(3, nil, nil), &GuildsSyncedEvent{}},
	{"response_open_channel", OKResponse(20, OpenChannelResult{
		Channel:  Channel{ID: "1049931213073821696", GuildID: str("1000000000000000001"), Type: "text", Name: "general", Topic: "chat", ParentID: str("1000000000000000010"), Position: 0, LastMessageID: str("1049931339989602304"), Unread: UnreadRead, MentionCount: 0, Muted: false, Recipients: []User{}, LastReadMessageID: str("1049931339989602304")},
		Messages: []Message{sampleReply, sampleMessage},
		HasMore:  true,
	}), &typedResponse[OpenChannelResult]{}},
	{"response_close_channel", OKResponse(21, EmptyResult{}), &typedResponse[EmptyResult]{}},
	{"response_history", OKResponse(22, HistoryResult{Messages: []Message{sampleSystem}, HasMore: false}), &typedResponse[HistoryResult]{}},
	{"response_history_empty", OKResponse(22, HistoryResult{Messages: []Message{}, HasMore: false}), &typedResponse[HistoryResult]{}},
	{"response_ack", OKResponse(23, EmptyResult{}), &typedResponse[EmptyResult]{}},
	{"response_empty_dm_refused", ErrResponse(24, &Error{Code: CodeEmptyDMRefused, Message: "refusing to open a DM with no history; send a message from the official client first"}), &typedResponse[struct{}]{}},
	{"event_message_create", NewMessageCreate(sampleMessage, true, "general"), &MessageCreateEvent{}},
	{"event_message_create_own_echo", NewMessageCreate(sampleOwn, false, "general"), &MessageCreateEvent{}},
	{"event_message_update", NewMessageUpdate(sampleMessage), &MessageUpdateEvent{}},
	{"event_message_delete", NewMessageDelete("1049931213073821696", str("1000000000000000001"), "1049931339989602304"), &MessageDeleteEvent{}},
	{"event_message_delete_dm", NewMessageDelete("1049931213073821696", nil, "1049931339989602304"), &MessageDeleteEvent{}},
	{"event_typing_start", NewTypingStart("1049931213073821696", str("1000000000000000001"), "2000000000000000001", "ada", "2026-08-20T14:03:22.000Z"), &TypingStartEvent{}},
	{"event_read_state_changed", NewReadStateChanged("1049931213073821696", str("1000000000000000001"), true, 2, str("1049931302442426390"), 5), &ReadStateChangedEvent{}},
	{"event_read_state_changed_dm_ack", NewReadStateChanged("1049931213073821696", nil, false, 0, str("1049931339989602304"), 0), &ReadStateChangedEvent{}},
	{"response_send", OKResponse(31, SendResult{MessageID: "1049931339989602304", Nonce: "a1b2c3d4e5f60718"}), &typedResponse[SendResult]{}},
	{"response_edit", OKResponse(32, EmptyResult{}), &typedResponse[EmptyResult]{}},
	{"response_delete", OKResponse(33, EmptyResult{}), &typedResponse[EmptyResult]{}},
	{"response_react", OKResponse(34, EmptyResult{}), &typedResponse[EmptyResult]{}},
	{"response_typing", OKResponse(35, EmptyResult{}), &typedResponse[EmptyResult]{}},
	{"response_set_presence", OKResponse(36, EmptyResult{}), &typedResponse[EmptyResult]{}},
	{"response_forbidden", ErrResponse(32, &Error{Code: CodeForbidden, Message: "only own messages can be edited"}), &typedResponse[struct{}]{}},
	{"response_rate_limited", ErrResponse(31, &Error{Code: CodeRateLimited, Message: "rate limited: retry after 2.5s"}), &typedResponse[struct{}]{}},
	{"response_fetch_media_hit", OKResponse(40, FetchMediaResult{Cached: true, Path: "/home/m/.cache/omarchy-discord/media/ab12cd34ef56ab12cd34ef56ab12cd34ef56ab12cd34ef56ab12cd34ef56ab12.png"}), &typedResponse[FetchMediaResult]{}},
	{"response_fetch_media_miss", OKResponse(41, FetchMediaResult{Cached: false}), &typedResponse[FetchMediaResult]{}},
	{"response_media_error", ErrResponse(42, &Error{Code: CodeMediaError, Message: "disallowed host"}), &typedResponse[struct{}]{}},
	{"response_set_config", OKResponse(43, EmptyResult{}), &typedResponse[EmptyResult]{}},
	{"response_upload", OKResponse(44, SendResult{MessageID: "1049931401540221011", Nonce: "e5f6a7b8c9d0e1f2"}), &typedResponse[SendResult]{}},
	{"response_upload_too_large", ErrResponse(44, &Error{Code: CodeUploadTooLarge, Message: "52428801 bytes exceeds the 10485760 byte upload limit"}), &typedResponse[struct{}]{}},
	{"response_start_qr_login", OKResponse(50, EmptyResult{}), &typedResponse[EmptyResult]{}},
	{"response_cancel_qr_login", OKResponse(51, EmptyResult{}), &typedResponse[EmptyResult]{}},
	{"response_qr_unavailable", ErrResponse(50, &Error{Code: CodeQRUnavailable, Message: "a QR login is already in progress"}), &typedResponse[struct{}]{}},
	{"event_media_ready", NewMediaReady("https://cdn.discordapp.com/attachments/1049931213073821696/1049931339989602305/shot-1.png", true, "/home/m/.cache/omarchy-discord/media/ab12cd34ef56ab12cd34ef56ab12cd34ef56ab12cd34ef56ab12cd34ef56ab12.png", ""), &MediaReadyEvent{}},
	{"event_media_ready_failed", NewMediaReady("https://cdn.discordapp.com/attachments/1049931213073821696/1049931339989602305/gone.png", false, "", "http 404"), &MediaReadyEvent{}},
	{"event_upload_progress", NewUploadProgress(44, "shot-1.png", 262144, 1048576), &UploadProgressEvent{}},
	{"event_upload_progress_final", NewUploadProgress(44, "shot-1.png", 1048576, 1048576), &UploadProgressEvent{}},
	{"event_qr_code", NewQRCode("https://discord.com/ra/0123456789abcdef0123456789abcdef0123456789ab", "0123456789abcdef0123456789abcdef0123456789ab", 120000, "/run/user/1000/omarchy-discord/qr.png"), &QRCodeEvent{}},
	{"event_qr_scanned", NewQRScanned(QRUser{ID: "183627919046737920", Username: "m", Discriminator: "0", AvatarHash: "a1b2c3"}), &QRScannedEvent{}},
	{"event_qr_approved", NewQRApproved(), &QRApprovedEvent{}},
	{"event_qr_cancelled_declined", NewQRCancelled(QRReasonDeclined, ""), &QRCancelledEvent{}},
	{"event_qr_cancelled_expired", NewQRCancelled(QRReasonExpired, ""), &QRCancelledEvent{}},
	{"event_qr_cancelled_error", NewQRCancelled(QRReasonError, "remoteauth: ticket exchange failed: http 400"), &QRCancelledEvent{}},
	{"response_quick_switch", OKResponse(60, QuickSwitchResult{Entries: []QuickSwitchEntry{
		{Channel: sampleThread, GuildName: str("Omarchy"), LastMessagePreview: "shall we?", Score: 12.97},
		{Channel: Channel{ID: "1049931213073821696", GuildID: nil, Type: "dm", Name: "ada", Topic: "", ParentID: nil, Position: 0, LastMessageID: str("1049931302442426390"), Unread: UnreadRead, MentionCount: 0, Muted: false,
			Recipients: []User{{ID: "2000000000000000001", Username: "ada", DisplayName: "ada", AvatarURL: ""}}}, GuildName: nil, LastMessagePreview: "", Score: 7},
	}}), &typedResponse[QuickSwitchResult]{}},
	{"response_quick_switch_empty", OKResponse(60, QuickSwitchResult{Entries: []QuickSwitchEntry{}}), &typedResponse[QuickSwitchResult]{}},
	{"response_list_threads", OKResponse(61, ListThreadsResult{Threads: []Channel{sampleThread}}), &typedResponse[ListThreadsResult]{}},
	{"response_list_threads_empty", OKResponse(61, ListThreadsResult{Threads: []Channel{}}), &typedResponse[ListThreadsResult]{}},
	{"response_subscribe_members", OKResponse(62, EmptyResult{}), &typedResponse[EmptyResult]{}},
	{"response_unsubscribe_members", OKResponse(63, EmptyResult{}), &typedResponse[EmptyResult]{}},
	{"response_list_emoji", OKResponse(64, ListEmojiResult{Guilds: []GuildEmoji{{GuildID: "1000000000000000001", GuildName: "Omarchy", Emoji: []Emoji{
		{ID: "1000000000000000099", Name: "omarchy", Animated: false, URL: "https://cdn.discordapp.com/emojis/1000000000000000099.png"},
		{ID: "1000000000000000098", Name: "partyblob", Animated: true, URL: "https://cdn.discordapp.com/emojis/1000000000000000098.gif"},
	}}}}), &typedResponse[ListEmojiResult]{}},
	{"response_list_emoji_empty", OKResponse(64, ListEmojiResult{Guilds: []GuildEmoji{}}), &typedResponse[ListEmojiResult]{}},
	{"event_channel_update_thread_create", NewChannelUpdate(ChannelChangeCreate, sampleThread), &ChannelUpdateEvent{}},
	{"event_channel_update_thread_archived", NewChannelUpdate(ChannelChangeUpdate, archivedThread), &ChannelUpdateEvent{}},
	{"event_channel_update_delete", NewChannelUpdate(ChannelChangeDelete, Channel{ID: "1049931500000000000", GuildID: str("1000000000000000001"), Type: "thread", Name: "", Topic: "", ParentID: str("1049931213073821696"), Position: 0, LastMessageID: nil, Unread: UnreadRead, MentionCount: 0, Muted: false, Recipients: []User{}}), &ChannelUpdateEvent{}},
	{"event_member_list_update", NewMemberListUpdate("1049931213073821696", str("1000000000000000001"),
		[]MemberGroup{{ID: "1000000000000000050", Name: "Admins", Count: 1}, {ID: "online", Name: "Online", Count: 1}, {ID: "offline", Name: "Offline", Count: 1}},
		[]Member{
			{User: sampleAuthor, GroupID: "1000000000000000050", Status: "online", Activity: "Playing Factorio"},
			{User: MessageAuthor{ID: "183627919046737920", Username: "m", DisplayName: "m", AvatarURL: "https://cdn.discordapp.com/avatars/183627919046737920/a.png?size=64", Bot: false}, GroupID: "online", Status: "dnd", Activity: ""},
			{User: MessageAuthor{ID: "2000000000000000002", Username: "lin", DisplayName: "lin", AvatarURL: "?size=64", Bot: true}, GroupID: "offline", Status: "offline", Activity: ""},
		}), &MemberListUpdateEvent{}},
	{"event_member_list_update_dm", NewMemberListUpdate("1049931213073821696", nil,
		[]MemberGroup{{ID: "online", Name: "Online", Count: 1}},
		[]Member{{User: sampleAuthor, GroupID: "online", Status: "idle", Activity: "🌙 sleepy"}}), &MemberListUpdateEvent{}},
	{"event_member_list_update_empty", NewMemberListUpdate("1049931213073821696", str("1000000000000000001"), nil, nil), &MemberListUpdateEvent{}},
	{"event_presence_update", NewPresenceUpdate("2000000000000000001", "idle", "Listening to Spotify"), &PresenceUpdateEvent{}},
	{"event_presence_update_offline", NewPresenceUpdate("2000000000000000001", "offline", ""), &PresenceUpdateEvent{}},
	{"event_voice_members", NewVoiceMembers("1000000000000000001", []VoiceChannelMembers{
		{ChannelID: "1000000000000000012", Users: []User{
			{ID: "183627919046737920", Username: "m", DisplayName: "m", AvatarURL: "https://cdn.discordapp.com/avatars/183627919046737920/a.png"},
			{ID: "2000000000000000001", Username: "ada", DisplayName: "Ada", AvatarURL: "https://cdn.discordapp.com/avatars/2000000000000000001/b.png"},
		}},
		{ChannelID: "1000000000000000013", Users: []User{
			{ID: "2000000000000000002", Username: "lin", DisplayName: "lin", AvatarURL: ""},
		}},
	}), &VoiceMembersEvent{}},
	{"event_voice_members_empty", NewVoiceMembers("1000000000000000001", nil), &VoiceMembersEvent{}},
	{"event_voice_speaking", NewVoiceSpeaking("2000000000000000001", true), &VoiceSpeakingEvent{}},
}

var archivedThread = func() Channel {
	c := sampleThread
	c.Archived = true
	return c
}()

var sampleThread = Channel{ID: "1049931500000000000", GuildID: str("1000000000000000001"), Type: "thread", Name: "release planning", Topic: "", ParentID: str("1049931213073821696"), Position: 0, LastMessageID: str("1049931339989602304"), Unread: UnreadUnread, MentionCount: 0, Muted: false, LastReadMessageID: str("1049931302442426390"), Recipients: []User{}, MessageCount: 42, MemberCount: 5}

var (
	sampleAuthor = MessageAuthor{ID: "2000000000000000001", Username: "ada", DisplayName: "Ada", AvatarURL: "https://cdn.discordapp.com/avatars/2000000000000000001/b.png?size=64", Bot: false}
	sampleReply  = Message{
		ID: "1049931302442426390", ChannelID: "1049931213073821696", GuildID: str("1000000000000000001"), Author: sampleAuthor,
		Content: "shall we?", Timestamp: "2026-08-20T14:03:10.004Z", EditedTimestamp: nil, Nonce: "", ReplyTo: nil,
		Attachments: []Attachment{}, Embeds: []Embed{}, Reactions: []Reaction{}, MentionsSelf: false, System: false,
	}
	sampleMessage = Message{
		ID: "1049931339989602304", ChannelID: "1049931213073821696", GuildID: str("1000000000000000001"), Author: sampleAuthor,
		Content: "on my way <@183627919046737920> **now**", Timestamp: "2026-08-20T14:03:22.117Z", EditedTimestamp: str("2026-08-20T14:04:01.000Z"), Nonce: "",
		ReplyTo:      &ReplyTo{MessageID: "1049931302442426390", AuthorDisplayName: "Ada", Preview: "shall we?"},
		Attachments:  []Attachment{{ID: "1049931339989602305", Filename: "SPOILER_shot-1.png", ContentType: "image/png", Size: 1048576, URL: "https://cdn.discordapp.com/attachments/1049931213073821696/1049931339989602305/SPOILER_shot-1.png", ProxyURL: "https://media.discordapp.net/attachments/1049931213073821696/1049931339989602305/SPOILER_shot-1.png", Width: 1920, Height: 1080, Spoiler: true}},
		Embeds:       []Embed{{Type: "link", Title: "Omarchy", Description: "An opinionated Arch/Hyprland setup", URL: "https://omarchy.org", ImageURL: "", ThumbnailURL: "https://omarchy.org/logo.png", Color: 0x5865F2}},
		Reactions:    []Reaction{{Emoji: "👍", Count: 2, Me: true}, {Emoji: "omarchy:1000000000000000099", Count: 1, Me: false}},
		MentionsSelf: true, System: false,
	}
	sampleOwn = Message{
		ID: "1049931401540221011", ChannelID: "1049931213073821696", GuildID: str("1000000000000000001"),
		Author:  MessageAuthor{ID: "183627919046737920", Username: "m", DisplayName: "m", AvatarURL: "https://cdn.discordapp.com/avatars/183627919046737920/a.png?size=64", Bot: false},
		Content: "on my way", Timestamp: "2026-08-20T14:05:00.250Z", EditedTimestamp: nil, Nonce: "a1b2c3d4", ReplyTo: nil,
		Attachments: []Attachment{}, Embeds: []Embed{}, Reactions: []Reaction{}, MentionsSelf: false, System: false,
	}
	sampleSystem = Message{
		ID: "1049931200000000000", ChannelID: "1049931213073821696", GuildID: str("1000000000000000001"), Author: sampleAuthor,
		Content: "Ada joined the server.", Timestamp: "2026-08-19T09:00:00.000Z", EditedTimestamp: nil, Nonce: "", ReplyTo: nil,
		Attachments: []Attachment{}, Embeds: []Embed{}, Reactions: []Reaction{}, MentionsSelf: false, System: true,
	}
)

func TestGoldenEncodeDecode(t *testing.T) {
	for _, g := range goldens {
		t.Run(g.name, func(t *testing.T) {
			got, err := Encode(g.value)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join("testdata", g.name+".json")
			if *update {
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run with -update to create)", err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("encoding drift for %s\n got: %s\nwant: %s", g.name, got, want)
			}
			if bytes.Count(got, []byte("\n")) != 1 || got[len(got)-1] != '\n' {
				t.Fatalf("%s is not exactly one newline-terminated line", g.name)
			}
			if err := json.Unmarshal(want, g.rt); err != nil {
				t.Fatal(err)
			}
			back, err := Encode(reflect.ValueOf(g.rt).Elem().Interface())
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(back, want) {
				t.Fatalf("round trip drift for %s\n got: %s\nwant: %s", g.name, back, want)
			}
		})
	}
}

func TestGoldenRequests(t *testing.T) {
	cases := []struct {
		name    string
		id      int64
		command string
		check   func(t *testing.T, r *Request)
	}{
		{"request_hello", 1, "hello", nil},
		{"request_ping", 2, "ping", nil},
		{"request_get_state", 3, "get_state", nil},
		{"request_login", 4, "login", func(t *testing.T, r *Request) {
			var p LoginParams
			if e := r.Params(&p); e != nil || p.Token != "<redacted>" {
				t.Fatalf("login params: %v %+v", e, p)
			}
		}},
		{"request_logout", 5, "logout", nil},
		{"request_list_guilds", 6, "list_guilds", nil},
		{"request_list_channels", 7, "list_channels", func(t *testing.T, r *Request) {
			var p ListChannelsParams
			if e := r.Params(&p); e != nil || p.GuildID != "1000000000000000001" {
				t.Fatalf("list_channels params: %v %+v", e, p)
			}
		}},
		{"request_list_dms", 8, "list_dms", nil},
		{"request_open_channel", 20, "open_channel", func(t *testing.T, r *Request) {
			var p OpenChannelParams
			if e := r.Params(&p); e != nil || p.ChannelID != "1049931213073821696" {
				t.Fatalf("open_channel params: %v %+v", e, p)
			}
		}},
		{"request_close_channel", 21, "close_channel", func(t *testing.T, r *Request) {
			var p CloseChannelParams
			if e := r.Params(&p); e != nil || p.ChannelID != "1049931213073821696" {
				t.Fatalf("close_channel params: %v %+v", e, p)
			}
		}},
		{"request_history", 22, "history", func(t *testing.T, r *Request) {
			var p HistoryParams
			if e := r.Params(&p); e != nil || p.ChannelID != "1049931213073821696" || p.BeforeID != "1049931302442426390" || p.Limit != 50 {
				t.Fatalf("history params: %v %+v", e, p)
			}
		}},
		{"request_history_default_limit", 22, "history", func(t *testing.T, r *Request) {
			var p HistoryParams
			if e := r.Params(&p); e != nil || p.Limit != 0 {
				t.Fatalf("history params: %v %+v", e, p)
			}
		}},
		{"request_ack", 23, "ack", func(t *testing.T, r *Request) {
			var p AckParams
			if e := r.Params(&p); e != nil || p.ChannelID != "1049931213073821696" || p.MessageID != "1049931339989602304" {
				t.Fatalf("ack params: %v %+v", e, p)
			}
		}},
		{"request_send", 31, "send", func(t *testing.T, r *Request) {
			var p SendParams
			if e := r.Params(&p); e != nil || p.ChannelID != "1049931213073821696" || p.Content != "on my way" || p.ReplyTo != "1049931302442426390" || p.ReplyMention != nil {
				t.Fatalf("send params: %v %+v", e, p)
			}
		}},
		{"request_send_no_mention", 31, "send", func(t *testing.T, r *Request) {
			var p SendParams
			if e := r.Params(&p); e != nil || p.ReplyMention == nil || *p.ReplyMention {
				t.Fatalf("send params: %v %+v", e, p)
			}
		}},
		{"request_edit", 32, "edit", func(t *testing.T, r *Request) {
			var p EditParams
			if e := r.Params(&p); e != nil || p.ChannelID != "1049931213073821696" || p.MessageID != "1049931401540221011" || p.Content != "on my way!" {
				t.Fatalf("edit params: %v %+v", e, p)
			}
		}},
		{"request_delete", 33, "delete", func(t *testing.T, r *Request) {
			var p DeleteParams
			if e := r.Params(&p); e != nil || p.ChannelID != "1049931213073821696" || p.MessageID != "1049931401540221011" {
				t.Fatalf("delete params: %v %+v", e, p)
			}
		}},
		{"request_react", 34, "react", func(t *testing.T, r *Request) {
			var p ReactParams
			if e := r.Params(&p); e != nil || p.Emoji != "👍" {
				t.Fatalf("react params: %v %+v", e, p)
			}
		}},
		{"request_unreact_custom", 34, "unreact", func(t *testing.T, r *Request) {
			var p ReactParams
			if e := r.Params(&p); e != nil || p.Emoji != "omarchy:1000000000000000099" {
				t.Fatalf("unreact params: %v %+v", e, p)
			}
		}},
		{"request_typing", 35, "typing", func(t *testing.T, r *Request) {
			var p TypingParams
			if e := r.Params(&p); e != nil || p.ChannelID != "1049931213073821696" {
				t.Fatalf("typing params: %v %+v", e, p)
			}
		}},
		{"request_set_presence", 36, "set_presence", func(t *testing.T, r *Request) {
			var p SetPresenceParams
			if e := r.Params(&p); e != nil || p.Status != "dnd" {
				t.Fatalf("set_presence params: %v %+v", e, p)
			}
		}},
		{"request_fetch_media", 40, "fetch_media", func(t *testing.T, r *Request) {
			var p FetchMediaParams
			if e := r.Params(&p); e != nil || p.URL != "https://cdn.discordapp.com/avatars/183627919046737920/a.png" || p.Size != 64 {
				t.Fatalf("fetch_media params: %v %+v", e, p)
			}
		}},
		{"request_set_config", 43, "set_config", func(t *testing.T, r *Request) {
			var p SetConfigParams
			if e := r.Params(&p); e != nil || p.MediaCacheMB == nil || *p.MediaCacheMB != 256 {
				t.Fatalf("set_config params: %v %+v", e, p)
			}
		}},
		{"request_upload", 44, "upload", func(t *testing.T, r *Request) {
			var p UploadParams
			if e := r.Params(&p); e != nil || p.ChannelID != "1049931213073821696" || len(p.Paths) != 1 || p.Paths[0] != "/run/user/1000/omarchy-discord/staged/shot-1.png" || p.Content != "look at this" || p.Spoiler {
				t.Fatalf("upload params: %v %+v", e, p)
			}
		}},
		{"request_start_qr_login", 50, "start_qr_login", nil},
		{"request_cancel_qr_login", 51, "cancel_qr_login", nil},
		{"request_quick_switch", 60, "quick_switch", func(t *testing.T, r *Request) {
			var p QuickSwitchParams
			if e := r.Params(&p); e != nil || p.Query != "gen" || p.Limit != 10 {
				t.Fatalf("quick_switch params: %v %+v", e, p)
			}
		}},
		{"request_quick_switch_empty", 60, "quick_switch", func(t *testing.T, r *Request) {
			var p QuickSwitchParams
			if e := r.Params(&p); e != nil || p.Query != "" || p.Limit != 0 {
				t.Fatalf("quick_switch params: %v %+v", e, p)
			}
		}},
		{"request_list_threads", 61, "list_threads", func(t *testing.T, r *Request) {
			var p ListThreadsParams
			if e := r.Params(&p); e != nil || p.ChannelID != "1049931213073821696" {
				t.Fatalf("list_threads params: %v %+v", e, p)
			}
		}},
		{"request_subscribe_members", 62, "subscribe_members", func(t *testing.T, r *Request) {
			var p SubscribeMembersParams
			if e := r.Params(&p); e != nil || p.ChannelID != "1049931213073821696" {
				t.Fatalf("subscribe_members params: %v %+v", e, p)
			}
		}},
		{"request_unsubscribe_members", 63, "unsubscribe_members", func(t *testing.T, r *Request) {
			var p SubscribeMembersParams
			if e := r.Params(&p); e != nil || p.ChannelID != "1049931213073821696" {
				t.Fatalf("unsubscribe_members params: %v %+v", e, p)
			}
		}},
		{"request_list_emoji", 64, "list_emoji", nil},
		{"request_voice_join", 70, "voice_join", func(t *testing.T, r *Request) {
			var p VoiceJoinParams
			if e := r.Params(&p); e != nil || p.GuildID != "1000000000000000001" || p.ChannelID != "1000000000000000012" {
				t.Fatalf("voice_join params: %v %+v", e, p)
			}
		}},
		{"request_voice_leave", 71, "voice_leave", nil},
		{"request_voice_set", 72, "voice_set", func(t *testing.T, r *Request) {
			var p VoiceSetParams
			if e := r.Params(&p); e != nil || p.Muted == nil || !*p.Muted || p.Deafened != nil {
				t.Fatalf("voice_set params: %v %+v", e, p)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			line, err := os.ReadFile(filepath.Join("testdata", c.name+".json"))
			if err != nil {
				t.Fatal(err)
			}
			line = bytes.TrimRight(line, "\n")
			r, e := DecodeRequest(line)
			if e != nil {
				t.Fatalf("decode: %v", e)
			}
			if r.ID != c.id || r.Command != c.command || r.V != 1 {
				t.Fatalf("got %+v", r)
			}
			if c.check != nil {
				c.check(t, r)
			}
		})
	}
}

func TestMalformedLineIsInvalidRequestWithIDZero(t *testing.T) {
	for _, line := range []string{`{not json`, ``, `[]`, `"str"`} {
		r, e := DecodeRequest([]byte(line))
		if e == nil || e.Code != CodeInvalidRequest || r != nil {
			t.Fatalf("%q: want invalid_request without request, got %v %+v", line, e, r)
		}
		out, err := Encode(ErrResponse(0, e))
		if err != nil {
			t.Fatal(err)
		}
		var resp Response
		if err := json.Unmarshal(out, &resp); err != nil {
			t.Fatal(err)
		}
		if resp.ID != 0 || resp.OK || resp.Err == nil || resp.Err.Code != CodeInvalidRequest || resp.Type != "response" {
			t.Fatalf("%q: bad response %s", line, out)
		}
	}
}

func TestMissingCommandEchoesID(t *testing.T) {
	for _, line := range []string{`{"v":1,"id":3}`, `{"v":1,"id":3,"command":""}`} {
		r, e := DecodeRequest([]byte(line))
		if e == nil || e.Code != CodeInvalidRequest || r == nil || r.ID != 3 {
			t.Fatalf("%q: got %v %+v", line, e, r)
		}
	}
}

func TestUnsupportedVersion(t *testing.T) {
	r, e := DecodeRequest([]byte(`{"v":2,"id":9,"command":"ping"}`))
	if e == nil || e.Code != CodeUnsupportedVersion || r == nil || r.ID != 9 {
		t.Fatalf("got %v %+v", e, r)
	}
}

const fakeToken = "MTgzNjI3OTE5MDQ2NzM3OTIw.GabcDE.xyz_123456789-abcdefghijklmnop"

func TestErrorfRedacts(t *testing.T) {
	for _, msg := range []string{
		"token rejected: Authorization: Bearer " + fakeToken,
		"boom token=" + fakeToken,
		"gateway said " + fakeToken,
	} {
		out, err := Encode(ErrResponse(1, Errorf(CodeInternalError, "%s", msg)))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(out, []byte(fakeToken)) || !bytes.Contains(out, []byte(`\u003credacted\u003e`)) {
			t.Fatalf("secret leaked: %s", out)
		}
		if !json.Valid(out) {
			t.Fatalf("invalid JSON: %s", out)
		}
	}
}

func TestEncodeDoesNotRedactUserContent(t *testing.T) {
	names := []string{"Authorization Team", "token=abc", `quote " back \ slash`, "authorization: bearer x", fakeToken}
	var guilds []Guild
	var dms []Channel
	for i, n := range names {
		guilds = append(guilds, Guild{ID: "1", Name: n, Unread: UnreadRead, Position: i})
		dms = append(dms, Channel{ID: "2", Type: "dm", Name: n, Topic: n, Unread: UnreadRead, Recipients: []User{{ID: "3", Username: n, DisplayName: n}}})
	}
	out, err := Encode(NewGuildsSynced(1, guilds, dms))
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(out) {
		t.Fatalf("invalid JSON: %s", out)
	}
	var back GuildsSyncedEvent
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatal(err)
	}
	for i, n := range names {
		if back.Guilds[i].Name != n || back.DMs[i].Name != n || back.DMs[i].Topic != n || back.DMs[i].Recipients[0].Username != n {
			t.Fatalf("content altered for %q: %s", n, out)
		}
	}
}
