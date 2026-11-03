package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
)

func TestCodexHooksCheck_Wired9(t *testing.T) {
	home := t.TempDir()
	if _, err := apply.EnsureCodexHooks(home, false); err != nil {
		t.Fatal(err)
	}
	ok, d := codexHooksCheck(home, true).Run()
	if !ok || !strings.Contains(d, "wired 9/9") || strings.HasPrefix(d, "cảnh báo:") { //znf:allow-lang
		t.Fatalf("ok=%v %q", ok, d)
	}
	if !strings.Contains(d, "trust: không kiểm tra được, xem /hooks") { //znf:allow-lang
		t.Fatalf("no trust line: %q", d)
	}
}

func TestCodexHooksCheck_FiveWarns(t *testing.T) {
	home := t.TempDir()
	var hs []string
	for i := 0; i < 5; i++ {
		hs = append(hs, `{"command":"zenify hooks-run x`+fmt.Sprint(i)+`"}`)
	}
	body := `{"hooks":{"PreToolUse":[{"hooks":[` + strings.Join(hs, ",") + `]}]}}`
	if err := os.WriteFile(filepath.Join(home, "hooks.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, d := codexHooksCheck(home, true).Run()
	if !ok || !strings.HasPrefix(d, "cảnh báo:") || !strings.Contains(d, "wired 5/9") { //znf:allow-lang
		t.Fatalf("ok=%v %q", ok, d)
	}
}

func TestCodexHooksCheck_NoFile(t *testing.T) {
	_, d := codexHooksCheck(t.TempDir(), true).Run()
	if !strings.Contains(d, "wired 0/9") {
		t.Fatal(d)
	}
}

func TestCodexHooksCheck_NoCodex(t *testing.T) {
	ok, d := codexHooksCheck(t.TempDir(), false).Run()
	if !ok || d != "codex không cài" { //znf:allow-lang
		t.Fatalf("%v %q", ok, d)
	}
}

func TestReviewerCheck_CodexMissingFromPath(t *testing.T) {
	env := func(k string) string {
		if k == "ZNF_REVIEWER" {
			return "codex"
		}
		return ""
	}
	ok, d := reviewerCheck(env, false).Run()
	if !ok || !strings.Contains(d, "agent (codex không có trên PATH)") { //znf:allow-lang
		t.Fatalf("%v %q", ok, d)
	}
}

func TestAgentsMdCheck(t *testing.T) {
	ws := t.TempDir()
	for _, r := range []string{"a", "b", "c"} {
		if err := os.MkdirAll(filepath.Join(ws, "repos", r), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(ws, "repos", "a", "AGENTS.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(ws, "repos", "c", "AGENTS.md"), make([]byte, 33*1024), 0o644)
	ok, d := agentsMdCheck(ws, []string{"repos/a", "repos/b", "repos/c"}).Run()
	if !ok || !strings.HasPrefix(d, "cảnh báo:") { //znf:allow-lang
		t.Fatalf("%v %q", ok, d)
	}
	for _, l := range []string{"repos/a/AGENTS.md: có", "repos/b/AGENTS.md: thiếu", "repos/c/AGENTS.md: vượt 32 KiB"} { //znf:allow-lang
		if !strings.Contains(d, l) {
			t.Fatalf("missing %q in %q", l, d)
		}
	}
	if ok, _ := agentsMdCheck("", nil).Run(); ok {
		t.Fatal("no workspace must be not-ok")
	}
}
