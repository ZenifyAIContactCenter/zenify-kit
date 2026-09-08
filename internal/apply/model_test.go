package apply

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// settingsPath/writeSettings are defined in globalhooks_test.go (same package);
// they take a base dir + build <dir>/.claude/settings.json, which is exactly the
// workspace layout EnsureWorkspaceModel reads.

func readModel(t *testing.T, ws string) (string, bool) {
	t.Helper()
	raw, err := os.ReadFile(settingsPath(ws))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	m, ok := root["model"]
	if !ok {
		return "", false
	}
	var s string
	if err := json.Unmarshal(m, &s); err != nil {
		t.Fatalf("model not a string: %v", err)
	}
	return s, true
}

// Absent file => created with the default model pinned.
func TestEnsureWorkspaceModel_CreatesWhenAbsent(t *testing.T) {
	ws := t.TempDir()
	changed, err := EnsureWorkspaceModel(ws, false)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true on create")
	}
	got, ok := readModel(t, ws)
	if !ok || got != DefaultModel {
		t.Fatalf("model = %q (ok=%v), want %q", got, ok, DefaultModel)
	}
}

// Enforce: a different existing model is overwritten, and foreign keys survive.
func TestEnsureWorkspaceModel_OverridesDifferentAndPreservesForeign(t *testing.T) {
	ws := t.TempDir()
	writeSettings(t, ws, `{
  "model": "claude-sonnet-5",
  "hooks": {
    "PreToolUse": [
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "zenify git-guard"}]}
    ]
  }
}`)
	changed, err := EnsureWorkspaceModel(ws, false)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true when overriding a different model")
	}
	got, _ := readModel(t, ws)
	if got != DefaultModel {
		t.Fatalf("model = %q, want %q", got, DefaultModel)
	}
	raw, _ := os.ReadFile(settingsPath(ws))
	if !strings.Contains(string(raw), "zenify git-guard") {
		t.Fatal("foreign hooks subtree was dropped")
	}
}

// Idempotent: already pinned => no change, byte-identical.
func TestEnsureWorkspaceModel_IdempotentWhenAlreadyPinned(t *testing.T) {
	ws := t.TempDir()
	writeSettings(t, ws, `{"model":"`+DefaultModel+`","hooks":{}}`)
	before, _ := os.ReadFile(settingsPath(ws))
	changed, err := EnsureWorkspaceModel(ws, false)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if changed {
		t.Fatal("expected changed=false when already pinned")
	}
	after, _ := os.ReadFile(settingsPath(ws))
	if string(before) != string(after) {
		t.Fatalf("file mutated despite no change:\nbefore=%s\nafter=%s", before, after)
	}
}

// Malformed settings.json => skip (error), file left untouched.
func TestEnsureWorkspaceModel_FailOpenOnMalformed(t *testing.T) {
	ws := t.TempDir()
	writeSettings(t, ws, `{ this is not json `)
	before, _ := os.ReadFile(settingsPath(ws))
	changed, err := EnsureWorkspaceModel(ws, false)
	if err == nil {
		t.Fatal("expected an error on malformed settings.json")
	}
	if changed {
		t.Fatal("expected changed=false on malformed input")
	}
	after, _ := os.ReadFile(settingsPath(ws))
	if string(before) != string(after) {
		t.Fatal("malformed file was mutated")
	}
}

// dryRun reports the pending change but writes nothing.
func TestEnsureWorkspaceModel_DryRunNoWrite(t *testing.T) {
	ws := t.TempDir()
	changed, err := EnsureWorkspaceModel(ws, true)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true (pending) in dry-run")
	}
	if _, err := os.Stat(settingsPath(ws)); !os.IsNotExist(err) {
		t.Fatal("dry-run must not create settings.json")
	}
}
