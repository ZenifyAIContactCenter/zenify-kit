package cli

import (
	"fmt"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/wt"
	"github.com/spf13/cobra"
)

// resolveHotfixBaseRef loads the repo's worktree.json and resolves its hotfix base
// through wt.HotfixBase — the same resolver `wt new --type hotfix` uses.
func resolveHotfixBaseRef(repoRoot string) (string, error) {
	c, err := wt.Load(repoRoot)
	if err != nil {
		return "", err
	}
	return wt.HotfixBase(c)
}

func newHotfixCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "hotfix", Short: "trợ giúp hotfix"} //znf:allow-lang
	baseref := &cobra.Command{
		Use:   "baseref <repoPath>",
		Short: "in base ref hotfix đã resolve theo chiến lược trong worktree.json", //znf:allow-lang
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := resolveHotfixBaseRef(args[0])
			if err != nil {
				return exitcode.New(exitcode.Fail, err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), ref)
			return nil
		},
	}
	cmd.AddCommand(baseref)
	return cmd
}
