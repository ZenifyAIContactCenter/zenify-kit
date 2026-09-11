// internal/cli/hookactions.go
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/observe"
)

const znfDisciplineSentinel = "znf-discipline-local"

// runDocsSyncHook drives docsSyncCore for the docs-sync hook id. Sync notes
// go to stderr (matching the `docs sync` subcommand); stdout always gets the
// hook's required "{}" so Claude Code's hook-output parser is satisfied.
// Fail-open: any error from the core is logged, never surfaced as a failure.
func runDocsSyncHook(wsRoot string, w io.Writer) int {
	defer failOpen(w)
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
	defer failOpen(w)
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

// failOpen guarantees the hook never crashes the user's session: a panic is
// recovered and swallowed so dispatchHook's caller still sees exit 0.
func failOpen(w io.Writer) {
	if r := recover(); r != nil {
		fmt.Fprintln(os.Stderr, "znf hook recovered:", r)
	}
}
