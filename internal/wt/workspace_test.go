package wt

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func mkWs(t *testing.T) string {
	t.Helper()
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".zenify"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".zenify", "manifest.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return ws
}

// mkGitRepo initialises a real repo at ws/repos/<name>; withWT adds worktree.json.
func mkGitRepo(t *testing.T, ws, name string, withWT bool) string {
	t.Helper()
	dir := filepath.Join(ws, "repos", name)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil { //nolint:gosec // G204 -- test fixture
		t.Fatalf("git init: %v %s", err, out)
	}
	if withWT {
		writeWorktreeJSON(t, dir, `{"abbrev":"`+name+`","user":"namph","portRange":[3200,3249],"deps":"none"}`)
	}
	return dir
}

func TestFindWorkspaceRoot_WalksUp(t *testing.T) {
	ws := mkWs(t)
	nested := filepath.Join(ws, "repos", "x", "deep")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatal(err)
	}
	got, ok := FindWorkspaceRoot(nested)
	if !ok || got != ws {
		t.Fatalf("FindWorkspaceRoot = %q,%v want %q", got, ok, ws)
	}
	if _, ok := FindWorkspaceRoot(t.TempDir()); ok {
		t.Fatal("no marker → must report not found")
	}
}

func TestWorkspaceRepos_FiltersToWorktreeJSON(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	ws := mkWs(t)
	a := mkGitRepo(t, ws, "a", true)
	mkGitRepo(t, ws, "b", false) // repo without worktree.json → excluded
	c := mkGitRepo(t, ws, "c", true)
	// a linked-worktree-like dir inside a: must NOT be enumerated as a repo.
	// Excluded by the depth cap (WalkDir never descends past scanDepth to reach
	// it) — not by the ".git" entry being a file rather than a directory, which
	// only matters for a nested worktree shallow enough to escape the cap.
	if err := os.MkdirAll(filepath.Join(a, ".worktrees", "t1", ".claude"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a, ".worktrees", "t1", ".git"), []byte("gitdir: x"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeWorktreeJSON(t, filepath.Join(a, ".worktrees", "t1"), `{"abbrev":"t1"}`)
	got := WorkspaceRepos(ws)
	if len(got) != 2 || got[0] != a || got[1] != c {
		t.Fatalf("WorkspaceRepos = %v, want [%s %s]", got, a, c)
	}
}
