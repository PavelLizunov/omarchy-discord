package panics

import (
	"runtime/debug"

	"github.com/mattcalayo/omarchy-discord/backend/internal/redact"
)

func Recover(what string) {
	if r := recover(); r != nil {
		Log(what, r)
	}
}

func Log(what string, r any) {
	redact.Logf("panic in %s: %v\n%s", what, r, debug.Stack())
}

func Go(what string, f func()) {
	go func() {
		defer Recover(what)
		f()
	}()
}
