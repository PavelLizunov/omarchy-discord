// Package redact scrubs secrets from free-text strings before they cross a
// boundary (journal, socket). It is applied at the text sources — log lines,
// protocol error messages, the state error field — never to serialized JSON
// wire lines, where a rule could match across field boundaries and corrupt the
// framing. The rules are nevertheless JSON-safe (they never consume quotes or
// backslashes). See docs/CONVENTIONS.md §7.
package redact

import (
	"fmt"
	"log"
	"os"
	"regexp"
)

var rules = []struct {
	re   *regexp.Regexp
	repl string
}{
	// JSON fields: "token": "...", "ticket": "...", "encrypted_token": "..."
	{regexp.MustCompile(`(?i)("(?:token|ticket|encrypted_token|password)"\s*:\s*)"[^"\\]*"`), `${1}"<redacted>"`},
	// Query/form params: token=..., ticket=...
	{regexp.MustCompile(`(?i)\b(token|ticket|encrypted_token|password)=[^&\s"'\\]+`), `${1}=<redacted>`},
	// Authorization headers, bearer or raw. The value class is deliberately
	// narrow so the rule can never run past a token into surrounding text.
	{regexp.MustCompile(`(?i)(authorization:?\s*)(bearer\s+)?[\w.-]+`), `${1}<redacted>`},
	// MFA-style user tokens.
	{regexp.MustCompile(`mfa\.[\w-]{20,}`), `<redacted>`},
	// Bare Discord token shape: base64(user id).base64(timestamp).hmac
	{regexp.MustCompile(`[\w-]{20,}\.[\w-]{5,}\.[\w-]{20,}`), `<redacted>`},
}

// Redact returns s with anything token-shaped replaced by "<redacted>".
func Redact(s string) string {
	for _, r := range rules {
		s = r.re.ReplaceAllString(s, r.repl)
	}
	return s
}

var logger = log.New(os.Stderr, "", log.LstdFlags|log.Lmsgprefix)

// Logf writes a redacted line to stderr (the journal).
func Logf(format string, args ...any) {
	logger.Output(2, Redact(fmt.Sprintf(format, args...)))
}
