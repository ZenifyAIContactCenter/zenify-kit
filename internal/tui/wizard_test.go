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
