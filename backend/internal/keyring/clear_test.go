package keyring

import (
	"context"
	"errors"
	"testing"
)

func TestClearPropagatesLookupFailure(t *testing.T) {
	want := errors.New("lookup failed")
	k := Keyring{Run: func(_ context.Context, _ string, args ...string) (string, int, error) {
		if args[0] == "lookup" {
			return "", -1, want
		}
		return "", 0, nil
	}}
	if err := k.Clear(context.Background()); !errors.Is(err, want) {
		t.Fatalf("expected lookup failure, got %v", err)
	}
}

func TestClearReportsEntriesRemaining(t *testing.T) {
	k := Keyring{Run: func(context.Context, string, ...string) (string, int, error) {
		return "still stored", 0, nil
	}}
	if err := k.Clear(context.Background()); err == nil {
		t.Fatal("clear reported success with entries remaining")
	}
}
