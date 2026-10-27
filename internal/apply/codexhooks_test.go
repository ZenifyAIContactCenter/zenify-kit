package apply

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexEntriesDropsReadGuardAndMapsMatchers(t *testing.T) {
	got := codexEntries(znfHookSpecs())
	if len(got) != 7 {
		t.Fatalf("want 7 entries, got %d", len(got))
	}
	for _, s := range got {
		if s.ID == "read-guard" {
			t.Fatal("read-guard must be dropped")
		}
		if strings.Contains(s.Matcher, "Read") || strings.Contains(s.Matcher, "Task") || strings.Contains(s.Matcher, "Agent") {
			t.Fatalf("matcher %q still names a Claude tool", s.Matcher)
		}
	}
	byID := map[string]hookSpec{}
	for _, s := range got {
		byID[s.ID] = s
	}
	if byID["observe-count"].Matcher != codexSubagentTool {
		t.Fatalf("observe-count matcher %q", byID["observe-count"].Matcher)
	}
	if byID["observe-meter"].Matcher != codexSubagentTool+"|Bash" {
		t.Fatalf("observe-meter matcher %q", byID["observe-meter"].Matcher)
	}
	if byID["docs-sync"].command() != "zenify hooks-run docs-sync" {
		t.Fatal("command must be unchanged")
	}
}

func TestHookSpecRuntimes(t *testing.T) {
	if got := (hookSpec{ID: "read-guard"}).Runtimes(); got != "Claude" {
		t.Fatalf("read-guard runtimes = %q", got)
	}
	if got := (hookSpec{ID: "docs-sync"}).Runtimes(); got != "Claude, Codex" {
		t.Fatalf("docs-sync runtimes = %q", got)
	}
	if n := len(CodexHookSpecs()); n != 7 {
		t.Fatalf("CodexHookSpecs len = %d", n)
	}
}

func codexHooksPath(dir string) string { return filepath.Join(dir, "hooks.json") }

func writeCodexFixture(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(codexHooksPath(dir), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const codexForeignFixture = `{"foo":{"a":1},"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"/me/own.sh"}]}]}}`

func TestEnsureCodexHooksCreatesThenNoop(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "codex")
	ch, err := EnsureCodexHooks(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if ch.Added != 8 {
		t.Fatalf("Added = %d, want 8", ch.Added)
	}
	first, err := os.ReadFile(codexHooksPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	ch, err = EnsureCodexHooks(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if ch.Added != 0 || ch.Updated != 0 || ch.Unchanged != 8 {
		t.Fatalf("second run = %+v, want only Unchanged=8", ch)
	}
	second, _ := os.ReadFile(codexHooksPath(dir))
	if string(first) != string(second) {
		t.Fatal("file changed on no-op run")
	}
}

func TestEnsureCodexHooksKeepsForeignEntries(t *testing.T) {
	dir := t.TempDir()
	writeCodexFixture(t, dir, codexForeignFixture)
	if _, err := EnsureCodexHooks(dir, false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(codexHooksPath(dir))
	var root map[string]json.RawMessage
	if err := json.Unmarshal(b, &root); err != nil {
		t.Fatal(err)
	}
	assertFoo(t, root["foo"])
	var hooks map[string][]struct {
		Matcher string `json:"matcher"`
		Hooks   []struct {
			Command string `json:"command"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(root["hooks"], &hooks); err != nil {
		t.Fatal(err)
	}
	var bash []string
	for _, g := range hooks["PreToolUse"] {
		if g.Matcher == "Bash" {
			for _, h := range g.Hooks {
				bash = append(bash, h.Command)
			}
		}
	}
	if len(bash) != 2 || bash[0] != "/me/own.sh" || bash[1] != codexGuardCommand {
		t.Fatalf("Bash group = %v", bash)
	}
}

func TestEnsureCodexHooksMalformedFailOpen(t *testing.T) {
	for _, body := range []string{`{"hooks":null}`, `not json`} {
		dir := t.TempDir()
		writeCodexFixture(t, dir, body)
		ch, err := EnsureCodexHooks(dir, false)
		if err == nil || !ch.Skipped {
			t.Fatalf("%q: err=%v skipped=%v", body, err, ch.Skipped)
		}
		b, _ := os.ReadFile(codexHooksPath(dir))
		if string(b) != body {
			t.Fatalf("%q: file changed to %q", body, b)
		}
	}
}

func TestEnsureCodexHooksMigratesGitGuardMatcher(t *testing.T) {
	dir := t.TempDir()
	writeCodexFixture(t, dir, `{"hooks":{"PreToolUse":[{"matcher":"Edit","hooks":[{"type":"command","command":"zenify git-guard"}]}]}}`)
	if _, err := EnsureCodexHooks(dir, false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(codexHooksPath(dir))
	var root struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(b, &root); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, g := range root.Hooks["PreToolUse"] {
		for _, h := range g.Hooks {
			if h.Command == codexGuardCommand {
				n++
				if g.Matcher != "Bash" {
					t.Fatalf("git-guard under matcher %q", g.Matcher)
				}
			}
		}
	}
	if n != 1 {
		t.Fatalf("git-guard entries = %d, want 1", n)
	}
}

func TestEnsureCodexHooksDryRunWritesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "codex")
	ch, err := EnsureCodexHooks(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if ch.Added != 8 {
		t.Fatalf("Added = %d, want 8", ch.Added)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("dry-run created %s (err=%v)", dir, err)
	}
}

func TestRemoveCodexHooksKeepsForeign(t *testing.T) {
	dir := t.TempDir()
	writeCodexFixture(t, dir, codexForeignFixture)
	if _, err := EnsureCodexHooks(dir, false); err != nil {
		t.Fatal(err)
	}
	r, err := RemoveCodexHooks(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if r.Removed != 8 {
		t.Fatalf("Removed = %d, want 8", r.Removed)
	}
	b, _ := os.ReadFile(codexHooksPath(dir))
	var root map[string]json.RawMessage
	if err := json.Unmarshal(b, &root); err != nil {
		t.Fatal(err)
	}
	assertFoo(t, root["foo"])
	var hooks map[string][]map[string]any
	if err := json.Unmarshal(root["hooks"], &hooks); err != nil {
		t.Fatal(err)
	}
	if len(hooks) != 1 || len(hooks["PreToolUse"]) != 1 {
		t.Fatalf("hooks = %s", root["hooks"])
	}
	inner := hooks["PreToolUse"][0]["hooks"].([]any)
	if len(inner) != 1 || inner[0].(map[string]any)["command"] != "/me/own.sh" {
		t.Fatalf("inner = %v", inner)
	}
}

func TestRemoveCodexHooksDeletesEmptyFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := EnsureCodexHooks(dir, false); err != nil {
		t.Fatal(err)
	}
	if _, err := RemoveCodexHooks(dir, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(codexHooksPath(dir)); !os.IsNotExist(err) {
		t.Fatalf("file still exists (err=%v)", err)
	}
}

func TestRemoveCodexHooksMissingFile(t *testing.T) {
	r, err := RemoveCodexHooks(t.TempDir(), false)
	if err != nil || r != (HookRemoval{}) {
		t.Fatalf("r=%+v err=%v", r, err)
	}
}

// assertFoo checks the foreign value survived content-identical; the writer
// re-indents the file (same as EnsureGlobalHooks), so compare compacted.
func assertFoo(t *testing.T, raw json.RawMessage) {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil || buf.String() != `{"a":1}` {
		t.Fatalf("foo = %s (err=%v)", raw, err)
	}
}

func TestCodexEntries_NoHubReport(t *testing.T) {
	for _, s := range codexEntries(znfHookSpecs()) {
		if s.ID == "hub-report" {
			t.Fatalf("codex entries contain hub-report for event %s", s.Event)
		}
	}
}
