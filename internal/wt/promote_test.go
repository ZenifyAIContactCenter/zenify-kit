package wt

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// promoteFixture builds <root> (main checkout, with an empty-install config so
// runInstall is a no-op) and <root>/.worktrees/my-task (registered in state),
// and creates a node_modules dir in the main checkout. Returns root and the
// worktree path.
func promoteFixture(t *testing.T) (root, wtPath string) {
	t.Helper()
	root = t.TempDir()
	writeWorktreeJSON(t, root, `{"abbrev":"myrepo","user":"namph","portRange":[3200,3249],"deps":"clone"}`)
	if err := os.MkdirAll(filepath.Join(root, "node_modules"), 0o750); err != nil {
		t.Fatal(err)
	}
	wtPath = filepath.Join(root, ".worktrees", "my-task")
	if err := os.MkdirAll(wtPath, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := SaveWorktree(root, Worktree{Slug: "my-task", Path: wtPath}, 1, "h", 1); err != nil {
		t.Fatal(err)
	}
	return root, wtPath
}

func promoteOpts(root string, g *gitStub) PromoteOptions {
	return PromoteOptions{RepoRoot: root, Slug: "my-task", Runner: g, Stdout: io.Discard, Stderr: io.Discard}
}

func TestRunPromote_SymlinkPromoted(t *testing.T) {
	root, wtPath := promoteFixture(t)
	dst := filepath.Join(wtPath, "node_modules")
	if err := os.Symlink(filepath.Join(root, "node_modules"), dst); err != nil {
		t.Fatal(err)
	}
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}

	if err := RunPromote(promoteOpts(root, g)); err != nil {
		t.Fatalf("RunPromote: %v", err)
	}
	fi, err := os.Lstat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("expected node_modules to be a real directory, still a symlink")
	}
	found := false
	for _, s := range g.seen {
		if s == "config --worktree wt.deps clone" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected config --worktree wt.deps clone to be recorded, seen: %v", g.seen)
	}
}

func TestRunPromote_AlreadyPrivate_NoOp(t *testing.T) {
	root, wtPath := promoteFixture(t)
	dst := filepath.Join(wtPath, "node_modules")
	if err := os.MkdirAll(dst, 0o750); err != nil {
		t.Fatal(err)
	}
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}

	if err := RunPromote(promoteOpts(root, g)); err != nil {
		t.Fatalf("RunPromote: %v", err)
	}
	for _, s := range g.seen {
		if strings.HasPrefix(s, "config") {
			t.Fatalf("Runner must not be called for config when already private, seen: %v", g.seen)
		}
	}
}

func TestRunPromote_NoNodeModulesAtAll(t *testing.T) {
	root, wtPath := promoteFixture(t)
	dst := filepath.Join(wtPath, "node_modules")
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}

	err := RunPromote(promoteOpts(root, g))
	if err == nil || !strings.Contains(err.Error(), "nothing to promote") {
		t.Fatalf("expected 'nothing to promote' error, got %v", err)
	}
	if _, statErr := os.Lstat(dst); statErr == nil {
		t.Fatal("dst must still be absent")
	}
}

func TestRunPromote_UnknownSlug(t *testing.T) {
	root, _ := promoteFixture(t)
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}
	o := promoteOpts(root, g)
	o.Slug = "does-not-exist"

	err := RunPromote(o)
	if err == nil || !strings.Contains(err.Error(), "no worktree") {
		t.Fatalf("expected 'no worktree' error, got %v", err)
	}
}

func TestPromoteNoneIsNoop(t *testing.T) {
	root := t.TempDir()
	writeWorktreeJSON(t, root, `{"abbrev":"myrepo","user":"namph","portRange":[3200,3249],"deps":"none"}`)
	wtPath := filepath.Join(root, ".worktrees", "my-task")
	if err := os.MkdirAll(wtPath, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := SaveWorktree(root, Worktree{Slug: "my-task", Path: wtPath}, 1, "h", 1); err != nil {
		t.Fatal(err)
	}
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}

	if err := RunPromote(promoteOpts(root, g)); err != nil {
		t.Fatalf("RunPromote deps:none → err %v, want nil", err)
	}
	if _, statErr := os.Lstat(filepath.Join(wtPath, "node_modules")); statErr == nil {
		t.Fatal("deps:none must not create/touch node_modules")
	}
	for _, s := range g.seen {
		if strings.HasPrefix(s, "config") {
			t.Fatalf("Runner must not be called for config with deps:none, seen: %v", g.seen)
		}
	}
}

func TestRunPromote_BookkeepingFailure(t *testing.T) {
	root, wtPath := promoteFixture(t)
	dst := filepath.Join(wtPath, "node_modules")
	if err := os.Symlink(filepath.Join(root, "node_modules"), dst); err != nil {
		t.Fatal(err)
	}
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}
	g.err[wtPath+"|config --worktree wt.deps clone"] = errors.New("git config failed")

	err := RunPromote(promoteOpts(root, g))
	if err == nil || !strings.Contains(err.Error(), "bookkeeping") {
		t.Fatalf("expected bookkeeping error, got %v", err)
	}
	fi, statErr := os.Lstat(dst)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		t.Fatal("expected the copy to have landed as a real directory despite the bookkeeping failure")
	}
}
