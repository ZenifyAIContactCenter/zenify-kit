package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
)

const agentsMdMaxBytes = 32 * 1024

// codexHooksCheck reports how many of the kit's Codex hooks are wired. Codex
// hook trust is not readable from disk, so the trust line is fixed text.
func codexHooksCheck(codexHome string, onPath bool) Check {
	return Check{
		Name: "codex-hooks",
		Run: func() (bool, string) {
			if !onPath {
				return true, "codex không cài" //znf:allow-lang
			}
			want := len(apply.CodexHookSpecs()) + 1 // + the git-guard entry EnsureCodexHooks adds
			n, err := apply.CountCodexKitHooks(codexHome)
			if err != nil {
				return false, fmt.Sprintf("hooks.json không đọc được: %v", err) //znf:allow-lang
			}
			detail := fmt.Sprintf("wired %d/%d", n, want)
			if n < want {
				detail = "cảnh báo: " + detail //znf:allow-lang
			}
			return true, detail + "\ntrust: không kiểm tra được, xem /hooks" //znf:allow-lang
		},
	}
}

// reviewerCheck reports the env override and the engine `zenify reviewer` picks.
func reviewerCheck(getenv func(string) string, onPath bool) Check {
	return Check{
		Name: "reviewer",
		Run: func() (bool, string) {
			engine, reason := decideReviewer(getenv, onPath)
			var sb strings.Builder
			printReviewer(&sb, engine, reason)
			return true, fmt.Sprintf("%s=%s; engine: %s", apply.ReviewerEnv, getenv(apply.ReviewerEnv), strings.TrimSpace(sb.String()))
		},
	}
}

// agentsMdCheck reports AGENTS.md presence/size per repo (plus repos/zenify when
// it exists). repos are paths relative to workspace.
func agentsMdCheck(workspace string, repos []string) Check {
	return Check{
		Name: "agents-md",
		Run: func() (bool, string) {
			if workspace == "" {
				return false, "không tìm thấy workspace" //znf:allow-lang
			}
			list := append([]string(nil), repos...)
			have := false
			for _, r := range list {
				if r == filepath.Join("repos", "zenify") {
					have = true
				}
			}
			if z := filepath.Join("repos", "zenify"); !have {
				if fi, err := os.Stat(filepath.Join(workspace, z)); err == nil && fi.IsDir() {
					list = append(list, z)
				}
			}
			warn := false
			var lines []string
			for _, r := range list {
				state := "có" //znf:allow-lang
				fi, err := os.Stat(filepath.Join(workspace, r, "AGENTS.md"))
				switch {
				case err != nil:
					state, warn = "thiếu", true //znf:allow-lang
				case fi.Size() > agentsMdMaxBytes:
					state, warn = "vượt 32 KiB", true //znf:allow-lang
				}
				lines = append(lines, fmt.Sprintf("%s/AGENTS.md: %s", filepath.ToSlash(r), state))
			}
			detail := strings.Join(lines, "\n")
			if warn {
				detail = "cảnh báo:\n" + detail //znf:allow-lang
			}
			return true, detail
		},
	}
}

// registerCodexChecks wires the three Codex-runtime checks; workspace, manifest
// and codex home resolve lazily at Run time.
func registerCodexChecks() {
	RegisterCheck(Check{Name: "codex-hooks", Run: func() (bool, string) {
		cwd, _ := os.Getwd()
		ws, _, ok := resolveWorkspace(cwd, "", os.Getenv, os.UserHomeDir)
		if !ok {
			return false, "no workspace found — run inside a zenify workspace"
		}
		return codexHooksCheck(filepath.Join(ws, ".codex"), codexOnPath()).Run()
	}})
	RegisterCheck(Check{Name: "reviewer", Run: func() (bool, string) {
		return reviewerCheck(os.Getenv, codexOnPath()).Run()
	}})
	RegisterCheck(Check{Name: "agents-md", Run: func() (bool, string) {
		cwd, _ := os.Getwd()
		ws, _, ok := resolveWorkspace(cwd, "", os.Getenv, os.UserHomeDir)
		if !ok {
			return agentsMdCheck("", nil).Run()
		}
		repos, err := manifestRepoPaths(ws)
		if err != nil {
			return false, fmt.Sprintf("manifest không đọc được: %v", err) //znf:allow-lang
		}
		return agentsMdCheck(ws, repos).Run()
	}})
}
