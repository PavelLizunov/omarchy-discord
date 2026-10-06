package session

import (
	"github.com/diamondburned/ningen/v3"

	"github.com/mattcalayo/omarchy-discord/backend/internal/panics"
)

func addSyncHandler[T any](n *ningen.State, name string, fn func(T)) {
	n.AddSyncHandler(func(ev T) {
		defer panics.Recover("session: handler " + name)
		fn(ev)
	})
}
