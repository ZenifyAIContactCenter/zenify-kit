package gitx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureExclude_AppendsMissingLinesOnce(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	p, err := EnsureExclude(repo, ".worktrees/", ".wt/")
	if err != nil {
		t.Fatal(err)
	}
	if p != filepath.Join(repo, ".git", "info", "exclude") {
		t.Fatalf("path = %q", p)
	}
	b, _ := os.ReadFile(p)
	if got := string(b); got != ".worktrees/\n.wt/\n" {
		t.Fatalf("content = %q", got)
	}
	// second call: nothing to add → "" and file unchanged
	p2, err := EnsureExclude(repo, ".worktrees/", ".wt/")
	if err != nil || p2 != "" {
		t.Fatalf("second call = %q, %v", p2, err)
	}
	// partial: only .wt/ missing, existing content without trailing newline
	if err := os.WriteFile(p, []byte("node_modules\n.worktrees/"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureExclude(repo, ".worktrees/", ".wt/"); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(p)
	if got := string(b); got != "node_modules\n.worktrees/\n.wt/\n" || strings.Count(got, ".worktrees/") != 1 {
		t.Fatalf("partial content = %q", got)
	}
}
