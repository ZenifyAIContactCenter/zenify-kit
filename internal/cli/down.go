package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/mcpreg"
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
			// up keys path-based entries (Codex trust, kit records) by the
			// absolute workspace; the "." default must resolve the same way.
			if abs, err := filepath.Abs(workspace); err == nil {
				workspace = abs
			}
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

			// 1. Workspace-local install: plugin, MCP servers, hooks, env, permissions.
			home, herr := os.UserHomeDir()
			if herr != nil || home == "" {
				home = ""
			}
			if !dryRun {
				removeLocalClaude(u, workspace, pluginRunner(workspace))
			} else {
				u.Step(ui.StatusInfo, "plugin", "sẽ gỡ znf@zenify-kit (local) và marketplace zenify-kit") //znf:allow-lang
				u.Step(ui.StatusInfo, "mcp", "sẽ gỡ 3 MCP server zenify-* (local)")                       //znf:allow-lang
			}
			local := apply.WorkspaceLocalSettings(workspace)
			res, rerr := apply.RemoveGlobalHooks(local, dryRun)
			switch {
			case rerr != nil:
				u.Step(ui.StatusWarn, "hooks", fmt.Sprintf("bỏ qua (%v)", rerr)) //znf:allow-lang
			case res.Removed > 0:
				u.Step(ui.StatusOK, "hooks", fmt.Sprintf("gỡ %d znf hook khỏi .claude/settings.local.json", res.Removed)) //znf:allow-lang
			default:
				u.Step(ui.StatusInfo, "hooks", "không có znf hook nào để gỡ") //znf:allow-lang
			}
			gRemoved, gerr := removeGuard(local, dryRun)
			switch {
			case gerr != nil:
				u.Step(ui.StatusWarn, "git-guard", fmt.Sprintf("skipped (%v)", gerr))
			case gRemoved:
				u.Step(ui.StatusOK, "git-guard", "removed git-guard hook from .claude/settings.local.json")
			default:
				u.Step(ui.StatusInfo, "git-guard", "no git-guard hook to remove")
			}
			wsCodex := filepath.Join(workspace, ".codex")
			codexRes, cerr := apply.RemoveCodexHooks(wsCodex, dryRun)
			switch {
			case cerr != nil:
				u.Step(ui.StatusWarn, "codex-hooks", fmt.Sprintf("bỏ qua (%v)", cerr)) //znf:allow-lang
			case codexRes.Removed > 0:
				u.Step(ui.StatusOK, "codex-hooks", fmt.Sprintf("gỡ %d hook khỏi hooks.json của Codex", codexRes.Removed)) //znf:allow-lang
			default:
				u.Step(ui.StatusInfo, "codex-hooks", "không có hook Codex nào để gỡ") //znf:allow-lang
			}
			agentNames, aerr := plugin.RemoveCodexAgents(wsCodex, dryRun)
			switch {
			case aerr != nil:
				u.Step(ui.StatusWarn, "codex-agents", fmt.Sprintf("bỏ qua (%v)", aerr)) //znf:allow-lang
			case len(agentNames) > 0:
				u.Step(ui.StatusOK, "codex-agents", fmt.Sprintf("gỡ %d agent khỏi thư mục agents của Codex", len(agentNames))) //znf:allow-lang
			default:
				u.Step(ui.StatusInfo, "codex-agents", "không có agent Codex nào để gỡ") //znf:allow-lang
			}
			linkRemoved, lerr := apply.RemoveCodexSkillsLink(filepath.Join(workspace, ".agents", "skills", "znf"), dryRun)
			switch {
			case lerr != nil:
				u.Step(ui.StatusWarn, "codex-skills", fmt.Sprintf("bỏ qua (%v)", lerr)) //znf:allow-lang
			case linkRemoved:
				u.Step(ui.StatusOK, "codex-skills", "gỡ symlink .agents/skills/znf") //znf:allow-lang
			default:
				u.Step(ui.StatusInfo, "codex-skills", "không có symlink skill Codex nào để gỡ") //znf:allow-lang
			}
			for _, e := range []struct {
				key string
				fn  func(string, bool) (bool, error)
			}{{apply.SubagentModelEnv, apply.RemoveSubagentModelEnv}, {apply.ReviewerEnv, apply.RemoveReviewerEnv}} {
				ok, eerr := e.fn(local, dryRun)
				switch {
				case eerr != nil:
					u.Step(ui.StatusWarn, "env", fmt.Sprintf("bỏ qua (%v)", eerr)) //znf:allow-lang
				case ok:
					u.Step(ui.StatusOK, "env", fmt.Sprintf("gỡ %s khỏi .claude/settings.local.json", e.key)) //znf:allow-lang
				}
			}
			if home != "" {
				if _, terr := apply.RemoveCodexTrust(codexHomeDir(home, os.Getenv), workspace, dryRun); terr != nil {
					u.Step(ui.StatusWarn, "codex-trust", fmt.Sprintf("bỏ qua (%v)", terr)) //znf:allow-lang
				}
				kt := apply.WorkspaceKitTarget(home, workspace)
				for _, l := range []struct {
					what string
					fn   func(apply.KitTarget, bool) (int, error)
				}{{"permissions", apply.RemoveKitPermissions}, {"sandbox", apply.RemoveKitSandbox}, {"ask", apply.RemoveKitAsk}} {
					n, lerr := l.fn(kt, dryRun)
					switch {
					case errors.Is(lerr, fs.ErrNotExist):
					case lerr != nil:
						u.Step(ui.StatusWarn, l.what, fmt.Sprintf("bỏ qua (%v)", lerr)) //znf:allow-lang
					case n > 0:
						u.Step(ui.StatusOK, l.what, fmt.Sprintf("gỡ %d rule của kit khỏi .claude/settings.local.json", n)) //znf:allow-lang
					}
				}
			}

			// 2. User-level leftovers of older kits (machines not yet migrated).
			if home != "" {
				var run func(string, ...string) ([]byte, error)
				if !dryRun {
					run = pluginRunner(workspace)
				}
				removed, merr := migrateUserInstalls(home, codexHomeDir(home, os.Getenv), userKitMCPs, userKeep{}, run, dryRun)
				if len(removed) > 0 {
					u.Step(ui.StatusOK, "user-level", fmt.Sprintf("gỡ %d mục cài ở cấp user", len(removed))) //znf:allow-lang
				}
				if merr != nil {
					u.Step(ui.StatusWarn, "user-level", fmt.Sprintf("bỏ qua (%v)", merr)) //znf:allow-lang
				}
				if removedStrong, serr := apply.RemoveStrongModelEnv(apply.UserSettings(home), dryRun); serr != nil {
					u.Step(ui.StatusWarn, "env", fmt.Sprintf("bỏ qua (%v)", serr)) //znf:allow-lang
				} else if removedStrong {
					u.Step(ui.StatusOK, "env", fmt.Sprintf("gỡ %s khỏi ~/.claude/settings.json", apply.StrongModelEnv)) //znf:allow-lang
				}
				// This workspace's .zenify/.env deny rule (other workspaces' stay).
				removedDeny, derr := apply.RemoveKitDeny(home, workspace, dryRun)
				switch {
				case derr != nil:
					u.Step(ui.StatusWarn, "deny", fmt.Sprintf("bỏ qua (%v)", derr)) //znf:allow-lang
				case removedDeny > 0:
					u.Step(ui.StatusOK, "deny", fmt.Sprintf("gỡ %d luật deny .zenify/.env của workspace khỏi ~/.claude/settings.json", removedDeny)) //znf:allow-lang
				}
				removedShims, sherr := apply.RemoveShims(home, dryRun)
				switch {
				case sherr != nil:
					u.Step(ui.StatusWarn, "shims", fmt.Sprintf("bỏ qua (%v)", sherr)) //znf:allow-lang
				case len(removedShims) > 0:
					u.Step(ui.StatusOK, "shims", fmt.Sprintf("gỡ %d shim wt/pm khỏi ~/.local/bin", len(removedShims))) //znf:allow-lang
				}
			}

			// 3. Each repo: exclude line + owned settings skeleton.
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

// removeLocalClaude removes the workspace's local-scope plugin, marketplace and
// MCP registrations through claude. "Not installed / not found" answers (measured
// on 2.1.318) count as already clean, so down is idempotent; any other failure is
// a warning. The plugin stamp goes once the plugin is gone.
func removeLocalClaude(u *ui.Writer, workspace string, run func(string, ...string) ([]byte, error)) {
	pluginGone := false
	if out, err := run("claude", "plugin", "uninstall", plugin.PluginID, "--scope", "local"); err != nil && !strings.Contains(string(out), "is not installed in") {
		u.Step(ui.StatusWarn, "plugin", fmt.Sprintf("bỏ qua (%v: %s)", err, strings.TrimSpace(string(out)))) //znf:allow-lang
	} else {
		pluginGone = true
		u.Step(ui.StatusOK, "plugin", "gỡ znf@zenify-kit (local)") //znf:allow-lang
	}
	if out, err := run("claude", "plugin", "marketplace", "remove", plugin.MarketplaceName); err != nil && !marketplaceNotFound(string(out)) {
		u.Step(ui.StatusWarn, "marketplace", fmt.Sprintf("bỏ qua (%v: %s)", err, strings.TrimSpace(string(out)))) //znf:allow-lang
	}
	if pluginGone {
		if err := os.Remove(pluginStampPath(workspace)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			u.Step(ui.StatusWarn, "plugin", fmt.Sprintf("không xoá được stamp (%v)", err)) //znf:allow-lang
		}
	}
	home, _ := os.UserHomeDir()
	for _, name := range userKitMCPs {
		if owned, err := mcpreg.KitOwned(filepath.Join(home, ".claude.json"), workspace, name); err != nil {
			u.Step(ui.StatusWarn, "mcp", fmt.Sprintf("bỏ qua (%v)", err)) //znf:allow-lang
			continue
		} else if !owned {
			continue
		}
		if _, err := mcpreg.RemoveScoped(name, "local", run); err != nil {
			u.Step(ui.StatusWarn, "mcp", fmt.Sprintf("bỏ qua (%v)", err)) //znf:allow-lang
		}
	}
}

// marketplaceNotFound matches `Marketplace '<name>' not found`, scoped so a
// missing claude binary ("command not found") is not swallowed.
func marketplaceNotFound(out string) bool {
	i := strings.Index(out, "Marketplace '")
	return i >= 0 && strings.Contains(out[i:], "' not found")
}
