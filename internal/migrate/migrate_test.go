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
		ListWT:     func(d string) ([]string, error) { return []string{d + "/.worktrees/w1"}, nil },
		Move:       func(from, to string) error { moves = append(moves, from+"->"+to); return nil },
		MkdirAll:   func(d string) error { return nil },
		Repair:     func(repo, wt string) error { repairs = append(repairs, repo+"|"+wt); return nil },
		Repoint:    func(a, b, c, d string) error { repoints = append(repoints, b); return nil },
		UpdateYAML: func(name, np string) error { yaml = append(yaml, name+"="+np); return nil },
		Resolve:    func(p string) string { return p },
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
		ListWT:     func(d string) ([]string, error) { return []string{d + "/.worktrees/w1"}, nil },
		Move:       func(from, to string) error { moves = append(moves, from+"->"+to); return nil },
		MkdirAll:   func(d string) error { return nil },
		Repair:     func(repo, wt string) error { return fmt.Errorf("repair boom") },
		Repoint:    func(a, b, c, d string) error { return nil },
		UpdateYAML: func(name, np string) error { yaml = append(yaml, name); return nil },
		Resolve:    func(p string) string { return p },
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

// TestApplyResolvesRepoOldForWorktreeClassification: ListWT (git worktree list) trả path
// ĐÃ resolve symlink, còn it.From (workspace.Discover) là path THÔ — nếu Apply so khớp
// prefix bằng it.From thô thì worktree nội bộ dưới workspace symlink (macOS /var→/private/var)
// bị coi nhầm là ngoài repo, Repair chạy sai path và repo bị refuse+rollback oan.
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
		t.Fatalf("worktree phải được coi NỘI BỘ + repair tại path mới dưới it.To: %v (muốn %q)", repairs, want)
	}
}

// TestApplyPartialWorktreeFailureRestoresEarlierOnes: repo có 2 worktree, worktree THỨ 2
// fail repair → move-back cả repo, nhưng worktree THỨ NHẤT đã repair+repoint xong TRƯỚC đó
// giờ dangling (còn trỏ vào it.To đã không còn ở đó) — Apply phải un-repair nó về path cũ,
// và note phải phản ánh đúng việc đã khôi phục (không được nói dối "hoàn toàn như cũ" mà
// không kiểm chứng).
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
			// w2 fail CHỈ ở hướng forward (repo=it.To); hướng restore (repo=it.From) phải
			// thành công để test được cả nhánh un-repair.
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

	// (a) move-back đã xảy ra.
	if len(moves) != 2 || moves[0] != "/ws/alpha->/ws/repos/alpha" || moves[1] != "/ws/repos/alpha->/ws/alpha" {
		t.Fatalf("muốn move đi rồi move-back: %v", moves)
	}
	// (b) w1 (đã repair thành công trước khi w2 fail) phải có lời gọi Repair KHÔI PHỤC
	// tại path cũ (repo=it.From, wt=path worktree cũ).
	if !containsSub(repairs, "/ws/alpha|/ws/alpha/.worktrees/w1") {
		t.Fatalf("thiếu lời gọi Repair khôi phục w1 tại path cũ: %v", repairs)
	}
	// (c) node_modules của w1 phải được repoint NGƯỢC (mainNew→mainOld đảo thành mainOld→mainNew args).
	if !containsSub(repoints, "/ws/repos/alpha/.worktrees/w1>/ws/alpha/.worktrees/w1") {
		t.Fatalf("thiếu repoint khôi phục cho w1: %v", repoints)
	}
	// (d) KHÔNG update YAML.
	if len(yaml) != 0 {
		t.Fatalf("repair fail thì KHÔNG update YAML, yaml=%v", yaml)
	}
	// (e) note phải nói thật là đã khôi phục (không chỉ "rollback" trơn, phải nhắc khôi phục).
	if !containsSub(notes, "rollback") || !containsSub(notes, "khôi phục") {
		t.Fatalf("note phải nhắc rollback VÀ đã khôi phục worktree trước đó: %v", notes)
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
			return nil // Repair luôn PASS cả 2 worktree — lỗi chỉ xảy ra ở Repoint.
		},
		Repoint: func(wtOld, wtNew, mainOld, mainNew string) error {
			// w2 fail CHỈ ở hướng forward (mainNew=it.To); hướng restore (mainNew=it.From)
			// phải PASS để test được nhánh un-repair cho worktree biên.
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
		t.Fatalf("muốn move đi rồi move-back: %v", moves)
	}
	if !containsSub(repairs, "/ws/alpha|/ws/alpha/.worktrees/w1") {
		t.Fatalf("thiếu Repair khôi phục w1 (worktree TRƯỚC worktree lỗi): %v", repairs)
	}
	// Đây là điều Finding A sửa: worktree BIÊN (w2 — Repair pass, chỉ Repoint fail) cũng
	// phải được un-repair, không được bỏ sót khỏi restore loop.
	if !containsSub(repairs, "/ws/alpha|/ws/alpha/.worktrees/w2") {
		t.Fatalf("thiếu Repair khôi phục w2 (worktree biên, Repair pass nhưng Repoint fail): %v", repairs)
	}
	if len(yaml) != 0 {
		t.Fatalf("repoint fail thì KHÔNG update YAML: %v", yaml)
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
