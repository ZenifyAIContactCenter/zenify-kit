package wt

import (
	"bytes"
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
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o750); err != nil {
		t.Fatal(err)
	}
	cfg := `{"abbrev":"tst","baseRef":"origin/main","worktreeDir":".worktrees/","portEnv":"PORT","portRange":[3200,3249],"user":"namph"}`
	if err := os.WriteFile(filepath.Join(root, ".claude", "worktree.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	wtPath := filepath.Join(root, ".worktrees", slug)
	if err := os.MkdirAll(wtPath, 0o750); err != nil {
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

func TestRunRm_GoneUnregisteredErrorsEvenWithForce(t *testing.T) {
	root := rmRepo(t, "real", "namph/feat/real") // only "real" is registered
	// "ghost" was never a worktree: no dir, no state entry, not in git's list.
	s := &rmStub{out: map[string]string{root + "|worktree list --porcelain": "worktree " + root + "\n"}}
	if err := RunRm(RmOptions{RepoRoot: root, Slug: "ghost", Force: true, Runner: s, Stderr: io.Discard, Pid: 1, Now: 2, Host: "h"}); err == nil {
		t.Fatal("a never-registered slug must error even with --force, not prune the repo")
	}
	if s.ran(root + "|worktree prune") {
		t.Fatal("must NOT run repo-wide worktree prune for a never-registered slug")
	}
}

// A worktree dir that git no longer registers (its .git file points at a gitdir
// that moved, e.g. after `zenify migrate`) makes `git worktree remove` fail.
// --force must then delete the directory itself, prune, and still delete the
// branch recorded in state.json.
func TestRunRm_OrphanDirDeletedWhenGitDoesNotRegisterIt(t *testing.T) {
	root := rmRepo(t, "foo", "namph/feat/foo")
	wtPath := filepath.Join(root, ".worktrees", "foo")
	s := &rmStub{
		out: map[string]string{
			root + "|worktree list --porcelain": "worktree " + root + "\n\n",
		},
		err: map[string]error{
			wtPath + "|symbolic-ref --quiet --short HEAD": errors.New("fatal: not a git repository"),
			root + "|worktree remove --force " + wtPath:   errors.New("exit status 128"),
		},
	}
	var stderr bytes.Buffer
	if err := RunRm(RmOptions{RepoRoot: root, Slug: "foo", Force: true, Runner: s, Stderr: &stderr, Pid: 1, Now: 2, Host: "h"}); err != nil {
		t.Fatalf("orphan dir must be removed directly, got: %v", err)
	}
	if _, e := os.Stat(wtPath); !os.IsNotExist(e) {
		t.Fatalf("orphan dir must be deleted, stat err=%v", e)
	}
	if !s.ran(root+"|worktree prune") || !s.ran(root+"|branch -D namph/feat/foo") {
		t.Fatalf("expected prune + branch -D from state.json, seen: %v", s.seen)
	}
	if !strings.Contains(stderr.String(), "not registered in git") {
		t.Fatalf("expected an explicit note on stderr, got %q", stderr.String())
	}
	st, _ := ReadState(root)
	if _, ok := st.Find("foo"); ok {
		t.Fatal("state entry must be dropped")
	}
}

// A dir git still registers is never deleted behind git's back: the remove
// failure stays a hard error.
func TestRunRm_RegisteredDirRemoveFailureIsError(t *testing.T) {
	root := rmRepo(t, "foo", "namph/feat/foo")
	wtPath := filepath.Join(root, ".worktrees", "foo")
	s := &rmStub{
		out: map[string]string{
			wtPath + "|symbolic-ref --quiet --short HEAD": "namph/feat/foo",
			root + "|worktree list --porcelain":           "worktree " + root + "\nworktree " + wtPath + "\n\n",
		},
		err: map[string]error{root + "|worktree remove --force " + wtPath: errors.New("locked")},
	}
	if err := RunRm(RmOptions{RepoRoot: root, Slug: "foo", Force: true, Runner: s, Stderr: io.Discard, Pid: 1, Now: 2, Host: "h"}); err == nil {
		t.Fatal("registered worktree whose removal fails must be an error")
	}
	if _, e := os.Stat(wtPath); e != nil {
		t.Fatal("registered dir must not be deleted behind git's back")
	}
}
