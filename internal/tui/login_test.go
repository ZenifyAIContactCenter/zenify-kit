package tui

import "testing"

func TestGHAuthStatus_Parse(t *testing.T) {
	// runner injected so no real gh call.
	account, ok := parseAuthStatus("github.com\n  ✓ Logged in to github.com account namph (keyring)\n")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if account != "namph" {
		t.Fatalf("account = %q, want namph", account)
	}
	_, ok2 := parseAuthStatus("You are not logged into any GitHub hosts.")
	if ok2 {
		t.Fatal("expected ok=false for logged-out")
	}
}

func TestDetectGH_MissingBinary(t *testing.T) {
	// lookPath injected to simulate missing gh
	err := detectGHWith(func(string) (string, error) { return "", errNotFound })
	if err == nil {
		t.Fatal("expected error when gh missing")
	}
}
