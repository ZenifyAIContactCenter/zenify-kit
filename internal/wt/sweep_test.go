package wt

import (
	"errors"
	"strings"
	"testing"
)

type swStub struct {
	out map[string]string
	err map[string]error
}

func (s swStub) Run(dir string, args ...string) ([]byte, error) {
	k := dir + "|" + strings.Join(args, " ")
	if e, ok := s.err[k]; ok {
		return nil, e
	}
	return []byte(s.out[k]), nil
}

func swList(paths ...string) string {
	var b strings.Builder
	for _, p := range paths {
		b.WriteString("worktree " + p + "\n\n")
	}
	return b.String()
}

func TestSweepPlan_Classifies(t *testing.T) {
	root := "/repo"
	wd := ".worktrees/"
	// four worktrees + main: removable, unmerged, dirty, not-wt
	list := "worktree /repo\n\n" + swList(
		"/repo/.worktrees/done",   // merged+clean → remove
		"/repo/.worktrees/wip",    // unmerged → keep
		"/repo/.worktrees/dirty",  // merged but dirty → keep
		"/repo/.worktrees/manual", // no wt.slug → keep
	)
	s := swStub{
		out: map[string]string{
			root + "|worktree list --porcelain": list,

			"/repo/.worktrees/done|config --get wt.slug":              "done",
			"/repo/.worktrees/done|symbolic-ref --quiet --short HEAD": "namph/feat/done",
			"/repo/.worktrees/done|config --get wt.port":              "3207",
			"/repo/.worktrees/done|status --porcelain":                "",
			root + "|diff origin/main..namph/feat/done":               "", // empty → merged

			"/repo/.worktrees/wip|config --get wt.slug":              "wip",
			"/repo/.worktrees/wip|symbolic-ref --quiet --short HEAD": "namph/feat/wip",
			"/repo/.worktrees/wip|config --get wt.port":              "3208",
			root + "|diff origin/main..namph/feat/wip":               "d", // non-empty → unmerged

			"/repo/.worktrees/dirty|config --get wt.slug":              "dirty",
			"/repo/.worktrees/dirty|symbolic-ref --quiet --short HEAD": "namph/feat/dirty",
			"/repo/.worktrees/dirty|config --get wt.port":              "3209",
			"/repo/.worktrees/dirty|status --porcelain":                " M x.go",
			root + "|diff origin/main..namph/feat/dirty":               "",

			"/repo/.worktrees/manual|config --get wt.slug":              "", // not wt
			"/repo/.worktrees/manual|symbolic-ref --quiet --short HEAD": "some/branch",
		},
		err: map[string]error{
			root + "|merge-base --is-ancestor namph/feat/done origin/main":  errors.New("x"),
			root + "|merge-base --is-ancestor namph/feat/wip origin/main":   errors.New("x"),
			root + "|merge-base --is-ancestor namph/feat/dirty origin/main": errors.New("x"),
		},
	}
	items, err := sweepPlan(s, root, "origin/main", wd)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 {
		t.Fatalf("want 4 items (main excluded), got %d: %+v", len(items), items)
	}
	by := map[string]SweepItem{}
	for _, it := range items {
		key := it.Slug
		if key == "" {
			key = "(manual)"
		}
		by[key] = it
	}
	if !by["done"].Remove {
		t.Errorf("done should be removable: %+v", by["done"])
	}
	if by["wip"].Remove || !strings.Contains(by["wip"].Reason, "merge trace") {
		t.Errorf("wip should be kept (no merge trace): %+v", by["wip"])
	}
	if by["dirty"].Remove || !strings.Contains(by["dirty"].Reason, "uncommitted") {
		t.Errorf("dirty should be kept: %+v", by["dirty"])
	}
	if by["(manual)"].Remove || !strings.Contains(by["(manual)"].Reason, "not created by wt") {
		t.Errorf("manual should be kept: %+v", by["(manual)"])
	}
}

func TestSweepPlan_DetachedKept(t *testing.T) {
	root := "/repo"
	list := swList("/repo/.worktrees/det")
	s := swStub{out: map[string]string{
		root + "|worktree list --porcelain":                      list,
		"/repo/.worktrees/det|config --get wt.slug":              "det",
		"/repo/.worktrees/det|symbolic-ref --quiet --short HEAD": "", // detached
	}}
	items, _ := sweepPlan(s, root, "origin/main", ".worktrees/")
	if len(items) != 1 || items[0].Remove || !strings.Contains(items[0].Reason, "detached") {
		t.Fatalf("detached must be kept: %+v", items)
	}
}

func TestRunSweep_DryRunCountsAndRemovesNothing(t *testing.T) {
	root := "/repo"
	list := "worktree /repo\n\n" + swList("/repo/.worktrees/done", "/repo/.worktrees/wip")
	s := swStub{
		out: map[string]string{
			root + "|worktree list --porcelain":                       list,
			"/repo/.worktrees/done|config --get wt.slug":              "done",
			"/repo/.worktrees/done|symbolic-ref --quiet --short HEAD": "namph/feat/done",
			"/repo/.worktrees/done|config --get wt.port":              "3207",
			"/repo/.worktrees/done|status --porcelain":                "",
			root + "|diff origin/main..namph/feat/done":               "",
			"/repo/.worktrees/wip|config --get wt.slug":               "wip",
			"/repo/.worktrees/wip|symbolic-ref --quiet --short HEAD":  "namph/feat/wip",
			"/repo/.worktrees/wip|config --get wt.port":               "3208",
			root + "|diff origin/main..namph/feat/wip":                "d",
		},
		err: map[string]error{
			root + "|merge-base --is-ancestor namph/feat/done origin/main": errors.New("x"),
			root + "|merge-base --is-ancestor namph/feat/wip origin/main":  errors.New("x"),
		},
	}
	items, _ := sweepPlan(s, root, "origin/main", ".worktrees/")
	removable := 0
	for _, it := range items {
		if it.Remove {
			removable++
		}
	}
	if removable != 1 {
		t.Fatalf("want exactly 1 removable (done), got %d", removable)
	}
}
