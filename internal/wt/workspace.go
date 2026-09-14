package wt

import (
	"os"
	"path/filepath"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitstate"
)

// FindWorkspaceRoot walks up from start to the directory holding
// .zenify/manifest.json. Lives here (not in internal/cli) because wt needs it
// for cross-repo work and cli already imports wt.
func FindWorkspaceRoot(start string) (string, bool) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	for {
		marker := filepath.Join(dir, ".zenify", "manifest.json")
		if fi, err := os.Stat(marker); err == nil && !fi.IsDir() {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// WorkspaceRepos lists the repos under root that wt manages: every git repo
// gitstate.Scope finds (depth ≤3, node_modules/Library skipped, $HOME refused,
// linked worktrees excluded by the depth cap) that declares
// .claude/worktree.json. Sorted, like Scope.
func WorkspaceRepos(root string) []string {
	var out []string
	for _, repo := range gitstate.Scope(root) {
		if fi, err := os.Stat(filepath.Join(repo, ".claude", "worktree.json")); err == nil && !fi.IsDir() {
			out = append(out, repo)
		}
	}
	return out
}
