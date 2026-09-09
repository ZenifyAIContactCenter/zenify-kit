package cli

import (
	"os"
	"path/filepath"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/workspace"
)

// resolveWorkspaceRepoDir returns <repo-path>/sub, finding the repo by name via
// workspace.Discover — so it works for BOTH the flat layout (<ws>/<repo>) and the
// team-standard layout after `zenify migrate` (<ws>/repos/<repo>). Not found → falls
// back to <ws>/<repo>/sub (kept for backward compatibility; the caller already
// fail-opens on a nonexistent path). readDir is injected for pure testing.
func resolveWorkspaceRepoDir(workspaceDir, repo, sub string, readDir func(string) ([]os.DirEntry, error)) string {
	base := filepath.Join(workspaceDir, repo)
	if p, ok := workspace.Resolve(workspaceDir, repo, workspace.DefaultMaxDepth, readDir); ok {
		base = p
	}
	return filepath.Join(base, sub)
}
