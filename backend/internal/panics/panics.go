// Package panics turns a panic inside one unit of work — a socket request, a
// gateway event handler, a background goroutine — into a logged incident
// instead of a dead daemon. Library code we do not control panics on inputs we
// do not control (ningen's member-list arithmetic, discordmd on hostile
// markdown), and a single such payload must never take the session down.
package panics

import (
	"runtime/debug"

	"github.com/mattcalayo/omarchy-discord/backend/internal/redact"
)

// Recover is deferred at the top of a unit of work: it swallows a panic and
// logs it (redacted, with the stack) under what. It must be deferred directly
// (`defer panics.Recover(...)`) — recover() only works one frame deep.
func Recover(what string) {
	if r := recover(); r != nil {
		Log(what, r)
	}
}

// Log reports a recovered panic value. Callers that need to know a panic
// happened recover themselves (recover() cannot be delegated) and hand the
// value here.
func Log(what string, r any) {
	redact.Logf("panic in %s: %v\n%s", what, r, debug.Stack())
}

// Go runs f on a new goroutine under Recover.
func Go(what string, f func()) {
	go func() {
		defer Recover(what)
		f()
	}()
}
