// internal/cli/hookactions.go
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitstate"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/observe"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/wt"
)

const znfDisciplineSentinel = "znf-discipline-local"

// runDocsSyncHook drives docsSyncCore for the docs-sync hook id. Sync notes
// go to stderr (matching the `docs sync` subcommand); stdout always gets the
// hook's required "{}" so Claude Code's hook-output parser is satisfied.
// Fail-open: any error from the core is logged, never surfaced as a failure.
func runDocsSyncHook(wsRoot string, w io.Writer) int {
	defer failOpen(w, nil)
	if err := docsSyncCore(wsRoot, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "znf docs-sync:", err)
	}
	fmt.Fprintln(w, "{}")
	return 0
}

// runObserveHook calls the already-extracted observe cores directly. count's
// stdout MUST be w (not io.Discard) — the soft-cap warning IS the hook's
// output; discarding it would silently drop the entire feature. Neither core
// gets a trailing "{}" appended here: they own their own stdout contract
// (count emits a warn-JSON only when warning, meter emits nothing), and both
// self-recover from panic internally, so this wrapper only routes.
func runObserveHook(wsRoot, kind string, w io.Writer) int {
	switch kind {
	case "count":
		return runObserveCount(os.Stdin, w, os.Getenv, observe.Bump)
	case "meter":
		return runObserveMeter(os.Stdin, observe.Record)
	}
	return 0
}

// runSessionStart is the Go body of the SessionStart hook (it runs as a global
// hook, so no $CLAUDE_PLUGIN_ROOT is available; the materialized skill path is
// used directly). Order matters: docs-sync first so the store is current, then
// ensureWorkspace so freshly pulled rules land in .claude/rules — the harness
// has already loaded this session's settings/rules by the time SessionStart
// fires, so the writes take effect from the next session; the CREATE/UPDATE
// lines forwarded to stdout are what tell the CURRENT session its on-disk
// rules are newer than what it loaded. Then the BOOTSTRAP digest for machines
// without the discipline sentinel.
func runSessionStart(wsRoot string, w io.Writer) int {
	defer failOpen(w, nil)
	if err := docsSyncCore(wsRoot, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "znf docs-sync:", err)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		ensureWorkspace(wsRoot, home, w, os.Stderr)
	}
	if !sentinelPresent() {
		digest := readBootstrapDigest()
		if digest != "" {
			fmt.Fprint(w, digest)
			return 0
		}
	}
	return 0
}

func sentinelPresent() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	raw, err := os.ReadFile(filepath.Join(home, ".claude", "CLAUDE.md")) //nolint:gosec // G304 -- path is ~/.claude/CLAUDE.md, computed from home, not user input
	if err != nil {
		return false
	}
	return strings.Contains(string(raw), znfDisciplineSentinel)
}

func readBootstrapDigest() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	p := filepath.Join(home, ".claude", "skills", "znf", "skills", "using-zenify-kit", "BOOTSTRAP.txt")
	raw, err := os.ReadFile(p) //nolint:gosec // G304 -- p is a fixed path under ~/.claude, computed from home, not user input
	if err != nil {
		return ""
	}
	return string(raw)
}

// runGitStateHook prints the repo-state report for the workspace (FR-6).
// Session mode: plain text — Claude Code adds SessionStart stdout to context.
// Stop mode: Claude Code does NOT surface plain Stop stdout, so the standing
// alarm travels as {"systemMessage": ...}; a clean workspace prints "{}".
// Scope base is $CLAUDE_PROJECT_DIR when set (the directory the session was
// opened in), else wsRoot; deploy tiers stop at wsRoot's own .claude/deploy-branches
// (stopAt is inclusive — nothing above wsRoot is read).
func runGitStateHook(wsRoot string, mode gitstate.Mode, w io.Writer) int {
	var wrote bool
	defer failOpen(w, &wrote)
	base := os.Getenv("CLAUDE_PROJECT_DIR")
	if base == "" {
		base = wsRoot
	}
	report := gitstate.Run(base, wsRoot, mode)
	// Built into a string and written once at the end (rather than at each
	// early return) so failOpen can tell, via wrote, whether a panic during
	// gitstate.Run left the hook's contract unfulfilled.
	var out string
	switch {
	case mode == gitstate.Stop && report == "":
		out = "{}\n"
	case mode == gitstate.Stop:
		b, _ := json.Marshal(map[string]string{"systemMessage": report})
		out = string(b) + "\n"
	case report == "":
		// Session mode, nothing to report: SessionStart stdout is injected
		// verbatim as context, so a literal "{}" would land in the model's
		// context as noise — print nothing instead.
	default:
		out = report
	}
	wrote = true
	if out != "" {
		fmt.Fprint(w, out)
	}
	return 0
}

// failOpen guarantees the hook never crashes the user's session: a panic is
// recovered and swallowed so dispatchHook's caller still sees exit 0. wrote
// tracks whether the caller had already written its output before the panic;
// when non-nil and still false, a bare "{}" is emitted so a hook whose
// contract requires JSON (e.g. the Stop hook) never comes back empty. Pass
// nil to opt out — used by callers whose contract has no such requirement.
func failOpen(w io.Writer, wrote *bool) {
	if r := recover(); r != nil {
		fmt.Fprintln(os.Stderr, "znf hook recovered:", r)
		if wrote != nil && !*wrote {
			fmt.Fprintln(w, "{}")
		}
	}
}

// runWtReportHook prints ONE line naming the merged worktrees and stale state
// entries `wt sweep --all` would remove across the workspace — or nothing.
// Read-only and bounded: no fetch, no lsof, each git call capped at 5s, and a
// repo that errors is dropped from the tally rather than failing the hook.
// SessionStart stdout is injected verbatim as context, so silence (not "{}")
// is the empty case, matching runGitStateHook. failOpen is pointed at
// io.Discard here (not w): on a panic before wrote flips true, "{}" is
// swallowed there instead of reaching w, so this hook never prints "{}".
func runWtReportHook(wsRoot string, w io.Writer) int {
	var wrote bool
	defer failOpen(io.Discard, &wrote) // never emit "{}" on this hook
	r := gitx.TimeoutRunner(5 * time.Second)
	var counts []wt.RepoCount
	for _, repo := range wt.WorkspaceRepos(wsRoot) {
		c, err := wt.CountSweepable(r, repo)
		if err != nil {
			continue
		}
		counts = append(counts, c)
	}
	if line := wt.FormatReport(counts); line != "" {
		fmt.Fprintln(w, line)
	}
	wrote = true
	return 0
}
