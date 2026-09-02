package wt

import (
	"errors"
	"io"
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
