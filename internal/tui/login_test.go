package tui

import (
	"errors"
	"testing"
)

func TestGHAuthStatus_Parse(t *testing.T) {
	account, st := parseAuthState("github.com\n  ✓ Logged in to github.com account namph (keyring)\n", nil)
	if st != authLoggedIn {
		t.Fatal("expected authLoggedIn")
	}
	if account != "namph" {
		t.Fatalf("account = %q, want namph", account)
	}
	_, st2 := parseAuthState("You are not logged into any GitHub hosts.", errors.New("exit status 1"))
	if st2 != authLoggedOut {
		t.Fatal("expected authLoggedOut for logged-out")
	}
}

func TestParseAuthState(t *testing.T) {
	cases := []struct {
		name    string
		out     string
		err     error
		wantAcc string
		want    authState
	}{
		{"logged-in", "github.com\n  ✓ Logged in to github.com account namph (keyring)\n", nil, "namph", authLoggedIn},
		{"logged-out", "You are not logged into any GitHub hosts.", errors.New("exit status 1"), "", authLoggedOut},
		{"offline-dial", "error connecting to github.com\ndial tcp: lookup github.com: no such host", errors.New("exit status 1"), "", authUnreachable},
		{"offline-refused", "could not connect to github.com: connection refused", errors.New("exit status 1"), "", authUnreachable},
		{"offline-timeout", "dial tcp 140.82.121.3:443: i/o timeout", errors.New("exit status 1"), "", authUnreachable},
		{"auth-timeout-not-network", "SAML authorization timed out; you are not logged into github.com", errors.New("exit status 1"), "", authLoggedOut},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			acc, st := parseAuthState(c.out, c.err)
			if st != c.want {
				t.Fatalf("state = %d, want %d", st, c.want)
			}
			if acc != c.wantAcc {
				t.Fatalf("account = %q, want %q", acc, c.wantAcc)
			}
		})
	}
}

func TestDetectGH_MissingBinary(t *testing.T) {
	// lookPath injected to simulate missing gh
	err := detectGHWith(func(string) (string, error) { return "", errNotFound })
	if err == nil {
		t.Fatal("expected error when gh missing")
	}
}
