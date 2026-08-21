package session

import (
	"github.com/diamondburned/ningen/v3"

	"github.com/mattcalayo/omarchy-discord/backend/internal/panics"
)

// addSyncHandler registers a ningen sync handler whose body cannot take the
// daemon down. Gateway payloads are attacker-influenced input processed by
// library code that panics on shapes it does not expect (ningen's member-list
// arithmetic, discordmd on hostile markdown), so a panic here degrades one
// event instead of killing the session. Handlers stay sync — ningen's caches
// are consistent inside them — and the recover only wraps the body.
func addSyncHandler[T any](n *ningen.State, name string, fn func(T)) {
	n.AddSyncHandler(func(ev T) {
		defer panics.Recover("session: handler " + name)
		fn(ev)
	})
}
