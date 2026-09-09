package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/plugin"
	"github.com/spf13/cobra"
)

func newSkillsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "skills", Short: "quản lý plugin skill znf"} //znf:allow-lang
	sync := &cobra.Command{
		Use:   "sync",
		Short: "materialize plugin znf vào ~/.claude/skills/znf", //znf:allow-lang
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
			fmt.Fprintf(cmd.OutOrStdout(), "znf sync: %d ghi, %d giữ (user sửa), %d không đổi → %s\n", //znf:allow-lang
				len(res.Written), len(res.Kept), len(res.Skipped), dest)

			home, _ := os.UserHomeDir()
			if home != "" {
				ch, err := apply.EnsureGlobalHooks(home, false)
				if err != nil {
					fmt.Fprintln(cmd.ErrOrStderr(), "warning: could not wire znf hooks:", err)
				} else if n := ch.Added + ch.Updated; n > 0 {
					// Only announce when something actually changed — an
					// already-wired session would otherwise print a "wired N"
					// line on every run (all Unchanged), which reads as noise.
					fmt.Fprintf(cmd.OutOrStdout(),
						"wired %d znf hooks into ~/.claude/settings.json\n", n)
				}
			}
			return nil
		},
	}
	cmd.AddCommand(sync)

	var repo, dest string
	install := &cobra.Command{
		Use:   "install",
		Short: "materialize coding skill (leg-1) cho repo hiện tại vào .claude/skills", //znf:allow-lang
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
				fmt.Fprintf(cmd.OutOrStdout(), "repo %q không có coding skill trong footprint map\n", repo) //znf:allow-lang
				return nil
			}
			man := filepath.Join(dest, ".manifest.json")
			res, err := plugin.InstallCoding(dest, man, skills)
			if err != nil {
				return exitcode.New(exitcode.Fail, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "install %s: %d ghi, %d giữ, %d không đổi → %s\n", //znf:allow-lang
				repo, len(res.Written), len(res.Kept), len(res.Skipped), dest)
			if recs := plugin.Leg2ForRepo(repo); len(recs) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "\nKhuyến nghị third-party (chạy thủ công rồi commit):\n") //znf:allow-lang
				for _, r := range recs {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", r)
				}
			}
			return nil
		},
	}
	install.Flags().StringVar(&repo, "repo", "", "tên repo (mặc định: basename cwd)")       //znf:allow-lang
	install.Flags().StringVar(&dest, "dest", "", "thư mục đích (mặc định: .claude/skills)") //znf:allow-lang
	cmd.AddCommand(install)

	return cmd
}
