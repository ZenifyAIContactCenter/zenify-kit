package cli

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitstate"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/observe"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/wt"
	"github.com/spf13/cobra"
)

// findWorkspaceRoot delegates to wt.FindWorkspaceRoot (moved there so
// internal/wt can use it without importing cli). A var so internal/cli's
// TestMain can refuse the real workspace above the package directory.
var findWorkspaceRoot = wt.FindWorkspaceRoot

// writeGuardRun is the write-guard entry point; a var so tests can stub it.
var writeGuardRun = func() int { return runWriteGuard(os.Stdin, os.Stderr, wt.FindWorkspaceRoot) }

// dispatchHook runs the action for id within workspace wsRoot. Every id
// returns 0 (fail-open) except read-guard and write-guard, which return 2 to
// deny a Read/Write the harness would otherwise let through; a hook must never
// make CC report an error for any other reason. wsRoot == "" means "outside a
// workspace" => no-op, except for write-guard, which runs first because it
// derives the workspace from the target file path, not from the hook's cwd.
func dispatchHook(id, wsRoot string, w io.Writer) int {
	noop := func() int { fmt.Fprintln(w, "{}"); return 0 }
	if id == "write-guard" {
		return writeGuardRun()
	}
	if id == "hub-report" {
		// Reports agent state by tmux pane, not by workspace, and prints nothing.
		runHubReport(os.Stdin, os.Getenv("TMUX_PANE"), time.Now().Unix())
		return 0
	}
	if wsRoot == "" {
		return noop()
	}
	switch id {
	case "session-start":
		return runSessionStart(wsRoot, w)
	case "docs-sync":
		return runDocsSyncHook(wsRoot, w)
	case "observe-count":
		return runObserveHook(wsRoot, "count", w)
	case "observe-meter":
		return runObserveHook(wsRoot, "meter", w)
	case "git-state":
		return runGitStateHook(wsRoot, gitstate.Session, w)
	case "git-state-stop":
		return runGitStateHook(wsRoot, gitstate.Stop, w)
	case "wt-report":
		return runWtReportHook(wsRoot, w)
	case "read-guard":
		return runReadGuard(os.Stdin, os.Stderr, os.Stat, observe.Record)
	default:
		return noop() // unknown id: fail-open
	}
}

func newHooksRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "hooks-run <id>",
		Short:  "Bộ điều phối nội bộ cho hook znf (chỉ chạy trong workspace, fail-open)", //znf:allow-lang
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, _ := os.Getwd()
			wsRoot, _ := findWorkspaceRoot(cwd)
			code := dispatchHook(args[0], wsRoot, cmd.OutOrStdout())
			if code != 0 {
				os.Exit(code) // read-guard/write-guard deny (2); every other id is 0
			}
			return nil
		},
	}
}
