package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ui"
	"github.com/spf13/cobra"
)

func newDownCmd() *cobra.Command {
	var (
		applyFlag    bool
		workspace    string
		manifestPath string
		overlayPath  string
	)
	cmd := &cobra.Command{
		Use:   "down",
		Short: "Offboard: gỡ znf global hooks + env subagent model, .worktrees/ + .wt/ excludes, và owned settings skeletons (preview mặc định; --apply để thực thi)", //znf:allow-lang
		RunE: func(cmd *cobra.Command, _ []string) error {
			if overlayPath == "" {
				overlayPath = filepath.Join(workspace, ".zenify-overlay.yaml")
			}
			u := uiOut(cmd)
			dryRun := !applyFlag

			m, _, err := loadKitManifest(manifestPath, overlayPath)
			if err != nil {
				return exitcode.New(exitcode.Fail, err)
			}
			owned, err := managed.Load(filepath.Join(workspace, ".zenify", "manifest.json"))
			if err != nil {
				return exitcode.New(exitcode.Fail, err)
			}

			mode := "APPLY"
			if dryRun {
				mode = "PREVIEW (dry-run — dùng --apply để thực thi)" //znf:allow-lang
			}
			u.Header(fmt.Sprintf("zenify down — %s", mode))

			// 1. Global hooks.
			if home, herr := os.UserHomeDir(); herr == nil && home != "" {
				res, rerr := apply.RemoveGlobalHooks(home, dryRun)
				switch {
				case rerr != nil:
					u.Step(ui.StatusWarn, "hooks", fmt.Sprintf("bỏ qua (%v)", rerr)) //znf:allow-lang
				case res.Removed > 0:
					u.Step(ui.StatusOK, "hooks", fmt.Sprintf("gỡ %d znf hook khỏi ~/.claude/settings.json", res.Removed)) //znf:allow-lang
				default:
					u.Step(ui.StatusInfo, "hooks", "không có znf hook nào để gỡ") //znf:allow-lang
				}
				removed, eerr := apply.RemoveSubagentModelEnv(home, dryRun)
				switch {
				case eerr != nil:
					u.Step(ui.StatusWarn, "env", fmt.Sprintf("bỏ qua (%v)", eerr)) //znf:allow-lang
				case removed:
					u.Step(ui.StatusOK, "env", fmt.Sprintf("gỡ %s khỏi ~/.claude/settings.json", apply.SubagentModelEnv)) //znf:allow-lang
				}
				removedStrong, serr := apply.RemoveStrongModelEnv(home, dryRun)
				switch {
				case serr != nil:
					u.Step(ui.StatusWarn, "env", fmt.Sprintf("bỏ qua (%v)", serr)) //znf:allow-lang
				case removedStrong:
					u.Step(ui.StatusOK, "env", fmt.Sprintf("gỡ %s khỏi ~/.claude/settings.json", apply.StrongModelEnv)) //znf:allow-lang
				}
			}

			// 2 + 3. Each repo: exclude line + owned settings skeleton.
			for _, r := range m.Repos {
				repoDir := filepath.Join(workspace, r.Path)
				if removed, eerr := apply.RemoveExclude(repoDir, dryRun); eerr != nil {
					u.Step(ui.StatusFail, r.Name+" exclude", fmt.Sprintf("lỗi %v", eerr)) //znf:allow-lang
				} else if removed {
					u.Step(ui.StatusOK, r.Name, "gỡ dòng .worktrees/ + .wt/") //znf:allow-lang
				}
				settings := filepath.Join(repoDir, ".claude", "settings.local.json")
				if act, serr := apply.RemoveOwnedSettings(settings, owned, dryRun); serr != nil {
					u.Step(ui.StatusFail, r.Name+" settings", fmt.Sprintf("lỗi %v", serr)) //znf:allow-lang
				} else if act == "removed" || act == "kept (modified)" {
					u.Step(ui.StatusOK, r.Name+" settings", act)
				}
			}

			u.Blank()
			u.Note("(down KHÔNG đụng cloned repo, .zenify/, hay docs store)") //znf:allow-lang
			return nil
		},
	}
	cmd.Flags().BoolVar(&applyFlag, "apply", false, "thực thi gỡ (mặc định chỉ preview)") //znf:allow-lang
	cmd.Flags().StringVar(&workspace, "workspace", ".", "workspace root directory")
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "path to repos.yaml (default: manifest/repos.yaml under cwd when present, else the copy embedded in the binary)")
	cmd.Flags().StringVar(&overlayPath, "overlay", "", "path to personal overlay")
	return cmd
}
