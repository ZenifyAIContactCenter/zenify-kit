package wt

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
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
	items, err := sweepPlan(s, root, "origin/main", wd, nil)
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
	items, _ := sweepPlan(s, root, "origin/main", ".worktrees/", nil)
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
	items, _ := sweepPlan(s, root, "origin/main", ".worktrees/", nil)
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

func TestSweepPlan_StaleStateEntries(t *testing.T) {
	root := t.TempDir() // real dir so os.Stat on entry paths is meaningful
	wd := ".worktrees/"
	live := filepath.Join(root, ".worktrees", "live")
	if err := os.MkdirAll(live, 0o750); err != nil {
		t.Fatal(err)
	}
	list := "worktree " + root + "\n\n" + swList(live)
	s := swStub{
		out: map[string]string{
			root + "|worktree list --porcelain":                list,
			live + "|config --get wt.slug":                     "live",
			live + "|symbolic-ref --quiet --short HEAD":        "namph/feat/live",
			live + "|config --get wt.port":                     "3201",
			root + "|diff origin/main..namph/feat/live":        "d",
			root + "|diff origin/main..namph/feat/gone-merged": "",
			root + "|diff origin/main..namph/feat/gone-open":   "d",
		},
		err: map[string]error{
			root + "|merge-base --is-ancestor namph/feat/live origin/main":        errors.New("x"),
			root + "|merge-base --is-ancestor namph/feat/gone-merged origin/main": errors.New("x"),
			root + "|merge-base --is-ancestor namph/feat/gone-open origin/main":   errors.New("x"),
		},
	}
	st := &StateFile{Version: 1, Worktrees: []Worktree{
		{Slug: "live", Branch: "namph/feat/live", Path: live, Ports: []int{3201}},
		{Slug: "gone-merged", Branch: "namph/feat/gone-merged", Path: filepath.Join(root, ".worktrees", "gone-merged"), Ports: []int{3202}},
		{Slug: "gone-open", Branch: "namph/feat/gone-open", Path: filepath.Join(root, ".worktrees", "gone-open"), Ports: []int{3203}},
	}}
	items, err := sweepPlan(s, root, "origin/main", wd, st)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]SweepItem{}
	for _, it := range items {
		by[it.Slug] = it
	}
	if len(items) != 3 {
		t.Fatalf("want 3 items (live + 2 stale), got %d: %+v", len(items), items)
	}
	if by["live"].Stale {
		t.Errorf("live entry must not be stale: %+v", by["live"])
	}
	gm := by["gone-merged"]
	if !gm.Stale || !gm.Remove || !gm.BranchMerged || gm.Port != "3202" || gm.Reason != "stale: dir missing" {
		t.Errorf("gone-merged wrong: %+v", gm)
	}
	go_ := by["gone-open"]
	if !go_.Stale || !go_.Remove || go_.BranchMerged || go_.Port != "3203" {
		t.Errorf("gone-open wrong: %+v", go_)
	}
}

// An entry whose dir is gone but which git still lists is NOT stale — it is a
// hand-deleted worktree git knows about; the existing RunRm path handles it.
func TestSweepPlan_GoneDirStillInGitIsNotStale(t *testing.T) {
	root := t.TempDir()
	gone := filepath.Join(root, ".worktrees", "gone")
	list := "worktree " + root + "\n\n" + swList(gone)
	s := swStub{out: map[string]string{
		root + "|worktree list --porcelain":         list,
		gone + "|config --get wt.slug":              "", // dir gone: config fails → treated as not-wt
		gone + "|symbolic-ref --quiet --short HEAD": "",
	}}
	st := &StateFile{Worktrees: []Worktree{{Slug: "gone", Branch: "b", Path: gone, Ports: []int{3204}}}}
	items, _ := sweepPlan(s, root, "origin/main", ".worktrees/", st)
	for _, it := range items {
		if it.Stale {
			t.Fatalf("entry still registered in git must not be classified stale: %+v", it)
		}
	}
}

func seedSweepRepo(t *testing.T, root string, entries []Worktree) {
	t.Helper()
	writeWorktreeJSON(t, root, `{"abbrev":"r","user":"namph","portRange":[3200,3249],"deps":"none","baseRef":"origin/main"}`)
	if err := os.MkdirAll(filepath.Join(root, ".wt"), 0o750); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(StateFile{Version: 1, Worktrees: entries})
	if err := os.WriteFile(filepath.Join(root, ".wt", "state.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRunSweep_StaleMergedDropsEntryAndBranch(t *testing.T) {
	root := t.TempDir()
	gone := filepath.Join(root, ".worktrees", "gone")
	seedSweepRepo(t, root, []Worktree{{Slug: "gone", Branch: "namph/feat/gone", Path: gone, Ports: []int{3202}}})
	s := &rmStub{out: map[string]string{
		root + "|worktree list --porcelain":         "worktree " + root + "\n\n",
		root + "|diff origin/main..namph/feat/gone": "",
	}, err: map[string]error{
		root + "|merge-base --is-ancestor namph/feat/gone origin/main": errors.New("x"),
	}}
	var out bytes.Buffer
	if err := RunSweep(SweepOptions{RepoRoot: root, Host: "h", Pid: 1, Now: 1, Runner: s, Stdout: &out, Stderr: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if !s.ran(root+"|worktree prune") || !s.ran(root+"|branch -D namph/feat/gone") {
		t.Fatalf("expected prune + branch -D, seen: %v", s.seen)
	}
	st, _ := ReadState(root)
	if _, ok := st.Find("gone"); ok {
		t.Fatal("stale entry must be dropped from state")
	}
	if !strings.Contains(out.String(), "wt: swept 1, left 0") {
		t.Fatalf("summary line missing: %q", out.String())
	}
}

func TestRunSweep_StaleUnmergedDropsEntryKeepsBranch(t *testing.T) {
	root := t.TempDir()
	gone := filepath.Join(root, ".worktrees", "gone")
	seedSweepRepo(t, root, []Worktree{{Slug: "gone", Branch: "namph/feat/gone", Path: gone, Ports: []int{3202}}})
	s := &rmStub{out: map[string]string{
		root + "|worktree list --porcelain":         "worktree " + root + "\n\n",
		root + "|diff origin/main..namph/feat/gone": "d",
	}, err: map[string]error{
		root + "|merge-base --is-ancestor namph/feat/gone origin/main": errors.New("x"),
	}}
	var out bytes.Buffer
	if err := RunSweep(SweepOptions{RepoRoot: root, Host: "h", Pid: 1, Now: 1, Runner: s, Stdout: &out, Stderr: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if s.ran(root + "|branch -D namph/feat/gone") {
		t.Fatal("unmerged branch must be kept")
	}
	if !s.ran(root + "|worktree prune") {
		t.Fatal("prune expected")
	}
	st, _ := ReadState(root)
	if _, ok := st.Find("gone"); ok {
		t.Fatal("stale entry must be dropped from state")
	}
	if !strings.Contains(out.String(), "state dropped, branch namph/feat/gone kept (unmerged)") {
		t.Fatalf("expected kept-branch line, got %q", out.String())
	}
}

func TestRunSweep_QuietPrintsNothingWhenNothingRemovable(t *testing.T) {
	root := t.TempDir()
	seedSweepRepo(t, root, nil)
	s := &rmStub{out: map[string]string{root + "|worktree list --porcelain": "worktree " + root + "\n\n"}}
	var out bytes.Buffer
	if err := RunSweep(SweepOptions{RepoRoot: root, Host: "h", Pid: 1, Now: 1, Runner: s, Stdout: &out, Stderr: io.Discard, Quiet: true}); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("quiet sweep must print nothing, got %q", out.String())
	}
}

// Stale entry whose recorded path is the pre-migrate layout while the directory
// still sits at <repo>/.worktrees/<slug> unregistered in git: sweep must delete
// the orphan directory, drop the entry and delete the merged branch instead of
// failing on `git worktree remove`.
func TestRunSweep_StaleMergedOrphanDirIsDeleted(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "old-layout", ".worktrees", "orphan")
	seedSweepRepo(t, root, []Worktree{{Slug: "orphan", Branch: "namph/feat/orphan", Path: oldPath, Ports: []int{3202}}})
	dir := filepath.Join(root, ".worktrees", "orphan")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	s := &rmStub{out: map[string]string{
		root + "|worktree list --porcelain":           "worktree " + root + "\n\n",
		root + "|diff origin/main..namph/feat/orphan": "",
	}, err: map[string]error{
		root + "|merge-base --is-ancestor namph/feat/orphan origin/main": errors.New("x"),
		root + "|worktree remove --force " + dir:                         errors.New("exit status 128"),
		dir + "|symbolic-ref --quiet --short HEAD":                       errors.New("fatal: not a git repository"),
	}}
	var out bytes.Buffer
	if err := RunSweep(SweepOptions{RepoRoot: root, Host: "h", Pid: 1, Now: 1, Runner: s, Stdout: &out, Stderr: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if _, e := os.Stat(dir); !os.IsNotExist(e) {
		t.Fatalf("orphan dir must be deleted, stat err=%v", e)
	}
	if !s.ran(root + "|branch -D namph/feat/orphan") {
		t.Fatalf("merged branch must be deleted, seen: %v", s.seen)
	}
	st, _ := ReadState(root)
	if _, ok := st.Find("orphan"); ok {
		t.Fatal("stale entry must be dropped from state")
	}
	if !strings.Contains(out.String(), "wt: swept 1, left 0") {
		t.Fatalf("summary line missing: %q", out.String())
	}
}
