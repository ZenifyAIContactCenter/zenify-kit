package cli

import (
	"fmt"
	"os"
	"path/filepath"

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

	var repo, dest string
	install := &cobra.Command{
		Use:   "install",
		Short: "materialize coding skill (leg-1) cho repo hiện tại vào .claude/skills",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if repo == "" {
				wd, err := os.Getwd()
				if err != nil {
					return exitcode.New(exitcode.Fail, err)
				}
				repo = filepath.Base(wd)
			}
			if dest == "" {
				dest = filepath.Join(".claude", "skills")
			}
			skills := plugin.SkillsForRepo(repo)
			if len(skills) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "repo %q không có coding skill trong footprint map\n", repo)
				return nil
			}
			man := filepath.Join(dest, ".manifest.json")
			res, err := plugin.InstallCoding(dest, man, skills)
			if err != nil {
				return exitcode.New(exitcode.Fail, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "install %s: %d ghi, %d giữ, %d không đổi → %s\n",
				repo, len(res.Written), len(res.Kept), len(res.Skipped), dest)
			return nil
		},
	}
	install.Flags().StringVar(&repo, "repo", "", "tên repo (mặc định: basename cwd)")
	install.Flags().StringVar(&dest, "dest", "", "thư mục đích (mặc định: .claude/skills)")
	cmd.AddCommand(install)

	return cmd
}
