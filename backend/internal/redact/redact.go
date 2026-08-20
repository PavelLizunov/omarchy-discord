// Package redact scrubs secrets from strings before they cross a boundary
// (journal, socket). Every log line and every socket write goes through
// Redact; see docs/CONVENTIONS.md §7.
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
	{regexp.MustCompile(`(?i)("(?:token|ticket|encrypted_token|password)"\s*:\s*)"[^"]*"`), `${1}"<redacted>"`},
	// Query/form params: token=..., ticket=...
	{regexp.MustCompile(`(?i)\b(token|ticket|encrypted_token|password)=[^&\s"']+`), `${1}=<redacted>`},
	// Authorization headers, bearer or raw.
	{regexp.MustCompile(`(?i)(authorization:?\s*)(bearer\s+)?\S+`), `${1}<redacted>`},
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
