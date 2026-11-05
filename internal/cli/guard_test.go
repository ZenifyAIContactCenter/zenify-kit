package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureGuardHookAddsEntry(t *testing.T) {
	in := `{"hooks":{"PreToolUse":[]}}`
	out, changed, err := ensureGuardHook([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("must report changed when adding an entry")
	}
	if !strings.Contains(string(out), "zenify git-guard") {
		t.Error("must insert command 'zenify git-guard'")
	}
}

func TestEnsureGuardHookReplacesBash(t *testing.T) {
	in := `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"~/.claude/hooks/guard-git-deploy.sh"}]}]}}`
	out, changed, _ := ensureGuardHook([]byte(in))
	if !changed {
		t.Error("must replace the old bash entry")
	}
	if strings.Contains(string(out), "guard-git-deploy.sh") {
		t.Error("must not still reference the bash guard")
	}
	if !strings.Contains(string(out), "zenify git-guard") {
		t.Error("must point at zenify git-guard")
	}
}

func TestEnsureGuardHookIdempotent(t *testing.T) {
	in := `{"hooks":{"PreToolUse":[]}}`
	once, _, _ := ensureGuardHook([]byte(in))
	_, changed, err := ensureGuardHook(once)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("second pass must be idempotent (changed=false)")
	}
}

func TestEnsureGuardHookPreservesOthers(t *testing.T) {
	in := `{"model":"x","hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"~/.claude/hooks/other.sh"}]}]}}`
	out, _, _ := ensureGuardHook([]byte(in))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["model"] != "x" {
		t.Error("must preserve other fields (model)")
	}
	if !strings.Contains(string(out), "other.sh") {
		t.Error("must preserve the other hook")
	}
}

func TestEnsureGuardHookBrokenJSON(t *testing.T) {
	if _, _, err := ensureGuardHook([]byte("{not json")); err == nil {
		t.Error("broken JSON must return an error, not overwrite")
	}
}

func TestEnsureGuardHookSplicesLegacyKeepsSibling(t *testing.T) {
	in := `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"~/.claude/hooks/guard-git-deploy.sh"},{"type":"command","command":"some-other-hook"}]}]}}`
	out, changed, err := ensureGuardHook([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("must report changed when dropping the legacy hook")
	}
	s := string(out)
	if !strings.Contains(s, "some-other-hook") {
		t.Error("the sibling hook (some-other-hook) in the same entry must be preserved")
	}
	if strings.Contains(s, "guard-git-deploy.sh") {
		t.Error("the legacy hook must be removed")
	}
	if !strings.Contains(s, "zenify git-guard") {
		t.Error("must point at zenify git-guard")
	}

	// Second pass must be idempotent now that the guard is present.
	_, changed2, err := ensureGuardHook(out)
	if err != nil {
		t.Fatal(err)
	}
	if changed2 {
		t.Error("second pass must be idempotent (changed=false)")
	}
}

// --- Extra coverage beyond the brief's minimum ---

func TestEnsureGuardHookEmptyInput(t *testing.T) {
	out, changed, err := ensureGuardHook(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("empty input must create a new hook → changed=true")
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("output must be valid JSON: %v", err)
	}
	if !strings.Contains(string(out), "zenify git-guard") {
		t.Error("must insert command 'zenify git-guard'")
	}
}

func TestEnsureGuardHookPreservesMcpServersAndOtherMatchers(t *testing.T) {
	in := `{
		"mcpServers": {"foo": {"command": "bar"}},
		"enabledPlugins": {"some-plugin": true},
		"hooks": {
			"PreToolUse": [
				{"matcher": "Write", "hooks": [{"type": "command", "command": "some-other-hook"}]}
			],
			"PostToolUse": [
				{"matcher": "*", "hooks": [{"type": "command", "command": "post-hook"}]}
			]
		}
	}`
	out, changed, err := ensureGuardHook([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("must add a new entry")
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["mcpServers"]; !ok {
		t.Error("must preserve mcpServers")
	}
	if _, ok := m["enabledPlugins"]; !ok {
		t.Error("must preserve enabledPlugins")
	}
	if !strings.Contains(string(out), "some-other-hook") {
		t.Error("must preserve the other Write matcher")
	}
	if !strings.Contains(string(out), "post-hook") {
		t.Error("must preserve PostToolUse")
	}
	if !strings.Contains(string(out), "zenify git-guard") {
		t.Error("must insert command 'zenify git-guard'")
	}

	// Second pass must be idempotent, and everything must still be intact.
	out2, changed2, err := ensureGuardHook(out)
	if err != nil {
		t.Fatal(err)
	}
	if changed2 {
		t.Error("second pass must be idempotent (changed=false)")
	}
	if !strings.Contains(string(out2), "mcpServers") || !strings.Contains(string(out2), "enabledPlugins") {
		t.Error("must preserve mcpServers/enabledPlugins after the second pass")
	}
	if !strings.Contains(string(out2), "some-other-hook") || !strings.Contains(string(out2), "post-hook") {
		t.Error("must preserve the other hooks after the second pass")
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	data := []byte(`{"a":1}`)
	if err := writeFileAtomic(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Errorf("got %q, want %q", got, data)
	}
	// Overwrite an existing file.
	data2 := []byte(`{"a":2}`)
	if err := writeFileAtomic(path, data2, 0o644); err != nil {
		t.Fatal(err)
	}
	got2, err := os.ReadFile(path) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if err != nil {
		t.Fatal(err)
	}
	if string(got2) != string(data2) {
		t.Errorf("got %q, want %q", got2, data2)
	}
	// No leftover temp files.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("expected exactly 1 file in dir, got %d: %v", len(entries), entries)
	}
}

func TestGuardInstallPreservesExistingFileMode(t *testing.T) {
	// A pre-existing settings.json with a restrictive mode (0o600) must keep
	// that mode after install — install must never silently widen it to the
	// 0o644 default used for a brand-new file.
	t.Setenv("HOME", t.TempDir())
	settingsPath := guardWorkspace(t)
	if err := os.WriteFile(settingsPath, []byte(`{"hooks":{"PreToolUse":[]}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := newGuardCmd()
	cmd.SetArgs([]string{"install"})
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	fi, err := os.Stat(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode after install = %o, want %o (preserved)", fi.Mode().Perm(), 0o600)
	}
}

// guardWorkspace makes a temp workspace (marker included), chdirs into it so
// `guard install` resolves it, and returns its settings.local.json path.
func guardWorkspace(t *testing.T) string {
	t.Helper()
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".zenify"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".zenify", "manifest.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(ws, ".claude"), 0o750); err != nil {
		t.Fatal(err)
	}
	t.Chdir(ws)
	return filepath.Join(ws, ".claude", "settings.local.json")
}

func TestGuardInstallUsesTempHOME(t *testing.T) {
	// guard install writes the resolved workspace's settings.local.json, never
	// the real ~/.claude/settings.json (HOME is a throwaway TempDir too).
	t.Setenv("HOME", t.TempDir())
	settingsPath := guardWorkspace(t)
	if err := os.WriteFile(settingsPath, []byte(`{"hooks":{"PreToolUse":[]}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := newGuardCmd()
	cmd.SetArgs([]string{"install"})
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(settingsPath) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "zenify git-guard") {
		t.Error("settings.local.json must contain the zenify git-guard command after install")
	}

	// Second run: idempotent, should report "already present".
	cmd2 := newGuardCmd()
	cmd2.SetArgs([]string{"install"})
	var out2 strings.Builder
	cmd2.SetOut(&out2)
	cmd2.SetErr(&out2)
	if err := cmd2.Execute(); err != nil {
		t.Fatal(err)
	}
}

// installGuard runs on every up/SessionStart, so it must not reformat the
// user's file: foreign values keep their inner key order (whitespace is
// re-indented, as in apply) and && is not
// HTML-escaped to &.
func TestEnsureGuardHookPreservesForeignOrderAndNoEscape(t *testing.T) {
	in := []byte(`{"permissions":{"zeta":1,"alpha":2},"hooks":{"Stop":[{"matcher":"","hooks":[{"type":"command","command":"a && b"}]}]}}`)
	out, changed, err := ensureGuardHook(in)
	if err != nil || !changed {
		t.Fatalf("want changed, got changed=%v err=%v", changed, err)
	}
	s := string(out)
	if z, a := strings.Index(s, `"zeta"`), strings.Index(s, `"alpha"`); z < 0 || a < 0 || z > a {
		t.Errorf("foreign permissions keys reordered:\n%s", s)
	}
	if !strings.Contains(s, "a && b") || strings.Contains(s, `\u0026`) {
		t.Errorf("hook command HTML-escaped:\n%s", s)
	}
}

func TestEnsureGuardHookRefusesNonObjectHooks(t *testing.T) {
	for _, in := range []string{`{"hooks":"x"}`, `{"hooks":null}`, `{"hooks":{"PreToolUse":{}}}`} {
		if _, _, err := ensureGuardHook([]byte(in)); err == nil {
			t.Errorf("%s: want error (do not replace user data), got nil", in)
		}
	}
}

func TestEnsureGuardHookNullDocumentNoPanic(t *testing.T) {
	out, changed, err := ensureGuardHook([]byte("null"))
	if err != nil || !changed || !strings.Contains(string(out), guardCommand) {
		t.Fatalf("null document: want guard added, got changed=%v err=%v out=%s", changed, err, out)
	}
}

// A leftover legacy guard beside the real one must not make doctor claim
// the hook is missing.
func TestGuardInstalledWithLegacySibling(t *testing.T) {
	home := t.TempDir()
	p := filepath.Join(home, ".claude", "settings.json")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte(`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"bash guard-git-deploy.sh"},{"type":"command","command":"zenify git-guard"}]}]}}`), 0o644)
	if ok, err := guardInstalled(guardSettingsPath(home)); err != nil || !ok {
		t.Fatalf("want installed=true, got %v err=%v", ok, err)
	}
}

// removeGuard is the inverse of installGuard: it drops the guard (and a
// legacy guard) but keeps sibling hooks and foreign settings; dryRun writes
// nothing.
func TestRemoveGuard_KeepsSiblingsAndDryRunWritesNothing(t *testing.T) {
	home := t.TempDir()
	p := filepath.Join(home, ".claude", "settings.json")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	body := []byte(`{"permissions":{"zeta":1,"alpha":2},"hooks":{"PreToolUse":[` +
		`{"matcher":"Bash","hooks":[{"type":"command","command":"zenify git-guard"}]},` +
		`{"matcher":"Read","hooks":[{"type":"command","command":"zenify hooks-run read-guard"},{"type":"command","command":"bash guard-git-deploy.sh"}]}]}}`)
	_ = os.WriteFile(p, body, 0o644)

	if removed, err := removeGuard(guardSettingsPath(home), true); err != nil || !removed {
		t.Fatalf("dry-run: want removed=true, got %v err=%v", removed, err)
	}
	if b, _ := os.ReadFile(p); !bytes.Equal(b, body) { //nolint:gosec // G304 -- test-local path under t.TempDir
		t.Fatal("dry-run must not write")
	}

	if removed, err := removeGuard(guardSettingsPath(home), false); err != nil || !removed {
		t.Fatalf("apply: want removed=true, got %v err=%v", removed, err)
	}
	b, _ := os.ReadFile(p) //nolint:gosec // G304 -- test-local path under t.TempDir
	s := string(b)
	if strings.Contains(s, guardCommand) || strings.Contains(s, legacyGuard) {
		t.Errorf("guard hooks still present:\n%s", s)
	}
	if !strings.Contains(s, "zenify hooks-run read-guard") {
		t.Errorf("sibling read-guard hook was dropped:\n%s", s)
	}
	if z, a := strings.Index(s, `"zeta"`), strings.Index(s, `"alpha"`); z < 0 || z > a {
		t.Errorf("foreign permissions reordered:\n%s", s)
	}
	if ok, _ := guardInstalled(guardSettingsPath(home)); ok {
		t.Error("guardInstalled must be false after removeGuard")
	}
	if removed, err := removeGuard(guardSettingsPath(home), false); err != nil || removed {
		t.Fatalf("second remove: want removed=false, got %v err=%v", removed, err)
	}
}

func TestInstallGuardOnLocalSettingsKeepsForeignKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{"autoMemoryDirectory":"/x","enabledPlugins":{"a@b":true},"env":{"FOO":"1"},"hooks":{"Stop":[{"hooks":[{"type":"command","command":"my-own.sh"}]}]}}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if changed, err := installGuard(path); err != nil || !changed {
		t.Fatalf("installGuard changed=%v err=%v", changed, err)
	}
	if ok, err := guardInstalled(path); err != nil || !ok {
		t.Fatalf("guardInstalled ok=%v err=%v", ok, err)
	}
	if changed, err := removeGuard(path, false); err != nil || !changed {
		t.Fatalf("removeGuard changed=%v err=%v", changed, err)
	}
	b, _ := os.ReadFile(path)
	s := string(b)
	if strings.Contains(s, "zenify git-guard") {
		t.Fatalf("guard remains: %s", s)
	}
	for _, want := range []string{`"autoMemoryDirectory"`, `"a@b"`, `"FOO"`, `my-own.sh`} {
		if !strings.Contains(s, want) {
			t.Fatalf("lost %s: %s", want, s)
		}
	}
}
