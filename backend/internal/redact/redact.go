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
	{regexp.MustCompile(`(?i)("(?:token|ticket|encrypted_token|password)"\s*:\s*)"[^"\\]*"`), `${1}"<redacted>"`},
	{regexp.MustCompile(`(?i)\b(token|ticket|encrypted_token|password)=[^&\s"'\\]+`), `${1}=<redacted>`},
	{regexp.MustCompile(`(?i)(authorization:?\s*)(bearer\s+)?[\w.-]+`), `${1}<redacted>`},
	{regexp.MustCompile(`mfa\.[\w-]{20,}`), `<redacted>`},
	{regexp.MustCompile(`[\w-]{20,}\.[\w-]{5,}\.[\w-]{20,}`), `<redacted>`},
}

func Redact(s string) string {
	for _, r := range rules {
		s = r.re.ReplaceAllString(s, r.repl)
	}
	return s
}

var logger = log.New(os.Stderr, "", log.LstdFlags|log.Lmsgprefix)

func Logf(format string, args ...any) {
	logger.Output(2, Redact(fmt.Sprintf(format, args...)))
}
