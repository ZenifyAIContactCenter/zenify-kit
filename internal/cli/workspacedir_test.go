package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// mkGitRepoDir creates <parent>/<name>/.git (a directory) so workspace.Discover treats it as a repo.
func mkGitRepoDir(t *testing.T, parent, name string) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// Regression after `zenify migrate`: repo lives at <ws>/repos/<repo> → the default must
// point there, NOT the flat path <ws>/<repo> (a bug that broke config/release-report defaults).
func TestResolveWorkspaceRepoDir_ReposLayout(t *testing.T) {
	ws := t.TempDir()
	repoDir := mkGitRepoDir(t, filepath.Join(ws, "repos"), "zenify-knowledge")

	got := resolveWorkspaceRepoDir(ws, "zenify-knowledge", "config", os.ReadDir)
	want := filepath.Join(repoDir, "config")
	if got != want {
		t.Fatalf("layout repos/: got %q, want %q", got, want)
	}
}

// The old flat layout (<ws>/<repo>) must still work (backward compatibility).
func TestResolveWorkspaceRepoDir_FlatLayout(t *testing.T) {
	ws := t.TempDir()
	repoDir := mkGitRepoDir(t, ws, "zenify-knowledge")

	got := resolveWorkspaceRepoDir(ws, "zenify-knowledge", "releases", os.ReadDir)
	want := filepath.Join(repoDir, "releases")
	if got != want {
		t.Fatalf("flat layout: got %q, want %q", got, want)
	}
}

// Repo not found → falls back to the flat path <ws>/<repo>/sub (fail-open: the caller
// itself handles a nonexistent path). Must NOT panic, must NOT return empty.
func TestResolveWorkspaceRepoDir_FallbackWhenAbsent(t *testing.T) {
	ws := t.TempDir()

	got := resolveWorkspaceRepoDir(ws, "zenify-knowledge", "config", os.ReadDir)
	want := filepath.Join(ws, "zenify-knowledge", "config")
	if got != want {
		t.Fatalf("fallback: got %q, want %q", got, want)
	}
}
