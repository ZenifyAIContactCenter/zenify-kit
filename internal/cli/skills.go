package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/plugin"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ui"
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
			u := uiOut(cmd)
			u.Step(ui.StatusOK, "znf sync", //znf:allow-lang
				fmt.Sprintf("%d ghi, %d giữ (user sửa), %d không đổi, %d gỡ → %s", //znf:allow-lang
					len(res.Written), len(res.Kept), len(res.Skipped), len(res.Removed), dest))

			home, _ := os.UserHomeDir()
			if home != "" {
				ch, err := apply.EnsureGlobalHooks(home, false)
				if err != nil {
					uiErr(cmd).Step(ui.StatusWarn, "could not wire znf hooks", err.Error())
				} else if n := ch.Added + ch.Updated; n > 0 {
					// Only announce when something actually changed — an
					// already-wired session would otherwise print a "wired N"
					// line on every run (all Unchanged), which reads as noise.
					u.Step(ui.StatusOK, fmt.Sprintf("wired %d znf hooks into ~/.claude/settings.json", n), "")
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
			u := uiOut(cmd)
			if len(skills) == 0 {
				u.Step(ui.StatusInfo, fmt.Sprintf("repo %q không có coding skill trong footprint map", repo), "") //znf:allow-lang
				return nil
			}
			man := filepath.Join(dest, ".manifest.json")
			res, err := plugin.InstallCoding(dest, man, skills)
			if err != nil {
				return exitcode.New(exitcode.Fail, err)
			}
			u.Step(ui.StatusOK, fmt.Sprintf("install %s", repo), //znf:allow-lang
				fmt.Sprintf("%d ghi, %d giữ, %d không đổi → %s", len(res.Written), len(res.Kept), len(res.Skipped), dest)) //znf:allow-lang
			if recs := plugin.Leg2ForRepo(repo); len(recs) > 0 {
				u.Section("Khuyến nghị third-party (chạy thủ công rồi commit):") //znf:allow-lang
				for _, r := range recs {
					u.Note(r)
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
