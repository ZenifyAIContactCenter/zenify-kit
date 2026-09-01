package wt

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rmStub keys on "dir|args"; records worktree/branch removals for assertions.
type rmStub struct {
	out  map[string]string
	err  map[string]error
	seen []string
}

func (s *rmStub) Run(dir string, args ...string) ([]byte, error) {
	k := dir + "|" + strings.Join(args, " ")
	s.seen = append(s.seen, k)
	if e, ok := s.err[k]; ok {
		return nil, e
	}
	return []byte(s.out[k]), nil
}

func (s *rmStub) ran(k string) bool {
	for _, x := range s.seen {
		if x == k {
			return true
		}
	}
	return false
}

// rmRepo makes a real dir tree so os.Stat(path) behaves; state/config come from
// a written worktree.json + a seeded state.json.
func rmRepo(t *testing.T, slug, branch string) (root string) {
	t.Helper()
	root = t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `{"abbrev":"tst","baseRef":"origin/main","worktreeDir":".worktrees/","portEnv":"PORT","portRange":[3200,3249],"user":"namph"}`
	if err := os.WriteFile(filepath.Join(root, ".claude", "worktree.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	wtPath := filepath.Join(root, ".worktrees", slug)
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SaveWorktree(root, Worktree{Slug: slug, Type: "feat", Branch: branch, Path: wtPath, Ports: []int{3207}}, 111, "h", 1); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestRunRm_RefusesDirtyTree(t *testing.T) {
	root := rmRepo(t, "foo", "namph/feat/foo")
	wtPath := filepath.Join(root, ".worktrees", "foo")
	s := &rmStub{
		out: map[string]string{
			wtPath + "|symbolic-ref --quiet --short HEAD": "namph/feat/foo",
			wtPath + "|status --porcelain":                " M file.go", // dirty
		},
	}
	err := RunRm(RmOptions{RepoRoot: root, Slug: "foo", Runner: s, Stderr: io.Discard, Pid: 1, Now: 2, Host: "h"})
	if err == nil {
		t.Fatal("dirty tree must be refused")
	}
	if s.ran(root + "|worktree remove --force " + wtPath) {
		t.Fatal("must NOT remove a dirty worktree")
	}
}

func TestRunRm_RefusesUnmerged(t *testing.T) {
	root := rmRepo(t, "foo", "namph/feat/foo")
	wtPath := filepath.Join(root, ".worktrees", "foo")
	s := &rmStub{
		out: map[string]string{
			wtPath + "|symbolic-ref --quiet --short HEAD": "namph/feat/foo",
			wtPath + "|status --porcelain":                "",  // clean
			root + "|diff origin/main..namph/feat/foo":    "d", // non-empty diff
		},
		err: map[string]error{root + "|merge-base --is-ancestor namph/feat/foo origin/main": errors.New("no")},
	}
	if err := RunRm(RmOptions{RepoRoot: root, Slug: "foo", Runner: s, Stderr: io.Discard, Pid: 1, Now: 2, Host: "h"}); err == nil {
		t.Fatal("unmerged branch must be refused")
	}
}

func TestRunRm_RemovesMergedCleanAndCleansCaches(t *testing.T) {
	root := rmRepo(t, "foo", "namph/feat/foo")
	wtPath := filepath.Join(root, ".worktrees", "foo")
	s := &rmStub{
		out: map[string]string{
			wtPath + "|symbolic-ref --quiet --short HEAD": "namph/feat/foo",
			wtPath + "|status --porcelain":                "",
			root + "|diff origin/main..namph/feat/foo":    "", // empty diff → merged
		},
		err: map[string]error{root + "|merge-base --is-ancestor namph/feat/foo origin/main": errors.New("not ancestor")},
	}
	if err := RunRm(RmOptions{RepoRoot: root, Slug: "foo", Runner: s, Stderr: io.Discard, Pid: 1, Now: 2, Host: "h"}); err != nil {
		t.Fatalf("merged+clean should remove: %v", err)
	}
	if !s.ran(root + "|worktree remove --force " + wtPath) {
		t.Fatal("expected git worktree remove --force")
	}
	if !s.ran(root + "|branch -D namph/feat/foo") {
		t.Fatal("expected branch -D")
	}
	st, _ := ReadState(root)
	for _, w := range st.Worktrees {
		if w.Slug == "foo" {
			t.Fatal("state.json must no longer list foo")
		}
	}
}

func TestRunRm_ForceSkipsGate(t *testing.T) {
	root := rmRepo(t, "foo", "namph/feat/foo")
	wtPath := filepath.Join(root, ".worktrees", "foo")
	s := &rmStub{
		out: map[string]string{
			wtPath + "|symbolic-ref --quiet --short HEAD": "namph/feat/foo",
			wtPath + "|status --porcelain":                " M dirty.go", // dirty but forced
		},
	}
	if err := RunRm(RmOptions{RepoRoot: root, Slug: "foo", Force: true, Runner: s, Stderr: io.Discard, Pid: 1, Now: 2, Host: "h"}); err != nil {
		t.Fatalf("--force must bypass the gate: %v", err)
	}
	if !s.ran(root + "|worktree remove --force " + wtPath) {
		t.Fatal("force should still remove")
	}
}

func TestRunRm_GoneNeedsForce(t *testing.T) {
	root := rmRepo(t, "foo", "namph/feat/foo")
	// delete the checkout dir → gone
	if err := os.RemoveAll(filepath.Join(root, ".worktrees", "foo")); err != nil {
		t.Fatal(err)
	}
	s := &rmStub{out: map[string]string{}}
	if err := RunRm(RmOptions{RepoRoot: root, Slug: "foo", Runner: s, Stderr: io.Discard, Pid: 1, Now: 2, Host: "h"}); err == nil {
		t.Fatal("a gone worktree without --force must error")
	}
	// with --force → prune + branch from state + cache cleanup
	s2 := &rmStub{out: map[string]string{}}
	if err := RunRm(RmOptions{RepoRoot: root, Slug: "foo", Force: true, Runner: s2, Stderr: io.Discard, Pid: 1, Now: 2, Host: "h"}); err != nil {
		t.Fatalf("gone+force should clean up: %v", err)
	}
	if !s2.ran(root + "|worktree prune") {
		t.Fatal("gone+force should prune")
	}
	if !s2.ran(root + "|branch -D namph/feat/foo") {
		t.Fatal("gone+force should delete the branch named in state")
	}
}
