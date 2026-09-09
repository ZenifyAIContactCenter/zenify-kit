package cli

import (
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
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	claudeDir := filepath.Join(tmpHome, ".claude")
	if err := os.MkdirAll(claudeDir, 0o750); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
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

func TestGuardInstallUsesTempHOME(t *testing.T) {
	// Never touches the real ~/.claude/settings.json — HOME is redirected to
	// a throwaway TempDir for the duration of this test.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	claudeDir := filepath.Join(tmpHome, ".claude")
	if err := os.MkdirAll(claudeDir, 0o750); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
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
		t.Error("settings.json must contain the zenify git-guard command after install")
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
