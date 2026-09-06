package tui

import (
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
		AuthStatusFn: func() (string, bool) { return "test", true },
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
		AuthStatusFn: func() (string, bool) { return "test", true },
	}
	if _, err := RunOnboard(cfg); err != nil {
		t.Fatalf("RunOnboard: %v", err)
	}
	if !applied {
		t.Fatal("ApplyFn not called despite AutoConfirm")
	}
}
