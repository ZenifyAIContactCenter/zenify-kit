package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
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
		Short: "Offboard: gỡ znf global hooks, .worktrees/ excludes, và owned settings skeletons (preview mặc định; --apply để thực thi)", //znf:allow-lang
		RunE: func(cmd *cobra.Command, _ []string) error {
			if overlayPath == "" {
				overlayPath = filepath.Join(workspace, ".zenify-overlay.yaml")
			}
			w := cmd.OutOrStdout()
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
			_, _ = fmt.Fprintf(w, "zenify down — %s\n", mode)

			// 1. Global hooks.
			if home, herr := os.UserHomeDir(); herr == nil && home != "" {
				res, rerr := apply.RemoveGlobalHooks(home, dryRun)
				switch {
				case rerr != nil:
					_, _ = fmt.Fprintf(w, "  hooks: bỏ qua (%v)\n", rerr) //znf:allow-lang
				case res.Removed > 0:
					_, _ = fmt.Fprintf(w, "  hooks: gỡ %d znf hook khỏi ~/.claude/settings.json\n", res.Removed) //znf:allow-lang
				default:
					_, _ = fmt.Fprintln(w, "  hooks: không có znf hook nào để gỡ") //znf:allow-lang
				}
			}

			// 2 + 3. Each repo: exclude line + owned settings skeleton.
			for _, r := range m.Repos {
				repoDir := filepath.Join(workspace, r.Path)
				if removed, eerr := apply.RemoveExclude(repoDir, dryRun); eerr != nil {
					_, _ = fmt.Fprintf(w, "  %s exclude: lỗi %v\n", r.Name, eerr) //znf:allow-lang
				} else if removed {
					_, _ = fmt.Fprintf(w, "  %s: gỡ dòng .worktrees/\n", r.Name) //znf:allow-lang
				}
				settings := filepath.Join(repoDir, ".claude", "settings.local.json")
				if act, serr := apply.RemoveOwnedSettings(settings, owned, dryRun); serr != nil {
					_, _ = fmt.Fprintf(w, "  %s settings: lỗi %v\n", r.Name, serr) //znf:allow-lang
				} else if act == "removed" || act == "kept (modified)" {
					_, _ = fmt.Fprintf(w, "  %s settings: %s\n", r.Name, act)
				}
			}

			_, _ = fmt.Fprintln(w, "(down KHÔNG đụng cloned repo, .zenify/, hay docs store)") //znf:allow-lang
			return nil
		},
	}
	cmd.Flags().BoolVar(&applyFlag, "apply", false, "thực thi gỡ (mặc định chỉ preview)") //znf:allow-lang
	cmd.Flags().StringVar(&workspace, "workspace", ".", "workspace root directory")
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "path to repos.yaml (default: manifest/repos.yaml under cwd when present, else the copy embedded in the binary)")
	cmd.Flags().StringVar(&overlayPath, "overlay", "", "path to personal overlay")
	return cmd
}
