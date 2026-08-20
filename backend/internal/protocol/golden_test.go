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

// goldens lists every response/event shape the backend emits in Phase 0. Each
// value is encoded with the real encoder and compared byte-for-byte with
// testdata/<name>.json, then decoded back into a fresh value of the same type
// and compared with the original.
// typedResponse mirrors Response with a concrete result type so a fixture can
// be decoded back into static types.
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
	rt    any // pointer to the static type used for the decode leg
}{
	{"response_hello", OKResponse(1, Hello()), &typedResponse[HelloResult]{}},
	{"response_ping", OKResponse(2, PingResult{Pong: true}), &typedResponse[PingResult]{}},
	{"response_get_state", OKResponse(3, State{
		ProtocolVersion: 1, BackendVersion: BackendVersion, Lifecycle: LifecycleReady,
		User:     &User{ID: "183627919046737920", Username: "m", DisplayName: "m", AvatarURL: "https://cdn.discordapp.com/avatars/183627919046737920/a.png"},
		Presence: "online", TotalMentionCount: 3, UnreadDMChannelID: str("1049931213073821696"), Generation: 7, Error: "",
	}), &typedResponse[State]{}},
	{"response_get_state_logged_out", OKResponse(3, State{
		ProtocolVersion: 1, BackendVersion: BackendVersion, Lifecycle: LifecycleLoggedOut,
		User: nil, Presence: "", TotalMentionCount: 0, UnreadDMChannelID: nil, Generation: 1, Error: "",
	}), &typedResponse[State]{}},
	{"response_login", OKResponse(4, LoginResult{User: User{ID: "183627919046737920", Username: "m", DisplayName: "m", AvatarURL: "https://cdn.discordapp.com/avatars/183627919046737920/a.png"}}), &typedResponse[LoginResult]{}},
	{"response_logout", OKResponse(5, EmptyResult{}), &typedResponse[EmptyResult]{}},
	{"response_list_guilds", OKResponse(6, ListGuildsResult{Guilds: []Guild{
		{ID: "1000000000000000001", Name: "Omarchy", IconURL: str("https://cdn.discordapp.com/icons/1000000000000000001/abc.png"), Unread: UnreadMentioned, MentionCount: 2, Position: 0},
		{ID: "1000000000000000002", Name: "Quiet", IconURL: nil, Unread: UnreadRead, MentionCount: 0, Position: 1},
	}}), &typedResponse[ListGuildsResult]{}},
	{"response_list_channels", OKResponse(7, ListChannelsResult{Channels: []Channel{
		{ID: "1000000000000000010", GuildID: str("1000000000000000001"), Type: "category", Name: "General", Topic: "", ParentID: nil, Position: 0, LastMessageID: nil, Unread: UnreadRead, MentionCount: 0, Muted: false},
		{ID: "1000000000000000011", GuildID: str("1000000000000000001"), Type: "text", Name: "general", Topic: "chat", ParentID: str("1000000000000000010"), Position: 0, LastMessageID: str("1000000000000000099"), Unread: UnreadMentioned, MentionCount: 2, Muted: false},
	}}), &typedResponse[ListChannelsResult]{}},
	{"response_list_dms", OKResponse(8, ListChannelsResult{Channels: []Channel{
		{ID: "1049931213073821696", GuildID: nil, Type: "dm", Name: "ada", Topic: "", ParentID: nil, Position: 0, LastMessageID: str("1049931302442426390"), Unread: UnreadUnread, MentionCount: 0, Muted: false,
			Recipients: []User{{ID: "2000000000000000001", Username: "ada", DisplayName: "ada", AvatarURL: "https://cdn.discordapp.com/avatars/2000000000000000001/b.png"}}},
	}}), &typedResponse[ListChannelsResult]{}},
	{"response_error", ErrResponse(9, &Error{Code: CodeUnknownChannel, Message: "channel is not accessible"}), &typedResponse[struct{}]{}},
	{"response_invalid_request", ErrResponse(0, &Error{Code: CodeInvalidRequest, Message: "malformed request"}), &typedResponse[struct{}]{}},
	{"event_state_changed", NewStateChanged(State{
		ProtocolVersion: 1, BackendVersion: BackendVersion, Lifecycle: LifecycleConnecting,
		User: nil, Presence: "", TotalMentionCount: 0, UnreadDMChannelID: nil, Generation: 2, Error: "",
	}), &StateChangedEvent{}},
	{"event_guilds_synced", NewGuildsSynced(
		[]Guild{{ID: "1000000000000000001", Name: "Omarchy", IconURL: nil, Unread: UnreadUnread, MentionCount: 0, Position: 0}},
		[]Channel{{ID: "1049931213073821696", GuildID: nil, Type: "dm", Name: "ada", Topic: "", ParentID: nil, Position: 0, LastMessageID: str("1049931302442426390"), Unread: UnreadRead, MentionCount: 0, Muted: false,
			Recipients: []User{{ID: "2000000000000000001", Username: "ada", DisplayName: "ada", AvatarURL: ""}}}},
	), &GuildsSyncedEvent{}},
	{"event_guilds_synced_empty", NewGuildsSynced(nil, nil), &GuildsSyncedEvent{}},
}

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
			// Decode back into the same static type and compare.
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

// requestGoldens are the request lines the QML client sends; each fixture is
// decoded with the real decoder and its params checked.
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
	for _, line := range []string{`{not json`, ``, `[]`, `"str"`, `{"v":1,"id":3}`} {
		r, e := DecodeRequest([]byte(line))
		if e == nil || e.Code != CodeInvalidRequest {
			t.Fatalf("%q: want invalid_request, got %v", line, e)
		}
		var id int64
		if r != nil && line == `{"v":1,"id":3}` {
			// missing command: id is readable but the request is still malformed
			id = 0
		}
		out, err := Encode(ErrResponse(id, e))
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

func TestUnsupportedVersion(t *testing.T) {
	r, e := DecodeRequest([]byte(`{"v":2,"id":9,"command":"ping"}`))
	if e == nil || e.Code != CodeUnsupportedVersion || r == nil || r.ID != 9 {
		t.Fatalf("got %v %+v", e, r)
	}
}

func TestEncodeRedactsAtChokePoint(t *testing.T) {
	out, err := Encode(ErrResponse(1, &Error{Code: CodeInternalError, Message: `boom token=abc.def.ghi`}))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("abc.def.ghi")) {
		t.Fatalf("secret leaked: %s", out)
	}
}
