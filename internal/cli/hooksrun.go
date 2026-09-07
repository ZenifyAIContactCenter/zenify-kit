package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// findWorkspaceRoot walks up from start to find an ancestor containing
// .zenify/manifest.json. This is the workspace marker written by `zenify up`.
func findWorkspaceRoot(start string) (string, bool) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	for {
		marker := filepath.Join(dir, ".zenify", "manifest.json")
		if fi, err := os.Stat(marker); err == nil && !fi.IsDir() {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false // reached filesystem root
		}
		dir = parent
	}
}

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
		return runSessionStart(wsRoot, w) // Task 4/5
	case "docs-sync":
		return runDocsSyncHook(wsRoot, w) // Task 4
	case "observe-count":
		return runObserveHook(wsRoot, "count", w) // Task 4
	case "observe-meter":
		return runObserveHook(wsRoot, "meter", w) // Task 4
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
