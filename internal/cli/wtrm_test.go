package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// gitInWorktree runs a git command inside a linked worktree with a fixed author,
// so an integration test can push a branch ahead of base deterministically.
func gitInWorktree(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// Reuses initGitRepo/runWt/requireGit from the cli package test harness.
func TestWtRm_Integration(t *testing.T) {
	requireGit(t)
	t.Setenv("WT_SESSION", "")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := initGitRepo(t)
	t.Setenv("WT_REPO_ROOT", root)
	if _, err := runWt(t, root, "new", "gone-task", "--type", "feat"); err != nil {
		t.Fatalf("precondition wt new: %v", err)
	}
	wtPath := filepath.Join(root, ".worktrees", "gone-task")
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("worktree should exist: %v", err)
	}
	// `wt new` bases the branch at base's tip, so it is trivially an ancestor of
	// base = "merged". Push it genuinely ahead with a REAL file change (an empty
	// commit leaves `diff base..branch` empty, which BranchMerged reads as merged)
	// so the "unmerged → refused" assertion is deterministic.
	if err := os.WriteFile(filepath.Join(wtPath, "work.txt"), []byte("wip"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitInWorktree(t, wtPath, "add", "work.txt")
	gitInWorktree(t, wtPath, "commit", "-q", "-m", "wip")

	// Now genuinely unmerged → rm without --force must fail.
	if _, err := runWt(t, root, "rm", "gone-task"); err == nil {
		t.Fatal("unmerged task must not be removable without --force")
	}
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatal("refused rm must leave the worktree in place")
	}
	// --force removes it.
	if _, err := runWt(t, root, "rm", "gone-task", "--force"); err != nil {
		t.Fatalf("--force rm should succeed: %v", err)
	}
	if _, err := os.Stat(wtPath); err == nil {
		t.Fatal("worktree dir should be gone after rm --force")
	}
}
