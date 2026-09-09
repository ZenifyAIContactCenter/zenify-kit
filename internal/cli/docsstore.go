package cli

import (
	"os"
	"path/filepath"
)

// resolveDocsStore returns the path to the REAL store repo (containing .git/.config/.claude).
// Resolve order (stops at the first one that EXISTS): $ZENIFY_HOME/knowledge →
// ~/.zenify/knowledge → fallback workspace docs (machine not migrated yet). If none
// exists → return the STANDARD path (env or ~/.zenify/knowledge) for onboarding
// to clone into. All I/O is injected to keep tests pure.
func resolveDocsStore(workspaceDir string, getenv func(string) string, userHome func() (string, error), stat func(string) (os.FileInfo, error), readDir func(string) ([]os.DirEntry, error)) string {
	home, _ := userHome()
	target := filepath.Join(home, ".zenify", "knowledge")
	if zh := getenv("ZENIFY_HOME"); zh != "" {
		target = filepath.Join(zh, "knowledge")
	}
	if isDirStat(stat, target) {
		return target
	}
	ws := resolveWorkspaceRepoDir(workspaceDir, defaultDocsRepo, "", readDir)
	if isDirStat(stat, ws) {
		return ws
	}
	return target
}

func isDirStat(stat func(string) (os.FileInfo, error), p string) bool {
	fi, err := stat(p)
	return err == nil && fi.IsDir()
}
