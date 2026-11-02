package cli

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/wt"
	"github.com/spf13/cobra"
)

// newEnsureCmd exposes ensureWorkspace on its own, for a machine that never runs
// `zenify up` and has no workspace manifest for the SessionStart hook to find:
// a container worker gets the same hooks, model pin and env as a teammate's
// laptop on the same kit version, instead of a hand-written copy that drifts.
func newEnsureCmd() *cobra.Command {
	var workspace string
	cmd := &cobra.Command{
		Use:   "ensure",
		Short: "Đưa máy và workspace về trạng thái kit cần (hook, model, env), không cần zenify up", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if workspace == "" {
				workspace, _ = os.Getwd()
				// Run from a repo inside a workspace: target that workspace,
				// never the repo, or a nested marker would shadow it.
				if root, ok := wt.FindWorkspaceRoot(workspace); ok {
					workspace = root
				}
			}
			ws, err := filepath.Abs(workspace)
			if err != nil {
				return exitcode.New(exitcode.BadArgs, err)
			}
			home, err := os.UserHomeDir()
			if err != nil || home == "" {
				return exitcode.New(exitcode.Fail, errors.New("không xác định được thư mục home")) //znf:allow-lang
			}
			ensureWorkspace(ws, home, cmd.OutOrStdout(), cmd.ErrOrStderr())
			return nil
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "thư mục workspace nhận model pin (mặc định: workspace chứa thư mục hiện tại, không có thì thư mục hiện tại)") //znf:allow-lang
	return cmd
}
