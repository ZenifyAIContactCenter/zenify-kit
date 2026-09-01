package wt

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func readStateRaw(t *testing.T, repoRoot string) *StateFile {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot, ".wt", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s StateFile
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	return &s
}

func TestSaveWorktree_CreatesAndUpserts(t *testing.T) {
	root := t.TempDir()
	w := Worktree{Slug: "foo", Type: "feat", Branch: "namph/feat/foo", Path: ".worktrees/foo", CreatedAt: "2026-09-01T00:00:00Z", Ports: []int{3207}}
	if err := SaveWorktree(root, w, 111, "h", 1); err != nil {
		t.Fatal(err)
	}
	got := readStateRaw(t, root)
	if got.Version != 1 || len(got.Worktrees) != 1 || got.Worktrees[0].Slug != "foo" || got.Worktrees[0].Ports[0] != 3207 {
		t.Fatalf("first save wrong: %+v", got)
	}
	// Upsert same slug: replace, not duplicate.
	w.Ports = []int{3208}
	if err := SaveWorktree(root, w, 111, "h", 2); err != nil {
		t.Fatal(err)
	}
	got = readStateRaw(t, root)
	if len(got.Worktrees) != 1 || got.Worktrees[0].Ports[0] != 3208 {
		t.Fatalf("upsert duplicated or did not replace: %+v", got)
	}
}

func TestRemoveWorktree(t *testing.T) {
	root := t.TempDir()
	_ = SaveWorktree(root, Worktree{Slug: "a", Path: ".worktrees/a"}, 1, "h", 1)
	_ = SaveWorktree(root, Worktree{Slug: "b", Path: ".worktrees/b"}, 1, "h", 2)
	removed, err := RemoveWorktree(root, "a", 1, "h", 3)
	if err != nil || !removed {
		t.Fatalf("remove a: removed=%v err=%v", removed, err)
	}
	got := readStateRaw(t, root)
	if len(got.Worktrees) != 1 || got.Worktrees[0].Slug != "b" {
		t.Fatalf("remove wrong survivor: %+v", got)
	}
	removed, _ = RemoveWorktree(root, "nope", 1, "h", 4)
	if removed {
		t.Fatal("removing absent slug reported removed")
	}
}

func TestWriteStateAtomic_NoTempLeftBehind(t *testing.T) {
	root := t.TempDir()
	if err := SaveWorktree(root, Worktree{Slug: "x", Path: ".worktrees/x"}, 1, "h", 1); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(root, ".wt"))
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" || len(e.Name()) > 10 && e.Name()[:6] == "state." && e.Name() != "state.json" {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}
