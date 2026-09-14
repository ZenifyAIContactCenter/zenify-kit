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

// gitStub records commands and returns canned results keyed by "dir|args".
type gitStub struct {
	out  map[string]string
	err  map[string]error
	seen []string
}

func (g *gitStub) Run(dir string, args ...string) ([]byte, error) {
	k := dir + "|" + strings.Join(args, " ")
	g.seen = append(g.seen, strings.Join(args, " "))
	if e, ok := g.err[k]; ok {
		return nil, e
	}
	return []byte(g.out[k]), nil
}

func baseOpts(root string, g *gitStub) NewOptions {
	return NewOptions{RepoRoot: root, Slug: "my-task", Type: "feat", Host: "h", Pid: 1, Now: 1, Runner: g, Stderr: io.Discard}
}

// seedCfg writes a minimal worktree.json so Config.Load succeeds.
func seedCfg(t *testing.T, root string) {
	t.Helper()
	writeWorktreeJSON(t, root, `{"abbrev":"ccbe","user":"namph","portRange":[3200,3249],"deps":"install"}`)
}

func TestRunNew_RejectsBadSlug(t *testing.T) {
	t.Setenv("WT_SESSION", "")
	root := t.TempDir()
	seedCfg(t, root)
	o := baseOpts(root, &gitStub{out: map[string]string{}, err: map[string]error{}})
	o.Slug = "Bad_Slug"
	if err := RunNew(o); err == nil {
		t.Fatal("uppercase/underscore slug must be rejected")
	}
}

func TestRunNew_RejectsBadType(t *testing.T) {
	t.Setenv("WT_SESSION", "")
	root := t.TempDir()
	seedCfg(t, root)
	o := baseOpts(root, &gitStub{out: map[string]string{}, err: map[string]error{}})
	o.Type = "wip"
	if err := RunNew(o); err == nil {
		t.Fatal("type outside feat|fix|chore|hotfix must be rejected")
	}
}

func TestRunNew_RejectsExistingBranch(t *testing.T) {
	t.Setenv("WT_SESSION", "")
	root := t.TempDir()
	seedCfg(t, root)
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}
	// show-ref --verify --quiet refs/heads/<branch> exits 0 → branch exists.
	g.out[root+"|show-ref --verify --quiet refs/heads/namph/feat/my-task"] = ""
	o := baseOpts(root, g)
	if err := RunNew(o); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing branch must be rejected, got %v", err)
	}
}

func TestRunNew_RejectsMissingBase(t *testing.T) {
	t.Setenv("WT_SESSION", "")
	root := t.TempDir()
	seedCfg(t, root)
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}
	// branch does NOT exist (show-ref errors); base rev-parse errors → not found.
	g.err[root+"|show-ref --verify --quiet refs/heads/namph/feat/my-task"] = errors.New("no ref")
	g.err[root+"|rev-parse --verify --quiet origin/main"] = errors.New("bad rev")
	o := baseOpts(root, g)
	o.BaseOverride = "origin/main"
	if err := RunNew(o); err == nil || !strings.Contains(err.Error(), "base") {
		t.Fatalf("missing base ref must be rejected, got %v", err)
	}
}

func TestRunNew_GuardTier2BlocksSecondUnmergedTask(t *testing.T) {
	t.Setenv("WT_SESSION", "sess-x")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	root := t.TempDir()
	seedCfg(t, root)
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}
	g.err[root+"|show-ref --verify --quiet refs/heads/namph/feat/my-task"] = errors.New("no ref")
	// RepoOpenTasks: one existing unmerged namph worktree → tier-2 blocks.
	list := strings.Join([]string{
		"worktree " + root,
		"branch refs/heads/staging",
		"",
		"worktree " + root + "/.worktrees/existing",
		"branch refs/heads/namph/feat/existing",
		"",
	}, "\n")
	g.out[root+"|worktree list --porcelain"] = list
	g.out[root+"/.worktrees/existing|config --get wt.slug"] = "existing"
	g.err[root+"|merge-base --is-ancestor namph/feat/existing origin/main"] = errors.New("not ancestor")
	g.out[root+"|diff origin/main..namph/feat/existing"] = "diff y" // unmerged
	o := baseOpts(root, g)
	if err := RunNew(o); err == nil || !strings.Contains(err.Error(), "--another") {
		t.Fatalf("tier-2 guard must block a second unmerged task, got %v", err)
	}
}

// FR-3.1: after a successful fetch, wt new sweeps merged+clean worktrees in
// this repo before creating the new one. Uses a real dir for the merged task
// so the sweep's RunRm path runs; the stub answers the git calls.
func TestRunNew_AutoSweepsMergedTasksAfterFetch(t *testing.T) {
	t.Setenv("WT_SESSION", "")
	root := t.TempDir()
	seedCfg(t, root)
	done := filepath.Join(root, ".worktrees", "done")
	if err := os.MkdirAll(done, 0o750); err != nil {
		t.Fatal(err)
	}
	g := &gitStub{out: map[string]string{
		root + "|worktree list --porcelain":         "worktree " + root + "\n\nworktree " + done + "\n\n",
		done + "|config --get wt.slug":              "done",
		done + "|symbolic-ref --quiet --short HEAD": "namph/feat/done",
		done + "|config --get wt.port":              "3207",
		done + "|status --porcelain":                "",
		root + "|diff origin/main..namph/feat/done": "",
	}, err: map[string]error{
		root + "|merge-base --is-ancestor namph/feat/done origin/main": errors.New("x"),
	}}
	var errb bytes.Buffer
	o := baseOpts(root, g)
	o.Stderr = &errb
	// The stub's default show-ref answer (nil error) reads as "branch already
	// exists", so RunNew stops at the duplicate-task check right after the
	// sweep — it never reaches `worktree add`. That's fine: this test only
	// needs to prove the sweep ran first.
	_ = RunNew(o)
	if !seenContains(g, "worktree remove --force "+done) {
		t.Fatalf("merged task must be swept before creating the new one; seen %v", g.seen)
	}
	if !strings.Contains(errb.String(), "wt: swept 1, left 0") {
		t.Fatalf("expected sweep summary on stderr, got %q", errb.String())
	}
}

// FR-3.1/SC-5: a failed fetch skips the sweep entirely.
func TestRunNew_FetchFailureSkipsAutoSweep(t *testing.T) {
	t.Setenv("WT_SESSION", "")
	root := t.TempDir()
	seedCfg(t, root)
	g := &gitStub{out: map[string]string{}, err: map[string]error{root + "|fetch origin --quiet": errors.New("offline")}}
	var errb bytes.Buffer
	o := baseOpts(root, g)
	o.Stderr = &errb
	_ = RunNew(o)
	if seenContains(g, "worktree list --porcelain") {
		t.Fatal("sweep must not run when fetch failed")
	}
	if !strings.Contains(errb.String(), "fetch failed") {
		t.Fatalf("expected fetch warning, got %q", errb.String())
	}
}

// seenContains reports whether the stub recorded a command whose joined args
// equal key. Note: seen stores only strings.Join(args, " ") — no dir prefix.
func seenContains(g *gitStub, key string) bool {
	for _, c := range g.seen {
		if c == key {
			return true
		}
	}
	return false
}

func TestFfLocalBase_NoLocalBranch(t *testing.T) {
	root := t.TempDir()
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}
	g.err[root+"|show-ref --verify --quiet refs/heads/main"] = errors.New("no ref")
	ffLocalBase(g, root, "main", io.Discard)
	for _, c := range g.seen {
		if strings.Contains(c, "merge --ff-only") || strings.Contains(c, "fetch origin main:main") {
			t.Fatalf("no local branch: must not advance anything, got %q", c)
		}
	}
}

func TestFfLocalBase_CurrentAndClean_MergesFfOnly(t *testing.T) {
	root := t.TempDir()
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}
	g.out[root+"|symbolic-ref --quiet --short HEAD"] = "main\n"
	g.out[root+"|status --porcelain"] = ""
	ffLocalBase(g, root, "main", io.Discard)
	if !seenContains(g, "merge --ff-only origin/main") {
		t.Fatalf("clean+current: expected merge --ff-only origin/main, seen=%v", g.seen)
	}
}

func TestFfLocalBase_CurrentAndDirty_Skips(t *testing.T) {
	root := t.TempDir()
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}
	g.out[root+"|symbolic-ref --quiet --short HEAD"] = "main\n"
	g.out[root+"|status --porcelain"] = " M internal/wt/new.go\n"
	ffLocalBase(g, root, "main", io.Discard)
	for _, c := range g.seen {
		if strings.Contains(c, "merge --ff-only") {
			t.Fatalf("dirty+current: must not merge, got %q", c)
		}
	}
}

func TestFfLocalBase_NotCurrent_FetchesRefspec(t *testing.T) {
	root := t.TempDir()
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}
	g.out[root+"|symbolic-ref --quiet --short HEAD"] = "namph/feat/other\n"
	ffLocalBase(g, root, "main", io.Discard)
	if !seenContains(g, "fetch origin main:main") {
		t.Fatalf("not current: expected fetch origin main:main, seen=%v", g.seen)
	}
	for _, c := range g.seen {
		if strings.Contains(c, "merge --ff-only") {
			t.Fatalf("not current: must not merge, got %q", c)
		}
	}
}

func TestFfLocalBase_DetachedHead_FetchesRefspec(t *testing.T) {
	root := t.TempDir()
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}
	// detached HEAD: symbolic-ref errors and yields empty output.
	g.err[root+"|symbolic-ref --quiet --short HEAD"] = errors.New("detached")
	ffLocalBase(g, root, "main", io.Discard)
	if !seenContains(g, "fetch origin main:main") {
		t.Fatalf("detached HEAD: expected fetch origin main:main, seen=%v", g.seen)
	}
	for _, c := range g.seen {
		if strings.Contains(c, "merge --ff-only") {
			t.Fatalf("detached HEAD: must not merge, got %q", c)
		}
	}
}

func TestRunNew_FetchesBeforeResolvingBase(t *testing.T) {
	t.Setenv("WT_SESSION", "")
	root := t.TempDir()
	seedCfg(t, root)
	g := &gitStub{out: map[string]string{}, err: map[string]error{}}
	g.err[root+"|rev-parse --verify --quiet origin/main"] = errors.New("bad rev")
	o := baseOpts(root, g)
	o.BaseOverride = "origin/main"
	_ = RunNew(o) // expected to error at the base-ref check; we only assert the fetch ran
	if !seenContains(g, "fetch origin --quiet") {
		t.Fatalf("expected auto-fetch before base resolution, seen=%v", g.seen)
	}
}

func TestTakenPorts_SkipsStaleEntries(t *testing.T) {
	root := t.TempDir()
	live := filepath.Join(root, ".worktrees", "live")
	if err := os.MkdirAll(live, 0o750); err != nil {
		t.Fatal(err)
	}
	st := &StateFile{Worktrees: []Worktree{
		{Slug: "live", Path: live, Ports: []int{3201}},
		{Slug: "gone", Path: filepath.Join(root, ".worktrees", "gone"), Ports: []int{3202}},
		{Slug: "nopath", Ports: []int{3203}}, // legacy entry without path: keep counting it
	}}
	got := takenPorts(st)
	if !got[3201] || got[3202] || !got[3203] {
		t.Fatalf("taken = %v, want 3201+3203 only", got)
	}
}
