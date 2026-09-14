package wt

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFormatReport_OneLineOnlyWhenSomethingToSweep(t *testing.T) {
	got := FormatReport([]RepoCount{{"web", 3, 3}, {"hub", 1, 1}, {"kit", 1, 0}, {"notification", 0, 1}, {"be", 0, 0}})
	want := "wt: 5 merged worktree(s) + 5 stale entr(ies) sweepable — web 3+3, hub 1+1, kit 1, notification 0+1 → run `wt sweep --all`"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	if FormatReport([]RepoCount{{"be", 0, 0}}) != "" || FormatReport(nil) != "" {
		t.Fatal("nothing to sweep must format as empty string")
	}
}

func TestCountSweepable_NoFetchNoLsof(t *testing.T) {
	root := t.TempDir()
	gone := filepath.Join(root, ".worktrees", "gone")
	done := filepath.Join(root, ".worktrees", "done")
	if err := os.MkdirAll(done, 0o750); err != nil {
		t.Fatal(err)
	}
	seedSweepRepo(t, root, []Worktree{{Slug: "gone", Branch: "namph/feat/gone", Path: gone, Ports: []int{3202}}})
	s := &rmStub{out: map[string]string{
		root + "|worktree list --porcelain":         "worktree " + root + "\n\n" + "worktree " + done + "\n\n",
		done + "|config --get wt.slug":              "done",
		done + "|symbolic-ref --quiet --short HEAD": "namph/feat/done",
		done + "|config --get wt.port":              "3207",
		done + "|status --porcelain":                "",
		root + "|diff origin/main..namph/feat/done": "",
		root + "|diff origin/main..namph/feat/gone": "d",
	}, err: map[string]error{
		root + "|merge-base --is-ancestor namph/feat/done origin/main": errors.New("x"),
		root + "|merge-base --is-ancestor namph/feat/gone origin/main": errors.New("x"),
	}}
	c, err := CountSweepable(s, root)
	if err != nil {
		t.Fatal(err)
	}
	if c.Abbrev != "r" || c.Removable != 1 || c.Stale != 1 {
		t.Fatalf("count = %+v, want r/1/1", c)
	}
	if s.ran(root + "|fetch origin --quiet") {
		t.Fatal("report must never fetch")
	}
}
