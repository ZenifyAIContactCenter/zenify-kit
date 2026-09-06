// Package tui implements the interactive onboarding wizard for `zenify up`.
// It holds NO reconcile logic: discovery, scanning and applying all happen
// behind the engine callbacks in OnboardConfig (PlanFn / ApplyFn), so this
// package only orchestrates presentation — huh forms for selection, lipgloss
// for rendering the plan.
package tui

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/reconcile"
	"github.com/charmbracelet/bubbles/progress"
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
	// DetectGHFn and AuthStatusFn override the real gh preflight/identity
	// checks (detectGH / ghAuthStatus) for tests, so RunOnboard never shells
	// out to a real `gh` in a test run. Nil uses the real implementation,
	// which is what TestRunOnboard_AccessiblePlanOnly relies on (this
	// workstation has `gh` installed and is logged in).
	DetectGHFn   func() error
	AuthStatusFn func() (string, bool)
}

// OnboardResult carries the plan, the selected repos, and whether apply ran
// to completion back to the caller.
type OnboardResult struct {
	Plan     []reconcile.RepoPlan
	Selected []string
	Done     bool
}

// RunOnboard runs the discover → select → scan → plan wizard. In PlanOnly
// (or headless Accessible) mode it builds the plan via PlanFn, renders it,
// and returns without prompting or applying — later tasks add the
// interactive multiselect + apply confirmation on top of this skeleton.
func RunOnboard(cfg OnboardConfig) (OnboardResult, error) {
	var res OnboardResult

	if err := loginStep(cfg); err != nil {
		return res, err
	}

	plan, err := cfg.PlanFn()
	if err != nil {
		return res, err
	}
	res.Plan = plan

	renderPlan(os.Stdout, plan, cfg.Accessible)

	if cfg.PlanOnly {
		return res, nil
	}

	var selected []string
	if cfg.AutoConfirm {
		// Headless / -y parity: apply the full plan, never block on an
		// interactive multiselect that has no terminal to read from.
		for _, p := range plan {
			selected = append(selected, p.Name)
		}
	} else {
		selected, err = selectRepos(plan, cfg.Accessible)
		if err != nil {
			return res, err
		}
	}
	res.Selected = selected

	if cfg.ApplyFn == nil || len(selected) == 0 {
		return res, nil
	}

	if !cfg.AutoConfirm {
		proceed := true
		if err := huh.NewForm(huh.NewGroup(
			huh.NewConfirm().Title("Proceed?").Value(&proceed),
		)).WithAccessible(cfg.Accessible).Run(); err != nil {
			return res, err
		}
		if !proceed {
			return res, nil // clean, no-apply return (FR-3.1)
		}
	}

	done := showApplyProgress(os.Stdout, cfg.Accessible)
	applyErr := cfg.ApplyFn(selected)
	done()
	if applyErr != nil {
		return res, applyErr
	}
	res.Done = true
	printDone(os.Stdout)
	return res, nil
}

// showApplyProgress prints a start indicator and returns a func to print the
// finished state. ApplyFn runs synchronously to completion in one call, so
// there is no incremental percentage to animate — the bar just moves from
// empty to full around the blocking call.
func showApplyProgress(w *os.File, accessible bool) func() {
	if accessible {
		fmt.Fprintln(w, "applying...")
		return func() { fmt.Fprintln(w, "done") }
	}
	p := progress.New(progress.WithDefaultGradient())
	fmt.Fprintln(w, p.ViewAs(0))
	return func() { fmt.Fprintln(w, p.ViewAs(1)) }
}

// printDone renders the verify/done summary (FR-3.5): the znf-hook wiring
// count and a nudge toward the next isolated-worktree step. It re-reads the
// hook count via a dry-run EnsureGlobalHooks call rather than parsing
// runApply's stdout text, so it never triggers a second real write.
func printDone(w *os.File) {
	fmt.Fprintln(w, "onboarding complete")
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if ch, err := apply.EnsureGlobalHooks(home, true); err == nil {
			fmt.Fprintf(w, "wired %d znf hooks into ~/.claude/settings.json\n", ch.Total())
		}
	}
	fmt.Fprintln(w, "next: run `wt new <task>` to start your first isolated feature branch")
}

// loginStep runs the gh preflight (FR-1.1) and identity check (FR-1.2)
// before discover/select/plan ever run, so a bare `zenify up` with no gh
// auth reaches this step instead of failing deep inside buildPlan.
//
// Not a running bubbletea Program at this point (huh.Form.Run manages its
// own Program per call, and RunOnboard itself never starts one), so
// tea.ExecProcess — which sends a Cmd into an already-running Program's
// Update loop — has nothing to suspend/resume. Instead the interactive path
// runs `gh auth login --web` directly via cmd.Run() with stdio attached to
// the real terminal, which gives the same effect: the wizard blocks here,
// the browser login happens, and RunOnboard continues once it returns.
func loginStep(cfg OnboardConfig) error {
	detect := cfg.DetectGHFn
	if detect == nil {
		detect = detectGH
	}
	if err := detect(); err != nil {
		return err
	}

	authStatus := cfg.AuthStatusFn
	if authStatus == nil {
		authStatus = ghAuthStatus
	}
	if _, ok := authStatus(); ok {
		return nil
	}

	if cfg.Accessible {
		// Headless / non-TTY path (FR-1.4): never open a browser, fail
		// with a friendly instruction instead.
		return errors.New("not logged in — run: gh auth login")
	}

	// Interactive path: suspend the wizard, run gh's browser login, then
	// re-check status once it returns.
	cmd := loginCmd()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	if _, ok := authStatus(); !ok {
		return errors.New("not logged in — run: gh auth login")
	}
	return nil
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
