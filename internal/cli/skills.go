package cli

import (
	"fmt"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/plugin"
	"github.com/spf13/cobra"
)

func newSkillsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "skills", Short: "quản lý plugin skill znf"}
	sync := &cobra.Command{
		Use:   "sync",
		Short: "materialize plugin znf vào ~/.claude/skills/znf",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dest, err := plugin.DefaultDest()
			if err != nil {
				return exitcode.New(exitcode.Fail, err)
			}
			man, err := plugin.DefaultManifest()
			if err != nil {
				return exitcode.New(exitcode.Fail, err)
			}
			res, err := plugin.Sync(dest, man)
			if err != nil {
				return exitcode.New(exitcode.Fail, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "znf sync: %d ghi, %d giữ (user sửa), %d không đổi → %s\n",
				len(res.Written), len(res.Kept), len(res.Skipped), dest)
			return nil
		},
	}
	cmd.AddCommand(sync)
	return cmd
}
