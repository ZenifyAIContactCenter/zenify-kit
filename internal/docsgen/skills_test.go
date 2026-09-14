package docsgen

import (
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/plugin"
)

func TestParseFrontmatter(t *testing.T) {
	b, _ := os.ReadFile("testdata/skills/demo/SKILL.md")
	fm, err := ParseFrontmatter(b)
	if err != nil {
		t.Fatal(err)
	}
	if fm.Name != "demo" || fm.ArgumentHint != "<slug>" || fm.AllowedTools != "Read Bash" || !fm.DisableModelInvocation {
		t.Fatalf("bad parse: %+v", fm)
	}
	if _, err := ParseFrontmatter([]byte("---\ndescription: no name\n---\n")); err == nil {
		t.Fatal("missing name must error")
	}
}

// TestParseFrontmatter_UnquotedColonAndQuotes covers the two real-data shapes
// a strict YAML mapping rejects: a plain (unquoted) description containing
// "word: word", and a quoted argument-hint whose surrounding quotes must be
// stripped.
func TestParseFrontmatter_UnquotedColonAndQuotes(t *testing.T) {
	b := []byte("---\nname: x\ndescription: Use before modifying: check first, then act.\nargument-hint: \"<slug>\"\n---\nbody\n")
	fm, err := ParseFrontmatter(b)
	if err != nil {
		t.Fatal(err)
	}
	if fm.Description != "Use before modifying: check first, then act." {
		t.Fatalf("description not preserved across unquoted colon: %q", fm.Description)
	}
	if fm.ArgumentHint != "<slug>" {
		t.Fatalf("argument-hint quotes not stripped: %q", fm.ArgumentHint)
	}
}

func TestGenSkills_PagesAndIndexes(t *testing.T) {
	znf := os.DirFS("testdata")
	files, err := GenSkills(znf, os.DirFS("testdata/empty-coding"))
	if err != nil {
		t.Fatal(err)
	}
	page := string(files["skills/demo.md"])
	for _, want := range []string{"title: /znf:demo", "`/znf:demo <slug>`", "Chỉ bạn gõ được"} { //znf:allow-lang
		if !strings.Contains(page, want) {
			t.Errorf("skills/demo.md missing %q\n%s", want, page)
		}
	}
	if strings.Contains(page, "Body must not be copied") {
		t.Fatal("body leaked into page")
	}
	if _, ok := files["skills/_shared.md"]; ok {
		t.Fatal("_shared is not a skill")
	}
	if strings.Contains(string(files["agents/scout.md"]), "sonnet") {
		t.Fatal("agent page must not expose the model: implementation detail")
	}
	if !strings.Contains(string(files["skills/index.md"]), "[/znf:demo](./demo)") {
		t.Fatal("skills index missing row")
	}
	if !strings.Contains(string(files["agents/index.md"]), "[scout](./scout)") {
		t.Fatal("agents index missing row")
	}
}

// TestGenSkills_RealEmbeddedTree exercises GenSkills against the real
// embedded assets (plugin.ZnfFS / plugin.CodingFS), not the synthetic
// fixtures — this is what caught the strict-YAML regression against real
// frontmatter containing an unquoted "key: value"-shaped description.
func TestGenSkills_RealEmbeddedTree(t *testing.T) {
	znf, coding := plugin.ZnfFS(), plugin.CodingFS()
	files, err := GenSkills(znf, coding)
	if err != nil {
		t.Fatal(err)
	}

	wantSkills := countSkillDirs(t, znf, "skills") + countSkillDirs(t, coding, ".")
	gotSkills := 0
	for rel := range files {
		if strings.HasPrefix(rel, "skills/") && rel != "skills/index.md" {
			gotSkills++
		}
	}
	if gotSkills != wantSkills {
		t.Fatalf("skills page count = %d, want %d (skills/*/SKILL.md on disk)", gotSkills, wantSkills)
	}

	agentEntries, err := fs.ReadDir(znf, "agents")
	if err != nil {
		t.Fatal(err)
	}
	wantAgents := 0
	for _, e := range agentEntries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			wantAgents++
		}
	}
	gotAgents := 0
	for rel := range files {
		if strings.HasPrefix(rel, "agents/") && rel != "agents/index.md" {
			gotAgents++
		}
	}
	if gotAgents != wantAgents {
		t.Fatalf("agent page count = %d, want %d (agents/*.md on disk)", gotAgents, wantAgents)
	}

	// Body must never leak: every "## " line a generated page carries must be
	// one of the generator's own section headers, never a heading pulled
	// from the source SKILL.md/agent body.
	allowedHeadings := map[string]bool{
		"## Cách gọi": true, //znf:allow-lang
	}
	for rel, content := range files {
		if rel == "skills/index.md" || rel == "agents/index.md" {
			continue
		}
		if len(content) == 0 {
			t.Fatalf("%s is empty", rel)
		}
		// Headings the catalog fragment itself carries are the generator's own.
		frag, _ := LoadFragment(strings.TrimSuffix(rel, ".md"))
		for _, line := range strings.Split(string(content), "\n") {
			if strings.HasPrefix(line, "## ") && !allowedHeadings[line] && !strings.Contains("\n"+frag.Body+"\n", "\n"+line+"\n") {
				t.Fatalf("%s: unexpected heading %q — looks like a leaked body heading", rel, line)
			}
		}
	}
}

// countSkillDirs counts <dir>/<name>/SKILL.md entries where <name> does not
// start with "_" (shared reference, not a skill) — mirrors eachSkill's rule.
func countSkillDirs(t *testing.T, fsys fs.FS, dir string) int {
	t.Helper()
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "_") {
			continue
		}
		p := e.Name() + "/SKILL.md"
		if dir != "." {
			p = dir + "/" + p
		}
		if _, err := fs.Stat(fsys, p); err == nil {
			n++
		}
	}
	return n
}

func TestGenHooks_Table(t *testing.T) {
	files := GenHooks([]apply.HookSpec{{Event: "Stop", Matcher: "", ID: "docs-sync", Purpose: "Đồng bộ knowledge store"}, {Event: "PreToolUse", Matcher: "Task|Agent", ID: "observe-count", Purpose: "Đếm subagent"}, {Event: "PostToolUse", Matcher: "", ID: "escape-check", Purpose: "Ghi <x>|log"}}) //znf:allow-lang
	h := string(files["hooks.md"])
	for _, want := range []string{"| Event | Matcher | Lệnh | Mục đích |", "| Stop | — | `zenify hooks-run docs-sync` | Đồng bộ knowledge store |", "| PreToolUse | `Task\\|Agent` | `zenify hooks-run observe-count` | Đếm subagent |", "| PostToolUse | — | `zenify hooks-run escape-check` | Ghi \\<x\\>\\|log |", "zenify git-guard"} { //znf:allow-lang
		if !strings.Contains(h, want) {
			t.Errorf("hooks.md missing %q\n%s", want, h)
		}
	}
}
