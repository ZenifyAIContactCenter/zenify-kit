package migrate

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/workspace"
)

func TestBuildPlanClassifies(t *testing.T) {
	root := "/ws"
	repos := []workspace.Repo{
		{Name: "alpha", Path: "/ws/alpha"},     // Move
		{Name: "beta", Path: "/ws/repos/beta"}, // Skip — already in repos/
		{Name: "dup", Path: "/ws/dup"},         // Refuse — basename collision
		{Name: "dup", Path: "/ws/repos/dup"},   // Refuse — basename collision
	}
	items := BuildPlan(root, "repos", repos)

	by := map[string]Item{}
	for _, it := range items {
		by[it.Name+"|"+it.From] = it
	}
	if it := by["alpha|/ws/alpha"]; it.Action != Move || it.To != "/ws/repos/alpha" {
		t.Fatalf("alpha: %+v", it)
	}
	if it := by["beta|/ws/repos/beta"]; it.Action != Skip {
		t.Fatalf("beta should be Skip: %+v", it)
	}
	for _, k := range []string{"dup|/ws/dup", "dup|/ws/repos/dup"} {
		if it := by[k]; it.Action != Refuse {
			t.Fatalf("%s should be Refuse (basename collision): %+v", k, it)
		}
	}
}

func TestNewWorktreePath(t *testing.T) {
	// internal: rebase prefix
	got := newWorktreePath("/ws/repo/.worktrees/wt1", "/ws/repo", "/ws/repos/repo")
	if got != "/ws/repos/repo/.worktrees/wt1" {
		t.Fatalf("internal: got %q", got)
	}
	// external (herdr): unchanged
	got = newWorktreePath("/home/u/.herdr/worktrees/repo/wc", "/ws/repo", "/ws/repos/repo")
	if got != "/home/u/.herdr/worktrees/repo/wc" {
		t.Fatalf("external: got %q", got)
	}
}

func TestApplyMoveRepairSuccess(t *testing.T) {
	items := []Item{
		{Name: "alpha", From: "/ws/alpha", To: "/ws/repos/alpha", Action: Move},
		{Name: "beta", From: "/ws/beta", To: "", Action: Skip},
	}
	var moves, repairs, repoints, yaml []string
	io := ApplyIO{
		ListWT:     func(d string) ([]string, error) { return []string{d + "/.worktrees/w1"}, nil },
		Move:       func(from, to string) error { moves = append(moves, from+"->"+to); return nil },
		MkdirAll:   func(d string) error { return nil },
		Repair:     func(repo, wt string) error { repairs = append(repairs, repo+"|"+wt); return nil },
		Repoint:    func(a, b, c, d string) error { repoints = append(repoints, b); return nil },
		UpdateYAML: func(name, np string) error { yaml = append(yaml, name+"="+np); return nil },
		Resolve:    func(p string) string { return p },
	}
	Apply(items, io)
	// only alpha moves; its worktree is repaired at the new path; YAML updates alpha.
	if len(moves) != 1 || moves[0] != "/ws/alpha->/ws/repos/alpha" {
		t.Fatalf("moves: %v", moves)
	}
	if len(repairs) != 1 || repairs[0] != "/ws/repos/alpha|/ws/repos/alpha/.worktrees/w1" {
		t.Fatalf("repairs: %v", repairs)
	}
	if len(repoints) != 1 || repoints[0] != "/ws/repos/alpha/.worktrees/w1" {
		t.Fatalf("repoints: %v", repoints)
	}
	if len(yaml) != 1 || yaml[0] != "alpha=repos/alpha" {
		t.Fatalf("yaml: %v", yaml)
	}
}

func TestApplyRepairFailRollsBack(t *testing.T) {
	items := []Item{{Name: "alpha", From: "/ws/alpha", To: "/ws/repos/alpha", Action: Move}}
	var moves, yaml []string
	io := ApplyIO{
		ListWT:     func(d string) ([]string, error) { return []string{d + "/.worktrees/w1"}, nil },
		Move:       func(from, to string) error { moves = append(moves, from+"->"+to); return nil },
		MkdirAll:   func(d string) error { return nil },
		Repair:     func(repo, wt string) error { return fmt.Errorf("repair boom") },
		Repoint:    func(a, b, c, d string) error { return nil },
		UpdateYAML: func(name, np string) error { yaml = append(yaml, name); return nil },
		Resolve:    func(p string) string { return p },
	}
	notes := Apply(items, io)
	// moved out then moved back → 2 Move calls; YAML NOT updated.
	if len(moves) != 2 || moves[1] != "/ws/repos/alpha->/ws/alpha" {
		t.Fatalf("expected rollback move-back, moves=%v", moves)
	}
	if len(yaml) != 0 {
		t.Fatalf("repair failure must NOT update YAML, yaml=%v", yaml)
	}
	if !containsSub(notes, "rollback") {
		t.Fatalf("note must mention rollback: %v", notes)
	}
}

// TestApplyResolvesRepoOldForWorktreeClassification: ListWT (git worktree list) returns a
// path that is ALREADY symlink-resolved, while it.From (workspace.Discover) is a RAW path —
// if Apply prefix-matches using the raw it.From, an internal worktree under a workspace
// symlink (macOS /var→/private/var) is mistaken for one outside the repo, Repair runs
// against the wrong path, and the repo gets wrongly refused+rolled back.
func TestApplyResolvesRepoOldForWorktreeClassification(t *testing.T) {
	items := []Item{{Name: "alpha", From: "/var/ws/alpha", To: "/var/ws/repos/alpha", Action: Move}}
	var repairs []string
	io := ApplyIO{
		ListWT:     func(d string) ([]string, error) { return []string{"/private/var/ws/alpha/.worktrees/w1"}, nil },
		Move:       func(from, to string) error { return nil },
		MkdirAll:   func(d string) error { return nil },
		Repair:     func(repo, wt string) error { repairs = append(repairs, repo+"|"+wt); return nil },
		Repoint:    func(a, b, c, d string) error { return nil },
		UpdateYAML: func(name, np string) error { return nil },
		Resolve: func(p string) string {
			if p == "/var/ws/alpha" {
				return "/private/var/ws/alpha"
			}
			return p
		},
	}
	Apply(items, io)
	want := "/var/ws/repos/alpha|/var/ws/repos/alpha/.worktrees/w1"
	if len(repairs) != 1 || repairs[0] != want {
		t.Fatalf("worktree must be classified INTERNAL + repaired at the new path under it.To: %v (want %q)", repairs, want)
	}
}

// TestApplyPartialWorktreeFailureRestoresEarlierOnes: a repo has 2 worktrees, the 2nd
// worktree fails repair → the whole repo moves back, but the 1st worktree, already
// repaired+repointed before that, is now dangling (still pointing at it.To, which is no
// longer there) — Apply must un-repair it back to the old path, and the note must
// accurately reflect that it was restored (must not falsely claim "fully back to how it
// was" without verifying).
func TestApplyPartialWorktreeFailureRestoresEarlierOnes(t *testing.T) {
	items := []Item{{Name: "alpha", From: "/ws/alpha", To: "/ws/repos/alpha", Action: Move}}
	var moves, repairs, repoints, yaml []string
	io := ApplyIO{
		ListWT: func(d string) ([]string, error) {
			return []string{d + "/.worktrees/w1", d + "/.worktrees/w2"}, nil
		},
		Move:     func(from, to string) error { moves = append(moves, from+"->"+to); return nil },
		MkdirAll: func(d string) error { return nil },
		Repair: func(repo, wt string) error {
			repairs = append(repairs, repo+"|"+wt)
			// w2 fails ONLY in the forward direction (repo=it.To); the restore direction
			// (repo=it.From) must succeed so the un-repair branch is also exercised.
			if repo == "/ws/repos/alpha" && strings.Contains(wt, "w2") {
				return fmt.Errorf("repair w2 boom")
			}
			return nil
		},
		Repoint:    func(a, b, c, d string) error { repoints = append(repoints, a+">"+b); return nil },
		UpdateYAML: func(name, np string) error { yaml = append(yaml, name); return nil },
		Resolve:    func(p string) string { return p },
	}
	notes := Apply(items, io)

	// (a) move-back happened.
	if len(moves) != 2 || moves[0] != "/ws/alpha->/ws/repos/alpha" || moves[1] != "/ws/repos/alpha->/ws/alpha" {
		t.Fatalf("expected move out then move-back: %v", moves)
	}
	// (b) w1 (already repaired successfully before w2 failed) must have a RESTORING Repair
	// call at the old path (repo=it.From, wt=old worktree path).
	if !containsSub(repairs, "/ws/alpha|/ws/alpha/.worktrees/w1") {
		t.Fatalf("missing restoring Repair call for w1 at the old path: %v", repairs)
	}
	// (c) w1's node_modules must be repointed BACKWARD (mainNew→mainOld reversed to mainOld→mainNew args).
	if !containsSub(repoints, "/ws/repos/alpha/.worktrees/w1>/ws/alpha/.worktrees/w1") {
		t.Fatalf("missing restoring repoint for w1: %v", repoints)
	}
	// (d) YAML NOT updated.
	if len(yaml) != 0 {
		t.Fatalf("repair failure must NOT update YAML, yaml=%v", yaml)
	}
	// (e) note must honestly say it was restored (not just plain "rollback", must mention restored).
	if !containsSub(notes, "rollback") || !containsSub(notes, "restored") {
		t.Fatalf("note must mention rollback AND that earlier worktrees were restored: %v", notes)
	}
}

// TestApplyRepointFailAtBoundaryWorktreeIncludedInRestore: worktree #2 PASSES Repair (its
// gitdir link now points into it.To) but then FAILS Repoint. Before the Finding-A fix,
// okCount was only incremented after BOTH Repair and Repoint succeeded, so worktree #2 was
// excluded from the restore loop — after move-back it stayed dangling (gitdir link pointing
// at a now-gone it.To) with no un-repair and no honest note about it.
func TestApplyRepointFailAtBoundaryWorktreeIncludedInRestore(t *testing.T) {
	items := []Item{{Name: "alpha", From: "/ws/alpha", To: "/ws/repos/alpha", Action: Move}}
	var moves, repairs, yaml []string
	io := ApplyIO{
		ListWT: func(d string) ([]string, error) {
			return []string{d + "/.worktrees/w1", d + "/.worktrees/w2"}, nil
		},
		Move:     func(from, to string) error { moves = append(moves, from+"->"+to); return nil },
		MkdirAll: func(d string) error { return nil },
		Repair: func(repo, wt string) error {
			repairs = append(repairs, repo+"|"+wt)
			return nil // Repair always PASSES both worktrees — the failure only happens at Repoint.
		},
		Repoint: func(wtOld, wtNew, mainOld, mainNew string) error {
			// w2 fails ONLY in the forward direction (mainNew=it.To); the restore direction
			// (mainNew=it.From) must PASS so the un-repair branch for the boundary worktree is
			// also exercised.
			if strings.Contains(wtOld, "w2") && mainNew == "/ws/repos/alpha" {
				return fmt.Errorf("repoint w2 boom")
			}
			return nil
		},
		UpdateYAML: func(name, np string) error { yaml = append(yaml, name); return nil },
		Resolve:    func(p string) string { return p },
	}
	notes := Apply(items, io)

	if len(moves) != 2 || moves[1] != "/ws/repos/alpha->/ws/alpha" {
		t.Fatalf("expected move out then move-back: %v", moves)
	}
	if !containsSub(repairs, "/ws/alpha|/ws/alpha/.worktrees/w1") {
		t.Fatalf("missing restoring Repair for w1 (worktree BEFORE the failing one): %v", repairs)
	}
	// This is what Finding A fixes: the BOUNDARY worktree (w2 — Repair passes, only Repoint
	// fails) must also be un-repaired, not left out of the restore loop.
	if !containsSub(repairs, "/ws/alpha|/ws/alpha/.worktrees/w2") {
		t.Fatalf("missing restoring Repair for w2 (boundary worktree, Repair passes but Repoint fails): %v", repairs)
	}
	if len(yaml) != 0 {
		t.Fatalf("repoint failure must NOT update YAML: %v", yaml)
	}
	if !containsSub(notes, "rollback") {
		t.Fatalf("note must mention rollback: %v", notes)
	}
}

func containsSub(ss []string, sub string) bool {
	for _, s := range ss {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
