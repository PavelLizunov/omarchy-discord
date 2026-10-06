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

var (
	Attributes = []string{"service", "quickshell-discord", "kind", "user-token"}
	Label      = "Omarchy Discord user token"
)

const maxClear = 20

const timeout = 15 * time.Second

var ErrNotFound = errors.New("keyring: no token stored")

var ErrUnavailable = errors.New("keyring: secret-tool not found")

type Runner func(ctx context.Context, stdin string, args ...string) (stdout string, exitCode int, err error)

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
	cmd.Stderr = &errb
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

func Available() bool {
	_, err := exec.LookPath("secret-tool")
	return err == nil
}

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

func (k Keyring) Clear(ctx context.Context) error {
	for i := 0; i < maxClear; i++ {
		_, code, err := k.run(ctx, "", append([]string{"clear"}, Attributes...)...)
		if err != nil {
			return err
		}
		if code != 0 {
			return nil
		}
		if _, err := k.Lookup(ctx); errors.Is(err, ErrNotFound) {
			return nil
		} else if err != nil {
			return err
		}
	}
	return errors.New("keyring: entries remain after clear limit")
}
