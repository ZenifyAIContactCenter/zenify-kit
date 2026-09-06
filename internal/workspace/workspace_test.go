package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func writeRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverFlatAndNested(t *testing.T) {
	root := t.TempDir()
	writeRepo(t, filepath.Join(root, "flat-repo"))            // depth 1
	writeRepo(t, filepath.Join(root, "repos", "nested-repo")) // depth 2
	// linked worktree: .git là FILE, không phải repo
	wt := filepath.Join(root, "flat-repo", ".worktrees", "wt1")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := Discover(root, DefaultMaxDepth, os.ReadDir)
	names := map[string]string{}
	for _, r := range got {
		names[r.Name] = r.Path
	}
	if _, ok := names["flat-repo"]; !ok {
		t.Errorf("thiếu flat-repo: %+v", got)
	}
	if _, ok := names["nested-repo"]; !ok {
		t.Errorf("thiếu nested-repo (depth 2): %+v", got)
	}
	if _, ok := names["wt1"]; ok {
		t.Errorf("linked worktree KHÔNG được coi là repo: %+v", got)
	}
	if len(got) != 2 {
		t.Errorf("muốn đúng 2 repo, được %d: %+v", len(got), got)
	}
}

func TestResolvePrefersShallowest(t *testing.T) {
	root := t.TempDir()
	writeRepo(t, filepath.Join(root, "dup"))          // depth 1
	writeRepo(t, filepath.Join(root, "repos", "dup")) // depth 2
	path, ok := Resolve(root, "dup", DefaultMaxDepth, os.ReadDir)
	if !ok {
		t.Fatal("phải resolve được dup")
	}
	if path != filepath.Join(root, "dup") {
		t.Errorf("phải trả cái nông nhất (depth 1), được %s", path)
	}
}

func TestDiscoverRecognisesWorktreeJSONWithoutGitDir(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "kit-repo")
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".claude", "worktree.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := Discover(root, DefaultMaxDepth, os.ReadDir)
	names := map[string]string{}
	for _, r := range got {
		names[r.Name] = r.Path
	}
	if _, ok := names["kit-repo"]; !ok {
		t.Errorf("thiếu kit-repo (chỉ có .claude/worktree.json, không có .git dir): %+v", got)
	}
}
