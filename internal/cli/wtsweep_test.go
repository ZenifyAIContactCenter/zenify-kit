package cli

import (
	"strings"
	"testing"
)

func TestWtSweep_DryRunIntegration(t *testing.T) {
	requireGit(t)
	t.Setenv("WT_SESSION", "")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := initGitRepo(t)
	t.Setenv("WT_REPO_ROOT", root)
	if _, err := runWt(t, root, "new", "sweepable", "--type", "feat"); err != nil {
		t.Fatalf("precondition wt new: %v", err)
	}
	// initGitRepo's worktree.json sets baseRef=main (local), and a freshly-created
	// branch sits at main's tip → BranchMerged true → clean → dry-run must list it
	// as would-remove, and remove NOTHING.
	out, err := runWt(t, root, "sweep", "--dry-run")
	if err != nil {
		t.Fatalf("sweep --dry-run: %v", err)
	}
	if !strings.Contains(out, "sweepable") || !strings.Contains(out, "would") {
		t.Fatalf("dry-run should list sweepable as would-remove: %q", out)
	}
	// worktree still present after a dry-run
	ls, _ := runWt(t, root, "ls")
	if !strings.Contains(ls, "sweepable") {
		t.Fatalf("dry-run must not remove the worktree: %q", ls)
	}
}
