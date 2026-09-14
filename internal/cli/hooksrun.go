package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitstate"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/wt"
	"github.com/spf13/cobra"
)

// findWorkspaceRoot delegates to wt.FindWorkspaceRoot (moved there so
// internal/wt can use it without importing cli).
func findWorkspaceRoot(start string) (string, bool) { return wt.FindWorkspaceRoot(start) }

// dispatchHook runs the action for id within workspace wsRoot. It ALWAYS
// returns 0 (fail-open): a hook must never make CC report an error.
// wsRoot == "" means "outside a workspace" => no-op.
func dispatchHook(id, wsRoot string, w io.Writer) int {
	noop := func() int { fmt.Fprintln(w, "{}"); return 0 }
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
	default:
		return noop() // unknown id: fail-open
	}
}

func newHooksRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "hooks-run <id>",
		Short:  "Internal dispatcher for znf hooks (workspace-guarded, fail-open)",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, _ := os.Getwd()
			wsRoot, _ := findWorkspaceRoot(cwd)
			code := dispatchHook(args[0], wsRoot, cmd.OutOrStdout())
			if code != 0 {
				os.Exit(code) // should never happen at P1
			}
			return nil
		},
	}
}
