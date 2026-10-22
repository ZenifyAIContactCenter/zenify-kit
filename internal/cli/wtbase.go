package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/wt"
	"github.com/spf13/cobra"
)

// resolveWtBase returns the ref this worktree was really cut from: wt.base
// (recorded by `wt new`), else baseRef of .claude/worktree.json, else
// origin/main. Skills use it as the merge-base target, since a worktree
// branched from a feature branch must not be diffed against the repo trunk.
func resolveWtBase(r gitx.Runner, worktree string, load func(string) (*wt.Config, error)) string {
	if b, err := r.Run(worktree, "config", "--worktree", "--get", "wt.base"); err == nil {
		if v := strings.TrimSpace(string(b)); v != "" {
			return v
		}
	}
	if load != nil {
		if cfg, err := load(worktree); err == nil && cfg != nil && cfg.BaseRef != "" {
			return cfg.BaseRef
		}
	}
	return "origin/main"
}

func newWtBaseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "base",
		Short: "In base thật của worktree hiện tại (wt.base, rồi baseRef, rồi origin/main)", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			load := func(string) (*wt.Config, error) {
				root, err := repoRoot()
				if err != nil {
					return nil, err
				}
				return wt.Load(root)
			}
			// Output contract: exactly the ref + newline, so `$(zenify wt base)` is usable.
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), resolveWtBase(gitx.ExecRunner(), cwd, load))
			return nil
		},
	}
}
