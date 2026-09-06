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
		{Name: "beta", Path: "/ws/repos/beta"}, // Skip — đã ở repos/
		{Name: "dup", Path: "/ws/dup"},         // Refuse — trùng basename
		{Name: "dup", Path: "/ws/repos/dup"},   // Refuse — trùng basename
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
		t.Fatalf("beta muốn Skip: %+v", it)
	}
	for _, k := range []string{"dup|/ws/dup", "dup|/ws/repos/dup"} {
		if it := by[k]; it.Action != Refuse {
			t.Fatalf("%s muốn Refuse (trùng basename): %+v", k, it)
		}
	}
}

func TestNewWorktreePath(t *testing.T) {
	// nội bộ: rebase prefix
	got := newWorktreePath("/ws/repo/.worktrees/wt1", "/ws/repo", "/ws/repos/repo")
	if got != "/ws/repos/repo/.worktrees/wt1" {
		t.Fatalf("internal: got %q", got)
	}
	// ngoài (herdr): giữ nguyên
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
		ListWT:   func(d string) ([]string, error) { return []string{d + "/.worktrees/w1"}, nil },
		Move:     func(from, to string) error { moves = append(moves, from+"->"+to); return nil },
		MkdirAll: func(d string) error { return nil },
		Repair:   func(repo, wt string) error { repairs = append(repairs, repo+"|"+wt); return nil },
		Repoint:  func(a, b, c, d string) error { repoints = append(repoints, b); return nil },
		UpdateYAML: func(name, np string) error { yaml = append(yaml, name+"="+np); return nil },
	}
	Apply(items, io)
	// chỉ alpha move; worktree của nó được repair tại path mới; YAML cập nhật alpha.
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
		ListWT:   func(d string) ([]string, error) { return []string{d + "/.worktrees/w1"}, nil },
		Move:     func(from, to string) error { moves = append(moves, from+"->"+to); return nil },
		MkdirAll: func(d string) error { return nil },
		Repair:   func(repo, wt string) error { return fmt.Errorf("repair boom") },
		Repoint:  func(a, b, c, d string) error { return nil },
		UpdateYAML: func(name, np string) error { yaml = append(yaml, name); return nil },
	}
	notes := Apply(items, io)
	// move đi rồi move-back → 2 lời gọi Move; KHÔNG update YAML.
	if len(moves) != 2 || moves[1] != "/ws/repos/alpha->/ws/alpha" {
		t.Fatalf("muốn rollback move-back, moves=%v", moves)
	}
	if len(yaml) != 0 {
		t.Fatalf("repair fail thì KHÔNG update YAML, yaml=%v", yaml)
	}
	if !containsSub(notes, "rollback") {
		t.Fatalf("note phải nhắc rollback: %v", notes)
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
