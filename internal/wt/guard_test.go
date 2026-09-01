package wt

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

var errFake = errors.New("not ancestor")

// fakeRunner returns canned output keyed by "dir|joined-args". Keying on dir too
// is required: RepoOpenTasks asks wt.slug of several worktrees with identical
// args, distinguished only by the dir passed to Run.
type fakeRunner struct {
	out map[string]string
	err map[string]error
}

func (f fakeRunner) Run(dir string, args ...string) ([]byte, error) {
	k := dir + "|" + strings.Join(args, " ")
	if e, ok := f.err[k]; ok {
		return nil, e
	}
	return []byte(f.out[k]), nil
}

func TestSessionPtr_OffWhenNoSession(t *testing.T) {
	t.Setenv("WT_SESSION", "")
	if _, active := SessionPtr(t.TempDir()); active {
		t.Fatal("empty WT_SESSION must disable the guard")
	}
}

func TestSessionPtr_PathIsPerRepoPerSession(t *testing.T) {
	t.Setenv("WT_SESSION", "sess1")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	repo := "/Users/x/WorkingSpace/repo-a"
	p, active := SessionPtr(repo)
	if !active {
		t.Fatal("want active with WT_SESSION set")
	}
	// The repo path is flattened into the filename (slashes → underscores).
	if !strings.Contains(p, "session-sess1") || !strings.Contains(filepath.Base(p), "_Users_x_WorkingSpace_repo-a") {
		t.Fatalf("pointer path wrong: %s", p)
	}
}

func TestBranchMerged_AncestorAndSquash(t *testing.T) {
	// Ancestor: merge-base --is-ancestor exits 0 (nil err) → merged.
	anc := fakeRunner{out: map[string]string{}, err: map[string]error{}}
	if !BranchMerged(anc, "/repo", "b", "base") {
		t.Fatal("ancestor branch should read merged")
	}
	// Not ancestor (err) but empty diff → squash-merged → merged.
	sq := fakeRunner{
		out: map[string]string{"/repo|diff base..b": ""},
		err: map[string]error{"/repo|merge-base --is-ancestor b base": errFake},
	}
	if !BranchMerged(sq, "/repo", "b", "base") {
		t.Fatal("empty diff should read squash-merged")
	}
	// Not ancestor AND non-empty diff → unmerged.
	un := fakeRunner{
		out: map[string]string{"/repo|diff base..b": "diff --git a b"},
		err: map[string]error{"/repo|merge-base --is-ancestor b base": errFake},
	}
	if BranchMerged(un, "/repo", "b", "base") {
		t.Fatal("non-empty diff should read unmerged")
	}
}

func TestRepoOpenTasks_FiltersUserAndMerged(t *testing.T) {
	// worktree list porcelain: main + two worktrees, one merged, one not.
	list := strings.Join([]string{
		"worktree /repo",
		"branch refs/heads/staging",
		"",
		"worktree /repo/.worktrees/open",
		"branch refs/heads/namph/feat/open",
		"",
		"worktree /repo/.worktrees/done",
		"branch refs/heads/namph/feat/done",
		"",
		"worktree /repo/.worktrees/other",
		"branch refs/heads/someone/feat/x",
		"",
	}, "\n")
	r := fakeRunner{
		out: map[string]string{
			"/repo|worktree list --porcelain":            list,
			"/repo/.worktrees/open|config --get wt.slug": "open",
			"/repo/.worktrees/done|config --get wt.slug": "done",
			"/repo|diff origin/staging..namph/feat/open": "diff x", // unmerged
			"/repo|diff origin/staging..namph/feat/done": "",       // merged (squash)
		},
		err: map[string]error{
			"/repo|merge-base --is-ancestor namph/feat/open origin/staging": errFake,
			"/repo|merge-base --is-ancestor namph/feat/done origin/staging": errFake,
		},
	}
	tasks, err := RepoOpenTasks(r, "/repo", "namph", ".worktrees/", "origin/staging")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Slug != "open" {
		t.Fatalf("want only unmerged namph task 'open', got %+v", tasks)
	}
}

func TestRepoOpenTasks_SiblingPrefixDirNotMatched(t *testing.T) {
	// A sibling dir whose NAME starts with the worktree-dir string
	// (".worktrees-backup") must NOT be treated as a managed worktree — the
	// prefix match requires a path-separator boundary.
	list := strings.Join([]string{
		"worktree /repo",
		"branch refs/heads/staging",
		"",
		"worktree /repo/.worktrees-backup/ghost",
		"branch refs/heads/namph/feat/ghost",
		"",
	}, "\n")
	r := fakeRunner{
		out: map[string]string{"/repo|worktree list --porcelain": list},
		err: map[string]error{},
	}
	tasks, err := RepoOpenTasks(r, "/repo", "namph", ".worktrees/", "origin/staging")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Fatalf("sibling-prefix dir must not be matched, got %+v", tasks)
	}
}
