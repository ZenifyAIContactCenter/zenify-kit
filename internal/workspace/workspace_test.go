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
	// linked worktree: .git is a FILE, not a repo
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
		t.Errorf("missing flat-repo: %+v", got)
	}
	if _, ok := names["nested-repo"]; !ok {
		t.Errorf("missing nested-repo (depth 2): %+v", got)
	}
	if _, ok := names["wt1"]; ok {
		t.Errorf("linked worktree must NOT count as a repo: %+v", got)
	}
	if len(got) != 2 {
		t.Errorf("want exactly 2 repos, got %d: %+v", len(got), got)
	}
}

func TestResolvePrefersShallowest(t *testing.T) {
	root := t.TempDir()
	writeRepo(t, filepath.Join(root, "dup"))          // depth 1
	writeRepo(t, filepath.Join(root, "repos", "dup")) // depth 2
	path, ok := Resolve(root, "dup", DefaultMaxDepth, os.ReadDir)
	if !ok {
		t.Fatal("must resolve dup")
	}
	if path != filepath.Join(root, "dup") {
		t.Errorf("must return the shallowest one (depth 1), got %s", path)
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
		t.Errorf("missing kit-repo (only has .claude/worktree.json, no .git dir): %+v", got)
	}
}
