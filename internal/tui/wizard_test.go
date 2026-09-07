package tui

import (
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/reconcile"
)

func TestRunOnboard_AccessiblePlanOnly(t *testing.T) {
	called := false
	cfg := OnboardConfig{
		Workspace:  t.TempDir(),
		Accessible: true,
		PlanOnly:   true, // stop after plan render (dry-run parity)
		PlanFn: func() ([]reconcile.RepoPlan, error) {
			called = true
			return []reconcile.RepoPlan{{Name: "contact-center-be", State: "CLONE", Reason: "not on disk"}}, nil
		},
		DetectGHFn:   func() error { return nil },
		AuthStatusFn: func() (string, authState) { return "test", authLoggedIn },
	}
	res, err := RunOnboard(cfg)
	if err != nil {
		t.Fatalf("RunOnboard: %v", err)
	}
	if !called {
		t.Fatal("PlanFn not invoked")
	}
	if len(res.Plan) != 1 || res.Plan[0].Name != "contact-center-be" {
		t.Fatalf("plan not surfaced: %+v", res.Plan)
	}
}

func TestRunOnboard_ApplyInvokesApplyFn(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // isolate printDone's read of ~/.claude/settings.json
	applied := false
	cfg := OnboardConfig{
		Workspace:   t.TempDir(),
		Accessible:  true,
		AutoConfirm: true, // parity for -y / headless
		PlanFn: func() ([]reconcile.RepoPlan, error) {
			return []reconcile.RepoPlan{{Name: "notification", State: "OK"}}, nil
		},
		ApplyFn:      func(sel []string) error { applied = true; return nil },
		DetectGHFn:   func() error { return nil },
		AuthStatusFn: func() (string, authState) { return "test", authLoggedIn },
	}
	if _, err := RunOnboard(cfg); err != nil {
		t.Fatalf("RunOnboard: %v", err)
	}
	if !applied {
		t.Fatal("ApplyFn not called despite AutoConfirm")
	}
}

// loginStep-level wiring: when gh is missing in interactive mode, loginStep
// must actually reach offerInstallGH (not just leave it unit-tested but
// unwired). Guards against a future reorder of the Accessible check.
func TestLoginStep_OffersInstallWhenGHMissing(t *testing.T) {
	installed := false
	cfg := OnboardConfig{
		Accessible:    false,
		DetectGitFn:   func() error { return nil },
		DetectGHFn:    func() error { if installed { return nil }; return errNotFound },
		ConfirmFn:     func(string) (bool, error) { return true, nil },
		InstallRunner: func(string) error { installed = true; return nil },
		lookPath:      func(name string) (string, error) { return "/opt/homebrew/bin/" + name, nil },
		AuthStatusFn:  func() (string, authState) { return "namph", authLoggedIn },
	}
	if err := loginStep(cfg); err != nil {
		t.Fatalf("loginStep should succeed after install, got %v", err)
	}
	if !installed {
		t.Fatal("expected offerInstallGH to run install from loginStep")
	}
}

// Headless (Accessible) with gh missing must return the guide error WITHOUT
// prompting — the ConfirmFn t.Fatal fires if loginStep ever prompts here.
func TestLoginStep_HeadlessGHMissingNoPrompt(t *testing.T) {
	cfg := OnboardConfig{
		Accessible:  true,
		DetectGitFn: func() error { return nil },
		DetectGHFn:  func() error { return errNotFound },
		ConfirmFn:   func(string) (bool, error) { t.Fatal("must not prompt in headless"); return false, nil },
	}
	if err := loginStep(cfg); err == nil {
		t.Fatal("expected guide error in headless when gh missing")
	}
}

func TestWelcomeNote_SkippedWhenAccessible(t *testing.T) {
	if err := welcomeNote(true); err != nil {
		t.Fatalf("welcomeNote(accessible) must be a no-op, got %v", err)
	}
}

// FR-7.1 no-browser guarantee at loginStep level: an unreachable auth status
// must return the network error and NEVER reach loginCmd() (which shells real
// gh). Guards against a future reorder of the switch vs the browser block.
func TestLoginStep_UnreachableSkipsBrowser(t *testing.T) {
	cfg := OnboardConfig{
		Accessible:   false,
		DetectGitFn:  func() error { return nil },
		DetectGHFn:   func() error { return nil },
		AuthStatusFn: func() (string, authState) { return "", authUnreachable },
	}
	err := loginStep(cfg)
	if err == nil {
		t.Fatal("expected unreachable error, got nil (would have opened browser)")
	}
	if !strings.Contains(err.Error(), "không kết nối") {
		t.Fatalf("expected network error message, got %v", err)
	}
}

// SC-13 at loginStep level: git missing is guide-only — must return the error
// without prompting or auto-installing.
func TestLoginStep_GitMissingNoPromptNoInstall(t *testing.T) {
	cfg := OnboardConfig{
		Accessible:    false,
		DetectGitFn:   func() error { return errNotFound },
		ConfirmFn:     func(string) (bool, error) { t.Fatal("must not prompt when git missing"); return false, nil },
		InstallRunner: func(string) error { t.Fatal("must not install when git missing"); return nil },
	}
	if err := loginStep(cfg); err == nil {
		t.Fatal("expected git guide error")
	}
}
