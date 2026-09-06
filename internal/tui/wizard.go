// Package tui implements the interactive onboarding wizard for `zenify up`.
// It holds NO reconcile logic: discovery, scanning and applying all happen
// behind the engine callbacks in OnboardConfig (PlanFn / ApplyFn), so this
// package only orchestrates presentation — huh forms for selection, lipgloss
// for rendering the plan.
package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/reconcile"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// OnboardConfig drives one wizard run. The TUI never reconciles state
// itself — PlanFn and ApplyFn are the engine's read-only plan builder and
// mutating applier, injected by internal/cli so this package stays free of
// ghx/gitx/reconcile wiring.
type OnboardConfig struct {
	Workspace string
	// Accessible forces the accessible (screen-reader / non-raw-TTY) huh
	// mode, which also makes the wizard runnable headlessly in tests.
	Accessible bool
	// PlanOnly stops the wizard after the plan is built and rendered,
	// without prompting for selection or invoking ApplyFn. This is the
	// dry-run parity path exercised by TestRunOnboard_AccessiblePlanOnly.
	PlanOnly bool
	// AutoConfirm skips the apply confirmation prompt (Task 11).
	AutoConfirm bool
	PlanFn      func() ([]reconcile.RepoPlan, error)
	ApplyFn     func(selected []string) error
}

// OnboardResult carries the plan (and, once Task 11 wires apply, the
// selected repos) back to the caller.
type OnboardResult struct {
	Plan     []reconcile.RepoPlan
	Selected []string
}

// RunOnboard runs the discover → select → scan → plan wizard. In PlanOnly
// (or headless Accessible) mode it builds the plan via PlanFn, renders it,
// and returns without prompting or applying — later tasks add the
// interactive multiselect + apply confirmation on top of this skeleton.
func RunOnboard(cfg OnboardConfig) (OnboardResult, error) {
	var res OnboardResult

	plan, err := cfg.PlanFn()
	if err != nil {
		return res, err
	}
	res.Plan = plan

	renderPlan(os.Stdout, plan, cfg.Accessible)

	if cfg.PlanOnly {
		return res, nil
	}

	selected, err := selectRepos(plan, cfg.Accessible)
	if err != nil {
		return res, err
	}
	res.Selected = selected

	if cfg.ApplyFn == nil || len(selected) == 0 {
		return res, nil
	}
	if err := cfg.ApplyFn(selected); err != nil {
		return res, err
	}
	return res, nil
}

// selectRepos prompts the user to pick which planned repos to onboard, via a
// huh multiselect over the plan's repo names. Accessible forces the
// non-raw-TTY fallback so it can run without a real terminal.
func selectRepos(plan []reconcile.RepoPlan, accessible bool) ([]string, error) {
	if len(plan) == 0 {
		return nil, nil
	}
	opts := make([]huh.Option[string], 0, len(plan))
	for _, p := range plan {
		opts = append(opts, huh.NewOption(fmt.Sprintf("%s (%s)", p.Name, p.State), p.Name))
	}
	var selected []string
	field := huh.NewMultiSelect[string]().
		Title("Select repos to onboard").
		Options(opts...).
		Value(&selected)
	form := huh.NewForm(huh.NewGroup(field)).WithAccessible(accessible)
	if err := form.Run(); err != nil {
		return nil, err
	}
	return selected, nil
}

var (
	headerStyle = lipgloss.NewStyle().Bold(true)
	stateStyle  = lipgloss.NewStyle().Faint(true)
)

// renderPlan prints the plan as a simple table. Accessible mode (and any
// non-interactive run) uses plain text with no styling.
func renderPlan(w *os.File, plan []reconcile.RepoPlan, accessible bool) {
	if len(plan) == 0 {
		fmt.Fprintln(w, "no repos to onboard")
		return
	}
	if accessible {
		for _, p := range plan {
			fmt.Fprintf(w, "%-22s %-8s %s\n", p.Name, p.State, p.Reason)
		}
		return
	}
	var b strings.Builder
	b.WriteString(headerStyle.Render(fmt.Sprintf("%-22s %-8s %s", "REPO", "STATE", "REASON")))
	b.WriteByte('\n')
	for _, p := range plan {
		b.WriteString(fmt.Sprintf("%-22s ", p.Name))
		b.WriteString(stateStyle.Render(fmt.Sprintf("%-8s", string(p.State))))
		b.WriteString(" " + p.Reason)
		b.WriteByte('\n')
	}
	fmt.Fprint(w, b.String())
}
