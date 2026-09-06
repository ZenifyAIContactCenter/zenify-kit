package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// docs sync resolve dir repo qua discovery (repos/<repo> hoặc phẳng).
func TestDocsDirResolves(t *testing.T) {
	ws := t.TempDir()
	repo := filepath.Join(ws, "zenify-knowledge")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := resolveWorkspaceRepoDir(ws, defaultDocsRepo, "", os.ReadDir)
	if filepath.Clean(got) != filepath.Clean(repo) {
		t.Fatalf("resolve docs dir: got %q want %q", got, repo)
	}
}
