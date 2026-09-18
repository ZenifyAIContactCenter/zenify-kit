package cli

import (
	"io"
	"os"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ghx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/reconcile"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/tui"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ui"
)

// runWizard drives the interactive onboarding wizard (internal/tui) over the
// engine's plan/apply callbacks. The TUI holds no reconcile logic itself —
// PlanFn rebuilds the plan the same way the headless dry-run path does, and
// ApplyFn (Task 11) will apply the user's selection. sources is the Where
// step's answer (nil when it did not run — e.g. the marker/pointer path).
func runWizard(w io.Writer, m *manifest.Manifest, workspace string, sources map[string]reconcile.Source) error {
	gh, git := ghx.ExecRunner(), gitx.ExecRunner()
	res, err := tui.RunOnboard(tui.OnboardConfig{
		Workspace:  workspace,
		Accessible: false,
		PlanFooter: planFooterRows(workspace),
		SecretKeys: []string{"MONGO_URL", "E2E_DOMAIN", "E2E_EMAIL", "E2E_PASSWORD"},
		PlanFn: func() ([]reconcile.RepoPlan, error) {
			// The wizard rebuilds the plan after login (repo discovery + per-repo
			// scan takes a few seconds); animate a spinner so the wait is not
			// silent. Stop() clears the line — the plan table renders right after,
			// and the outer buildPlan already printed the success line.
			sp := ui.NewSpinner(os.Stderr, "Đang dựng kế hoạch onboarding") //znf:allow-lang
			sp.Start()
			plans, _, perr := buildPlan(m, gh, git, workspace, sources)
			sp.Stop()
			return plans, perr
		},
		ApplyFn: func(sel []string) error { return runApplySelected(w, m, workspace, sel, gh, git, sources) },
	})
	_ = res
	return err
}

// runApplySelected applies the user's selected repos from the wizard: it
// rebuilds the plan the same way the dry-run path does, filters it down to
// the repos the user picked, then hands the filtered plan to the same
// runApply core the headless `--apply` path uses (lock, snapshot, apply,
// hook wiring, docs store — no logic duplicated here). An empty selection
// applies the full (unfiltered) plan rather than silently no-op'ing.
func runApplySelected(w io.Writer, m *manifest.Manifest, workspace string, selected []string, gh ghx.Runner, git gitx.Runner, sources map[string]reconcile.Source) error {
	plans, _, err := buildPlan(m, gh, git, workspace, sources)
	if err != nil {
		return err
	}
	// An empty selection applies the full plan. Unreachable from the TUI today
	// (RunOnboard returns before ApplyFn when nothing is picked), but kept as the
	// documented headless-parity contract: a non-TUI caller passing nil applies all.
	filtered := plans
	if len(selected) > 0 {
		want := make(map[string]bool, len(selected))
		for _, s := range selected {
			want[s] = true
		}
		filtered = make([]reconcile.RepoPlan, 0, len(plans))
		for _, p := range plans {
			if want[p.Name] {
				filtered = append(filtered, p)
			}
		}
	}
	return runApply(w, w, filtered, m, workspace, gh, git)
}
