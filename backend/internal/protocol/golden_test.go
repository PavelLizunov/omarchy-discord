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
	{"response_login", OKResponse(4, LoginResult{User: User{ID: "183627919046737920", Username: "m", DisplayName: "m", AvatarURL: "https://cdn.discordapp.com/avatars/183627919046737920/a.png"}, KeyringStored: true}), &typedResponse[LoginResult]{}},
	{"response_logout", OKResponse(5, EmptyResult{}), &typedResponse[EmptyResult]{}},
	{"response_list_guilds", OKResponse(6, ListGuildsResult{Guilds: []Guild{
		{ID: "1000000000000000001", Name: "Omarchy", IconURL: str("https://cdn.discordapp.com/icons/1000000000000000001/abc.png"), Unread: UnreadMentioned, MentionCount: 2, Position: 0},
		{ID: "1000000000000000002", Name: "Quiet", IconURL: nil, Unread: UnreadRead, MentionCount: 0, Position: 1},
	}}), &typedResponse[ListGuildsResult]{}},
	{"response_list_channels", OKResponse(7, ListChannelsResult{Channels: []Channel{
		{ID: "1000000000000000010", GuildID: str("1000000000000000001"), Type: "category", Name: "General", Topic: "", ParentID: nil, Position: 0, LastMessageID: nil, Unread: UnreadRead, MentionCount: 0, Muted: false, Recipients: []User{}},
		{ID: "1000000000000000011", GuildID: str("1000000000000000001"), Type: "text", Name: "general", Topic: "chat", ParentID: str("1000000000000000010"), Position: 0, LastMessageID: str("1000000000000000099"), Unread: UnreadMentioned, MentionCount: 2, Muted: false, Recipients: []User{}},
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
	{"event_guilds_synced", NewGuildsSynced(7,
		[]Guild{{ID: "1000000000000000001", Name: "Omarchy", IconURL: nil, Unread: UnreadUnread, MentionCount: 0, Position: 0}},
		[]Channel{{ID: "1049931213073821696", GuildID: nil, Type: "dm", Name: "ada", Topic: "", ParentID: nil, Position: 0, LastMessageID: str("1049931302442426390"), Unread: UnreadRead, MentionCount: 0, Muted: false,
			Recipients: []User{{ID: "2000000000000000001", Username: "ada", DisplayName: "ada", AvatarURL: ""}}}},
	), &GuildsSyncedEvent{}},
	{"event_guilds_synced_empty", NewGuildsSynced(3, nil, nil), &GuildsSyncedEvent{}},
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

// A parseable line with a missing command is invalid_request but keeps its id
// so the client can fail the pending request.
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

// Error messages are redacted where they are built (Errorf), so a token in a
// wrapped Discord error never reaches the wire.
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
		// json.Marshal HTML-escapes the angle brackets of the marker.
		if bytes.Contains(out, []byte(fakeToken)) || !bytes.Contains(out, []byte(`\u003credacted\u003e`)) {
			t.Fatalf("secret leaked: %s", out)
		}
		if !json.Valid(out) {
			t.Fatalf("invalid JSON: %s", out)
		}
	}
}

// User-controlled text (guild names, topics) must never be altered by
// redaction: the encoded line stays valid JSON and round-trips intact.
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
