package cli

import (
	"io"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ghx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/reconcile"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/tui"
)

// runWizard drives the interactive onboarding wizard (internal/tui) over the
// engine's plan/apply callbacks. The TUI holds no reconcile logic itself —
// PlanFn rebuilds the plan the same way the headless dry-run path does, and
// ApplyFn (Task 11) will apply the user's selection.
func runWizard(w io.Writer, m *manifest.Manifest, workspace string) error {
	gh, git := ghx.ExecRunner(), gitx.ExecRunner()
	res, err := tui.RunOnboard(tui.OnboardConfig{
		Workspace:  workspace,
		Accessible: false,
		PlanFn: func() ([]reconcile.RepoPlan, error) {
			plans, _, perr := buildPlan(m, gh, git, workspace)
			return plans, perr
		},
		ApplyFn: func(sel []string) error { return runApplySelected(w, m, workspace, sel, gh, git) },
	})
	_ = res
	return err
}

// runApplySelected applies the user's selected repos from the wizard. It is
// a stub in this task — Task 11 fills in the real apply-with-progress body.
func runApplySelected(w io.Writer, m *manifest.Manifest, workspace string, selected []string, gh ghx.Runner, git gitx.Runner) error {
	return nil
}
