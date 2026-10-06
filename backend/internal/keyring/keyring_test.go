package keyring

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type call struct {
	stdin string
	args  []string
}

func TestStoreUsesStdinNotArgv(t *testing.T) {
	var calls []call
	k := Keyring{Run: func(_ context.Context, stdin string, args ...string) (string, int, error) {
		calls = append(calls, call{stdin, args})
		return "", 0, nil
	}}
	if err := k.Store(context.Background(), "sekrit"); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].stdin != "sekrit" {
		t.Fatalf("calls=%+v", calls)
	}
	if strings.Contains(strings.Join(calls[0].args, " "), "sekrit") {
		t.Fatalf("token leaked into argv: %v", calls[0].args)
	}
	want := "store --label=Omarchy Discord user token service quickshell-discord kind user-token"
	if got := strings.Join(calls[0].args, " "); got != want {
		t.Fatalf("args=%q", got)
	}
}

func TestLookup(t *testing.T) {
	k := Keyring{Run: func(_ context.Context, _ string, args ...string) (string, int, error) {
		if args[0] != "lookup" {
			t.Fatalf("args=%v", args)
		}
		return "tok\n", 0, nil
	}}
	tok, err := k.Lookup(context.Background())
	if err != nil || tok != "tok" {
		t.Fatalf("%q %v", tok, err)
	}
	k = Keyring{Run: func(context.Context, string, ...string) (string, int, error) { return "", 1, nil }}
	if _, err := k.Lookup(context.Background()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestClearLoopsAndStops(t *testing.T) {
	stored := 3
	clears := 0
	k := Keyring{Run: func(_ context.Context, _ string, args ...string) (string, int, error) {
		switch args[0] {
		case "clear":
			clears++
			if stored > 0 {
				stored--
			}
			return "", 0, nil
		case "lookup":
			if stored > 0 {
				return "tok", 0, nil
			}
			return "", 1, nil
		}
		t.Fatalf("unexpected %v", args)
		return "", 1, nil
	}}
	if err := k.Clear(context.Background()); err != nil {
		t.Fatal(err)
	}
	if clears != 3 {
		t.Fatalf("clears=%d", clears)
	}
	k = Keyring{Run: func(_ context.Context, _ string, args ...string) (string, int, error) { return "tok", 0, nil }}
	clears = 0
	if err := k.Clear(context.Background()); err == nil {
		t.Fatal("expected clear limit error")
	}
}
