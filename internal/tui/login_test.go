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
		{"offline-client-timeout", `Get "https://api.github.com": net/http: request canceled (Client.Timeout exceeded while awaiting headers)`, errors.New("exit status 1"), "", authUnreachable},
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

func TestDetectGit_MissingBinary(t *testing.T) {
	err := detectGitWith(func(string) (string, error) { return "", errNotFound })
	if err == nil {
		t.Fatal("expected error when git missing")
	}
}

func TestOfferInstallGH(t *testing.T) {
	// brew có mặt + user đồng ý + runner cài xong → nil (re-detect ok)
	installed := false
	cfg := OnboardConfig{
		ConfirmFn:     func(string) (bool, error) { return true, nil },
		InstallRunner: func(tool string) error { installed = true; return nil },
		DetectGHFn:    func() error { if installed { return nil }; return errNotFound },
		lookPath:      func(name string) (string, error) { return "/opt/homebrew/bin/" + name, nil },
	}
	if err := offerInstallGH(cfg); err != nil {
		t.Fatalf("expected success after install, got %v", err)
	}
	if !installed {
		t.Fatal("expected InstallRunner to be called")
	}

	// user từ chối → lỗi guide, runner KHÔNG chạy
	ran := false
	cfg2 := OnboardConfig{
		ConfirmFn:     func(string) (bool, error) { return false, nil },
		InstallRunner: func(string) error { ran = true; return nil },
		lookPath:      func(name string) (string, error) { return "/opt/homebrew/bin/" + name, nil },
	}
	if err := offerInstallGH(cfg2); err == nil {
		t.Fatal("expected guide error when user declines")
	}
	if ran {
		t.Fatal("InstallRunner must not run when declined")
	}

	// brew vắng → lỗi guide, không hỏi
	cfg3 := OnboardConfig{
		ConfirmFn:     func(string) (bool, error) { t.Fatal("must not prompt when brew absent"); return false, nil },
		InstallRunner: func(string) error { return nil },
		lookPath:      func(string) (string, error) { return "", errNotFound },
	}
	if err := offerInstallGH(cfg3); err == nil {
		t.Fatal("expected guide error when brew absent")
	}
}
