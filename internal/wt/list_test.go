package wt

import (
	"errors"
	"strings"
	"testing"
)

// lsStub is a fake gitx.Runner keyed on "dir|args" (same style as guard_test).
type lsStub struct {
	out map[string]string
	err map[string]error
}

func (s lsStub) Run(dir string, args ...string) ([]byte, error) {
	k := dir + "|" + strings.Join(args, " ")
	if e, ok := s.err[k]; ok {
		return nil, e
	}
	return []byte(s.out[k]), nil
}

func lsCfg() *Config {
	return &Config{BaseRef: "origin/main", WorktreeDir: ".worktrees/", PortEnv: "PORT", User: "namph", PortRange: [2]int{3200, 3249}}
}

func TestList_JoinsGitAndConfig(t *testing.T) {
	root := "/repo"
	list := strings.Join([]string{
		"worktree /repo",
		"branch refs/heads/staging",
		"",
		"worktree /repo/.worktrees/foo",
		"branch refs/heads/namph/feat/foo",
		"",
	}, "\n")
	s := lsStub{
		out: map[string]string{
			root + "|worktree list --porcelain":        list,
			"/repo/.worktrees/foo|config --get wt.slug": "foo",
			"/repo/.worktrees/foo|config --get wt.port": "3207",
			"/repo/.worktrees/foo|config --get wt.deps": "clone",
			root + "|diff origin/main..namph/feat/foo":  "diff z", // unmerged
		},
		err: map[string]error{
			root + "|merge-base --is-ancestor namph/feat/foo origin/main": errors.New("not ancestor"),
		},
	}
	rows, err := List(s, root, lsCfg())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 worktree row (main excluded), got %d: %+v", len(rows), rows)
	}
	r := rows[0]
	if r.Slug != "foo" || r.Branch != "namph/feat/foo" || r.Port != "3207" || r.Deps != "clone" || r.Merged != "-" {
		t.Fatalf("row join wrong: %+v", r)
	}
	if r.Running != "running" && r.Running != "-" {
		t.Fatalf("Running must be running|-, got %q", r.Running)
	}
	if r.Path != "/repo/.worktrees/foo" {
		t.Fatalf("path wrong: %q", r.Path)
	}
}

func TestList_MergedAndUnmanaged(t *testing.T) {
	root := "/repo"
	list := strings.Join([]string{
		"worktree /repo/.worktrees/done",
		"branch refs/heads/namph/feat/done",
		"",
	}, "\n")
	s := lsStub{
		out: map[string]string{
			root + "|worktree list --porcelain":         list,
			"/repo/.worktrees/done|config --get wt.slug": "", // unmanaged (no slug)
			"/repo/.worktrees/done|config --get wt.port": "",
			"/repo/.worktrees/done|config --get wt.deps": "",
			root + "|diff origin/main..namph/feat/done":  "", // empty diff → merged
		},
		err: map[string]error{
			root + "|merge-base --is-ancestor namph/feat/done origin/main": errors.New("not ancestor"),
		},
	}
	rows, _ := List(s, root, lsCfg())
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if rows[0].Slug != "(unmanaged)" || rows[0].Port != "-" || rows[0].Deps != "-" || rows[0].Merged != "merged" {
		t.Fatalf("unmanaged/merged row wrong: %+v", rows[0])
	}
}

func TestURLFor(t *testing.T) {
	root := "/repo"
	list := strings.Join([]string{
		"worktree /repo/.worktrees/foo",
		"branch refs/heads/namph/feat/foo",
		"",
	}, "\n")
	s := lsStub{
		out: map[string]string{
			root + "|worktree list --porcelain":        list,
			"/repo/.worktrees/foo|config --get wt.slug": "foo",
			"/repo/.worktrees/foo|config --get wt.port": "3207",
			"/repo/.worktrees/foo|config --get wt.deps": "install",
			root + "|diff origin/main..namph/feat/foo":  "d",
		},
		err: map[string]error{root + "|merge-base --is-ancestor namph/feat/foo origin/main": errors.New("x")},
	}
	url, err := URLFor(s, root, lsCfg(), "foo")
	if err != nil {
		t.Fatal(err)
	}
	if url != "http://localhost:3207" {
		t.Fatalf("url wrong: %q", url)
	}
	if _, err := URLFor(s, root, lsCfg(), "nope"); err == nil {
		t.Fatal("unknown slug must error")
	}
}
