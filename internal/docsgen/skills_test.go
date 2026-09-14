package docsgen

import (
	"os"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
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

func TestGenSkills_PagesAndIndexes(t *testing.T) {
	znf := os.DirFS("testdata")
	files, err := GenSkills(znf, os.DirFS("testdata/empty-coding"))
	if err != nil {
		t.Fatal(err)
	}
	page := string(files["skills/demo.md"])
	for _, want := range []string{"title: /znf:demo", "`/znf:demo <slug>`", "Chỉ user gọi", "Read Bash"} { //znf:allow-lang
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
	if !strings.Contains(string(files["agents/scout.md"]), "sonnet") {
		t.Fatal("agent page missing model")
	}
	if !strings.Contains(string(files["skills/index.md"]), "[/znf:demo](./demo)") {
		t.Fatal("skills index missing row")
	}
	if !strings.Contains(string(files["agents/index.md"]), "[scout](./scout)") {
		t.Fatal("agents index missing row")
	}
}

func TestGenHooks_Table(t *testing.T) {
	files := GenHooks([]apply.HookSpec{{Event: "Stop", Matcher: "", ID: "docs-sync"}, {Event: "PreToolUse", Matcher: "Task|Agent", ID: "observe-count"}})
	h := string(files["hooks.md"])
	for _, want := range []string{"| Stop | — | `zenify hooks-run docs-sync` |", "| PreToolUse | `Task\\|Agent` | `zenify hooks-run observe-count` |", "zenify git-guard"} {
		if !strings.Contains(h, want) {
			t.Errorf("hooks.md missing %q\n%s", want, h)
		}
	}
}
