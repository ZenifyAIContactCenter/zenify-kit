package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/plugin"
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
		Short: "Offboard: gỡ znf global hooks + git-guard hook + env subagent model, .worktrees/ + .wt/ excludes, và owned settings skeletons (preview mặc định; --apply để thực thi)", //znf:allow-lang
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
				codexRes, cerr := apply.RemoveCodexHooks(codexHomeDir(home, os.Getenv), dryRun)
				switch {
				case cerr != nil:
					u.Step(ui.StatusWarn, "codex-hooks", fmt.Sprintf("bỏ qua (%v)", cerr)) //znf:allow-lang
				case codexRes.Removed > 0:
					u.Step(ui.StatusOK, "codex-hooks", fmt.Sprintf("gỡ %d hook khỏi hooks.json của Codex", codexRes.Removed)) //znf:allow-lang
				default:
					u.Step(ui.StatusInfo, "codex-hooks", "không có hook Codex nào để gỡ") //znf:allow-lang
				}
				agentNames, aerr := plugin.RemoveCodexAgents(codexHomeDir(home, os.Getenv), dryRun)
				switch {
				case aerr != nil:
					u.Step(ui.StatusWarn, "codex-agents", fmt.Sprintf("bỏ qua (%v)", aerr)) //znf:allow-lang
				case len(agentNames) > 0:
					u.Step(ui.StatusOK, "codex-agents", fmt.Sprintf("gỡ %d agent khỏi thư mục agents của Codex", len(agentNames))) //znf:allow-lang
				default:
					u.Step(ui.StatusInfo, "codex-agents", "không có agent Codex nào để gỡ") //znf:allow-lang
				}
				linkRemoved, lerr := apply.RemoveCodexSkillsLink(home, dryRun)
				switch {
				case lerr != nil:
					u.Step(ui.StatusWarn, "codex-skills", fmt.Sprintf("bỏ qua (%v)", lerr)) //znf:allow-lang
				case linkRemoved:
					u.Step(ui.StatusOK, "codex-skills", "gỡ symlink ~/.agents/skills/znf") //znf:allow-lang
				default:
					u.Step(ui.StatusInfo, "codex-skills", "không có symlink skill Codex nào để gỡ") //znf:allow-lang
				}
				// git-guard is not in the znf hook set RemoveGlobalHooks knows,
				// but up/SessionStart install it, so offboarding removes it too.
				gRemoved, gerr := removeGuard(home, dryRun)
				switch {
				case gerr != nil:
					u.Step(ui.StatusWarn, "git-guard", fmt.Sprintf("skipped (%v)", gerr))
				case gRemoved:
					u.Step(ui.StatusOK, "git-guard", "removed git-guard hook from ~/.claude/settings.json")
				default:
					u.Step(ui.StatusInfo, "git-guard", "no git-guard hook to remove")
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
				removedRev, rverr := apply.RemoveReviewerEnv(home, dryRun)
				switch {
				case rverr != nil:
					u.Step(ui.StatusWarn, "env", fmt.Sprintf("bỏ qua (%v)", rverr)) //znf:allow-lang
				case removedRev:
					u.Step(ui.StatusOK, "env", fmt.Sprintf("gỡ %s khỏi ~/.claude/settings.json", apply.ReviewerEnv)) //znf:allow-lang
				}
				removedPerms, perr := apply.RemoveKitPermissions(home, dryRun)
				switch {
				case perr != nil:
					u.Step(ui.StatusWarn, "permissions", fmt.Sprintf("bỏ qua (%v)", perr)) //znf:allow-lang
				case removedPerms > 0:
					u.Step(ui.StatusOK, "permissions", fmt.Sprintf("gỡ %d rule allow của kit khỏi ~/.claude/settings.json", removedPerms)) //znf:allow-lang
				}
				removedSbx, sberr := apply.RemoveKitSandbox(home, dryRun)
				switch {
				case sberr != nil:
					u.Step(ui.StatusWarn, "sandbox", fmt.Sprintf("bỏ qua (%v)", sberr)) //znf:allow-lang
				case removedSbx > 0:
					u.Step(ui.StatusOK, "sandbox", fmt.Sprintf("gỡ %d lệnh kit khỏi sandbox.excludedCommands", removedSbx)) //znf:allow-lang
				}
			}

			// Workspace deny rule for .zenify/.env.
			removedDeny, derr := apply.RemoveKitDeny(workspace, dryRun)
			switch {
			case derr != nil:
				u.Step(ui.StatusWarn, "deny", fmt.Sprintf("bỏ qua (%v)", derr)) //znf:allow-lang
			case removedDeny > 0:
				u.Step(ui.StatusOK, "deny", fmt.Sprintf("gỡ %d luật deny của kit khỏi .claude/settings.json của workspace", removedDeny)) //znf:allow-lang
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
