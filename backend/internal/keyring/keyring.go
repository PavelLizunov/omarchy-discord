// Package keyring stores the user token in the GNOME keyring via secret-tool.
// The token travels over stdin only, never argv (docs/CONVENTIONS.md §4).
package keyring

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Attributes identify the keyring entry; secret-tool is invoked with the token on stdin.
var (
	Attributes = []string{"service", "quickshell-discord", "kind", "user-token"}
	Label      = "Omarchy Discord user token"
)

// maxClear bounds the clear loop: secret-tool clear removes one entry per
// invocation.
const maxClear = 20

const timeout = 15 * time.Second

// ErrNotFound is returned by Lookup when no token is stored.
var ErrNotFound = errors.New("keyring: no token stored")

// ErrUnavailable is returned when secret-tool is not installed.
var ErrUnavailable = errors.New("keyring: secret-tool not found")

// Runner executes secret-tool; swapped in tests.
type Runner func(ctx context.Context, stdin string, args ...string) (stdout string, exitCode int, err error)

// Keyring wraps secret-tool. The zero value uses the real binary.
type Keyring struct {
	Run Runner
}

func (k Keyring) run(ctx context.Context, stdin string, args ...string) (string, int, error) {
	if k.Run != nil {
		return k.Run(ctx, stdin, args...)
	}
	return execSecretTool(ctx, stdin, args...)
}

func execSecretTool(ctx context.Context, stdin string, args ...string) (string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "secret-tool", args...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb // never forwarded: stderr could echo input on some failures
	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return out.String(), 0, nil
	case errors.As(err, &exitErr):
		return out.String(), exitErr.ExitCode(), nil
	case errors.Is(err, exec.ErrNotFound):
		return "", -1, ErrUnavailable
	default:
		return "", -1, fmt.Errorf("keyring: secret-tool: %w", err)
	}
}

// Available reports whether secret-tool is on PATH.
func Available() bool {
	_, err := exec.LookPath("secret-tool")
	return err == nil
}

// Lookup returns the stored token or ErrNotFound.
func (k Keyring) Lookup(ctx context.Context) (string, error) {
	out, code, err := k.run(ctx, "", append([]string{"lookup"}, Attributes...)...)
	if err != nil {
		return "", err
	}
	tok := strings.TrimRight(out, "\r\n")
	if code != 0 || tok == "" {
		return "", ErrNotFound
	}
	return tok, nil
}

// Store writes the token via stdin.
func (k Keyring) Store(ctx context.Context, token string) error {
	if token == "" {
		return errors.New("keyring: refusing to store empty token")
	}
	args := append([]string{"store", "--label=" + Label}, Attributes...)
	_, code, err := k.run(ctx, token, args...)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("keyring: secret-tool store exited %d", code)
	}
	return nil
}

// Clear removes every matching entry (looped, capped at maxClear). It is not
// an error when nothing was stored.
func (k Keyring) Clear(ctx context.Context) error {
	for i := 0; i < maxClear; i++ {
		_, code, err := k.run(ctx, "", append([]string{"clear"}, Attributes...)...)
		if err != nil {
			return err
		}
		if code != 0 {
			// Nothing (more) to clear.
			return nil
		}
		// secret-tool may report success even when nothing matched; stop once
		// a lookup confirms the entry is gone.
		if _, err := k.Lookup(ctx); errors.Is(err, ErrNotFound) {
			return nil
		}
	}
	return nil
}
