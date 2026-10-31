package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func wsW(p string) (string, bool) {
	if p == "/w" || strings.HasPrefix(p, "/w/") {
		return "/w", true
	}
	return "", false
}

func TestWriteGuard_Table(t *testing.T) { // SC-12 + Review Focus 1
	cases := []struct {
		name, payload string
		want          int
	}{
		{"png-root", `{"tool_name":"Write","cwd":"/w","tool_input":{"file_path":"/w/check1.png"}}`, 2},
		{"passwords", `{"tool_name":"Write","tool_input":{"file_path":"/w/passwords/a.txt"}}`, 2},
		{"docs-file", `{"tool_name":"Write","tool_input":{"file_path":"/w/docs/note.md"}}`, 2},
		{"docs-readme", `{"tool_name":"Edit","tool_input":{"file_path":"/w/docs/README.md"}}`, 2},
		{"docs-handoffs", `{"tool_name":"Write","tool_input":{"file_path":"/w/docs/handoffs/x.md"}}`, 2},
		{"exports", `{"tool_name":"Write","tool_input":{"file_path":"/w/docs/exports/report.docx"}}`, 0},
		{"external", `{"tool_name":"Write","tool_input":{"file_path":"/w/docs/external/brief.pdf"}}`, 0},
		{"spec", `{"tool_name":"Write","tool_input":{"file_path":"/w/docs/specs/r/x-design.md"}}`, 0},
		{"docs-dot", `{"tool_name":"Write","tool_input":{"file_path":"/w/docs/.claude/x"}}`, 0},
		{"repos", `{"tool_name":"Write","tool_input":{"file_path":"/w/repos/r/anything/deep.js"}}`, 0},
		{"claude-md", `{"tool_name":"Edit","tool_input":{"file_path":"/w/CLAUDE.md"}}`, 0},
		{"dialogue", `{"tool_name":"Write","tool_input":{"file_path":"/w/.zenify/dialogue.md"}}`, 0},
		{"memory", `{"tool_name":"Write","tool_input":{"file_path":"/w/.claude/memory/x.md"}}`, 0},
		{"outside", `{"tool_name":"Write","tool_input":{"file_path":"/tmp/claude/scratch/x"}}`, 0},
		{"no-path", `{"tool_name":"Write","tool_input":{}}`, 0},
		{"relative", `{"tool_name":"Write","cwd":"/w","tool_input":{"file_path":"check.png"}}`, 2},
		{"notebook", `{"tool_name":"NotebookEdit","cwd":"/w","tool_input":{"notebook_path":"/w/nb.ipynb"}}`, 2},
		{"dotdot-escape", `{"tool_name":"Write","tool_input":{"file_path":"/w/../x.png"}}`, 0},
		{"dotdot-prefixed-entry", `{"tool_name":"Write","tool_input":{"file_path":"/w/..foo"}}`, 2},
		{"bad-json", `{`, 0},
	}
	for _, c := range cases {
		var e bytes.Buffer
		got := runWriteGuard(strings.NewReader(c.payload), &e, wsW)
		if got != c.want {
			t.Errorf("%s: code %d want %d (%s)", c.name, got, c.want, e.String())
		}
		if c.want == 2 && (!strings.Contains(e.String(), "docs/exports/") || !strings.Contains(e.String(), "write-guard: /w/")) {
			t.Errorf("%s: message %q", c.name, e.String())
		}
		if c.want == 0 && e.Len() != 0 {
			t.Errorf("%s: allow must be silent, got %q", c.name, e.String())
		}
	}
}

// Repo roots come from the manifest/overlay: a repo placed outside repos/ must stay writable.
func TestWriteGuard_OverlayRepoRoot(t *testing.T) {
	ws := t.TempDir()
	writeF(t, filepath.Join(ws, ".zenify-overlay.yaml"), "repos:\n  - name: contact-center-be\n    path: elsewhere/a\n")
	find := func(string) (string, bool) { return ws, true }
	var e bytes.Buffer
	payload := `{"tool_name":"Write","tool_input":{"file_path":"` + filepath.Join(ws, "elsewhere", "a", "x.go") + `"}}`
	if got := runWriteGuard(strings.NewReader(payload), &e, find); got != 0 {
		t.Fatalf("overlay repo root denied: %d %s", got, e.String())
	}
	payload = `{"tool_name":"Write","tool_input":{"file_path":"` + filepath.Join(ws, "other", "x.go") + `"}}`
	if got := runWriteGuard(strings.NewReader(payload), &e, find); got != 2 {
		t.Fatalf("non-repo root must still be denied: %d", got)
	}
}
