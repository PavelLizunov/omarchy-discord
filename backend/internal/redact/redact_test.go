package redact

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	const tok = "MTgzNjI3OTE5MDQ2NzM3OTIw.GabcDE.xyz_123456789-abcdefghijklmnop"
	cases := []struct{ in, mustNotContain string }{
		{`{"v":1,"id":4,"command":"login","token":"` + tok + `"}`, tok},
		{`{"token": "abc", "x": 1}`, `"abc"`},
		{`{"ticket":"t1"}`, `"t1"`},
		{`{"encrypted_token":"enc"}`, `"enc"`},
		{`GET /x?token=secretvalue&y=1`, `secretvalue`},
		{`Authorization: Bearer sekrit`, `sekrit`},
		{`authorization: ` + tok, tok},
		{`mfa.abcdefghijklmnopqrstuvwxyz0123456789`, `abcdefghijklmnop`},
		{`gateway error: ` + tok + ` rejected`, tok},
	}
	for _, c := range cases {
		got := Redact(c.in)
		if strings.Contains(got, c.mustNotContain) {
			t.Errorf("Redact(%q) = %q; still contains %q", c.in, got, c.mustNotContain)
		}
		if !strings.Contains(got, "<redacted>") {
			t.Errorf("Redact(%q) = %q; no redaction marker", c.in, got)
		}
	}
	plain := `{"type":"event","v":1,"event":"state_changed","state":{"lifecycle":"ready"}}`
	if Redact(plain) != plain {
		t.Errorf("plain line altered: %q", Redact(plain))
	}
}

// The rules are JSON-safe: even when (mis)applied to a serialized line they
// never consume quotes or escape backslashes, so the result stays valid JSON.
func TestRedactKeepsJSONValid(t *testing.T) {
	lines := []string{
		`{"name":"Authorization Team","topic":"x"}`,
		`{"name":"Authorization: Bearer abc","topic":"x"}`,
		`{"topic":"token=abc\"","x":1}`,
		`{"topic":"password=a\\b","x":1}`,
		`{"token":"a\"b","x":1}`,
	}
	for _, l := range lines {
		got := Redact(l)
		if !json.Valid([]byte(got)) {
			t.Errorf("Redact(%q) = %q is not valid JSON", l, got)
		}
	}
	if got := Redact(`{"name":"Authorization Team","topic":"x"}`); !strings.Contains(got, `"topic":"x"`) {
		t.Errorf("authorization rule ran past the field: %q", got)
	}
}
