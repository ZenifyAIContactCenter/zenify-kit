package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeF(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestShadowCheck_IgnoresSkills(t *testing.T) {
	home, ws := t.TempDir(), t.TempDir()
	writeF(t, filepath.Join(home, ".claude/skills/cook/SKILL.md"), "x")
	writeF(t, filepath.Join(home, ".claude/agents/scout.md"), "x")
	writeF(t, filepath.Join(home, ".claude/rules/r.md"), "x")
	writeF(t, filepath.Join(ws, ".claude/rules/r.md"), "x")
	ok, d := shadowCheck(home, ws).Run()
	if !ok || !strings.HasPrefix(d, "cảnh báo:") { //znf:allow-lang
		t.Fatalf("ok=%v %q", ok, d)
	}
	if !strings.Contains(d, "shadow: rule r — xoá hoặc đổi tên bản cá nhân") { //znf:allow-lang
		t.Fatalf("rule shadow missing: %q", d)
	}
	if n := strings.Count(d, "shadow: "); n != 1 {
		t.Fatalf("want only the rule line, got %d: %q", n, d)
	}
}

func TestShadowCheck_Clean(t *testing.T) {
	home, ws := t.TempDir(), t.TempDir()
	writeF(t, filepath.Join(home, ".claude/skills/mine/SKILL.md"), "x")
	ok, d := shadowCheck(home, ws).Run()
	if !ok || d != "shadow: không có" { //znf:allow-lang
		t.Fatalf("ok=%v %q", ok, d)
	}
}

func TestContextBudgetWarnings(t *testing.T) {
	home, ws := t.TempDir(), t.TempDir()
	writeF(t, filepath.Join(home, ".claude/CLAUDE.md"), strings.Repeat("a", 9000))
	writeF(t, filepath.Join(ws, "CLAUDE.md"), strings.Repeat("a", 9000))
	ok, d := contextBudgetCheck(home, ws, nil).Run()
	if !ok || !strings.HasPrefix(d, "cảnh báo:") || !strings.Contains(d, "tổng: 18000 byte") { //znf:allow-lang
		t.Fatalf("ok=%v %q", ok, d)
	}
	// only the home file big: no warning
	writeF(t, filepath.Join(ws, "CLAUDE.md"), "small")
	_, d = contextBudgetCheck(home, ws, nil).Run()
	if strings.Contains(d, "cảnh báo") || !strings.Contains(d, "tổng: 9005 byte") { //znf:allow-lang
		t.Fatalf("%q", d)
	}
}

func TestContextBudgetPathsRuleNotCounted(t *testing.T) {
	home, ws := t.TempDir(), t.TempDir()
	writeF(t, filepath.Join(ws, "CLAUDE.md"), "abc")
	_, d1 := contextBudgetCheck(home, ws, nil).Run()
	writeF(t, filepath.Join(ws, ".claude/rules/scoped.md"), "---\npaths:\n  - src/**\n---\n"+strings.Repeat("a", 5*1024))
	_, d2 := contextBudgetCheck(home, ws, nil).Run()
	if d1 != d2 {
		t.Fatalf("paths rule counted: %q vs %q", d1, d2)
	}
	writeF(t, filepath.Join(ws, ".claude/rules/always.md"), "plain")
	_, d3 := contextBudgetCheck(home, ws, nil).Run()
	if d3 == d1 {
		t.Fatalf("always-on rule not counted: %q", d3)
	}
}

func TestContextBudgetRepoWarnings(t *testing.T) {
	home, ws := t.TempDir(), t.TempDir()
	writeF(t, filepath.Join(ws, "CLAUDE.md"), "x")
	writeF(t, filepath.Join(ws, "repos/a/CLAUDE.md"), strings.Repeat("l\n", 201))
	writeF(t, filepath.Join(ws, "repos/b/CLAUDE.md"), "├── x\n")
	writeF(t, filepath.Join(ws, "repos/c/CLAUDE.md"), strings.Repeat(strings.Repeat("c", 19)+"\n", 100))
	ok, d := contextBudgetCheck(home, ws, []string{"repos/a", "repos/b", "repos/c", "repos/d"}).Run()
	if !ok {
		t.Fatalf("ok=false %q", d)
	}
	line := func(r string) string {
		for _, l := range strings.Split(d, "\n") {
			if strings.Contains(l, r+"/CLAUDE.md") {
				return l
			}
		}
		t.Fatalf("no line for %s in %q", r, d)
		return ""
	}
	if !strings.Contains(line("repos/a"), "cảnh báo: >200 dòng") { //znf:allow-lang
		t.Fatalf("a: %q", line("repos/a"))
	}
	if !strings.Contains(line("repos/b"), "cảnh báo: cây thư mục") { //znf:allow-lang
		t.Fatalf("b: %q", line("repos/b"))
	}
	if strings.Contains(line("repos/c"), "cảnh báo") || !strings.Contains(line("repos/c"), "2000 byte, 100 dòng") { //znf:allow-lang
		t.Fatalf("c: %q", line("repos/c"))
	}
	if !strings.Contains(line("repos/d"), "thiếu") || strings.Contains(line("repos/d"), "cảnh báo") { //znf:allow-lang
		t.Fatalf("d: %q", line("repos/d"))
	}
}

func TestContextBudgetMissingRepoClaudeMd(t *testing.T) {
	home, ws := t.TempDir(), t.TempDir()
	writeF(t, filepath.Join(ws, "CLAUDE.md"), "x")
	ok, d := contextBudgetCheck(home, ws, []string{"repos/d"}).Run()
	if !ok || !strings.Contains(d, "thiếu") || strings.Contains(d, "cảnh báo") { //znf:allow-lang
		t.Fatalf("ok=%v %q", ok, d)
	}
}

func TestContextBudgetNoWorkspace(t *testing.T) {
	if ok, _ := contextBudgetCheck(t.TempDir(), "", nil).Run(); ok {
		t.Fatal("want ok=false")
	}
}

func TestHasPathsFrontmatter(t *testing.T) {
	cases := map[string]bool{
		"---\npaths:\n  - a\n---\nbody": true,
		"---\nname: x\n---\npaths: no":  false,
		"no front\npaths:\n":            false,
		"---\nname: x\n":                false,
	}
	for in, want := range cases {
		if got := hasPathsFrontmatter([]byte(in)); got != want {
			t.Errorf("%q: got %v", in, got)
		}
	}
}

func TestWorkspaceRootCheck(t *testing.T) { // SC-14
	ws := t.TempDir()
	for _, d := range []string{"repos", "docs/specs", "docs/research", ".claude", ".zenify", ".znf", "docs/.claude"} {
		os.MkdirAll(filepath.Join(ws, d), 0o755)
	}
	writeF(t, filepath.Join(ws, "foo.png"), "x")
	writeF(t, filepath.Join(ws, "CLAUDE.md"), "x")
	os.MkdirAll(filepath.Join(ws, "docs", "handoffs"), 0o755)
	ok, d := workspaceRootCheck(ws).Run()
	if ok || !strings.Contains(d, "foo.png") || !strings.Contains(d, "docs/handoffs") || !strings.Contains(d, ".znf") || strings.Contains(d, "docs/.claude") {
		t.Fatalf("ok=%v %q", ok, d)
	}
	os.Remove(filepath.Join(ws, "foo.png"))
	os.Remove(filepath.Join(ws, "docs", "handoffs"))
	os.Remove(filepath.Join(ws, ".znf"))
	if ok, d := workspaceRootCheck(ws).Run(); !ok {
		t.Fatalf("after cleanup: %q", d)
	}
}

func TestMemoryIndexCheck(t *testing.T) {
	ws := t.TempDir()
	if ok, d := memoryIndexCheck(ws).Run(); !ok || d != "không có MEMORY.md" { //znf:allow-lang
		t.Fatal(d)
	}
	writeF(t, filepath.Join(ws, ".claude/memory/MEMORY.md"), strings.Repeat("x\n", 185))
	if ok, d := memoryIndexCheck(ws).Run(); ok || !strings.Contains(d, "185") || !strings.Contains(d, "200") {
		t.Fatalf("ok=%v %q", ok, d)
	}
	writeF(t, filepath.Join(ws, ".claude/memory/MEMORY.md"), strings.Repeat(strings.Repeat("x", 67)+"\n", 150))
	if ok, _ := memoryIndexCheck(ws).Run(); !ok {
		t.Fatal("150 lines / 10 KB must be ok")
	}
}

func TestWorkspaceRootCheck_OverlayRepoRootNotFlagged(t *testing.T) {
	ws := t.TempDir()
	os.MkdirAll(filepath.Join(ws, "elsewhere", "a"), 0o755)
	os.MkdirAll(filepath.Join(ws, "stray"), 0o755)
	writeF(t, filepath.Join(ws, ".zenify-overlay.yaml"), "repos:\n  - name: contact-center-be\n    path: elsewhere/a\n")
	ok, d := workspaceRootCheck(ws).Run()
	if ok || strings.Contains(d, "elsewhere") || !strings.Contains(d, "stray") {
		t.Fatalf("ok=%v %q", ok, d)
	}
}
