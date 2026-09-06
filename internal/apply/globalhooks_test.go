package apply

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func settingsPath(home string) string {
	return filepath.Join(home, ".claude", "settings.json")
}

func writeSettings(t *testing.T, home, body string) {
	t.Helper()
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath(home), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// SC-3: existing user/orca/git-guard hooks are byte-preserved; only znf entries added.
func TestEnsureGlobalHooks_PreservesForeignHooks(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, `{
  "hooks": {
    "PreToolUse": [
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "zenify git-guard"}]}
    ]
  }
}`)
	if _, err := ensureGlobalHooks(home, false); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	raw, _ := os.ReadFile(settingsPath(home))
	s := string(raw)
	if !strings.Contains(s, "zenify git-guard") {
		t.Fatal("git-guard hook was dropped")
	}
	if !strings.Contains(s, "zenify hooks-run docs-sync") {
		t.Fatal("znf docs-sync hook not injected")
	}
	if !strings.Contains(s, "zenify hooks-run session-start") {
		t.Fatal("znf session-start hook not injected")
	}
}

// SC-4: second run with nothing changed => 0 changes, byte-identical.
func TestEnsureGlobalHooks_Idempotent(t *testing.T) {
	home := t.TempDir()
	if _, err := ensureGlobalHooks(home, false); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(settingsPath(home))
	ch, err := ensureGlobalHooks(home, false)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(settingsPath(home))
	if string(first) != string(second) {
		t.Fatal("second run changed the file (not idempotent)")
	}
	if ch.Added != 0 || ch.Updated != 0 {
		t.Fatalf("second run reported changes: %+v", ch)
	}
}

// SC-5: malformed settings => skip + no overwrite, return Skipped.
func TestEnsureGlobalHooks_MalformedSkips(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, `{ this is not json `)
	ch, err := ensureGlobalHooks(home, false)
	if err == nil {
		t.Fatal("expected error on malformed settings")
	}
	if !ch.Skipped {
		t.Fatal("expected Skipped=true")
	}
	raw, _ := os.ReadFile(settingsPath(home))
	if string(raw) != `{ this is not json ` {
		t.Fatal("malformed file was overwritten")
	}
}

// FR-4.2: entry with marker but stale command => replaced, not duplicated.
func TestEnsureGlobalHooks_ReplacesStaleMarker(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, `{
  "hooks": {
    "Stop": [
      {"hooks": [{"type": "command", "command": "zenify hooks-run docs-sync --OLD"}]}
    ]
  }
}`)
	ch, err := ensureGlobalHooks(home, false)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(settingsPath(home))
	if strings.Contains(string(raw), "--OLD") {
		t.Fatal("stale marked command not replaced")
	}
	if strings.Count(string(raw), "zenify hooks-run docs-sync") != 1 {
		t.Fatal("docs-sync duplicated instead of replaced")
	}
	if ch.Updated == 0 {
		t.Fatal("expected Updated>0")
	}
}

// FR-4.4: byte-write must not HTML-escape matcher special chars.
func TestEnsureGlobalHooks_NoHTMLEscape(t *testing.T) {
	home := t.TempDir()
	if _, err := ensureGlobalHooks(home, false); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(settingsPath(home))
	if strings.Contains(string(raw), `&`) {
		t.Fatal("ampersand HTML-escaped; use Encoder.SetEscapeHTML(false)")
	}
	// PostToolUse matcher uses '|' which is safe, but assert file parses back.
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("output not valid json: %v", err)
	}
}

// FR-4.3 (part): dryRun does not write.
func TestEnsureGlobalHooks_DryRunNoWrite(t *testing.T) {
	home := t.TempDir()
	ch, err := ensureGlobalHooks(home, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(settingsPath(home)); !os.IsNotExist(statErr) {
		t.Fatal("dryRun created/modified settings.json")
	}
	if ch.Added == 0 {
		t.Fatal("dryRun should still report planned Added>0")
	}
}

// Review fix round 1, finding 1 (Important): foreign top-level settings values
// must never be edited, reordered, or dropped — only the "hooks" subtree is
// decoded/normalized. The seed below is already in the canonical 2-space
// indent that marshalNoEscape produces, so the "permissions" block's bytes
// must survive verbatim in the output.
func TestEnsureGlobalHooks_PreservesForeignSettingsBytes(t *testing.T) {
	home := t.TempDir()
	permissionsBlock := `"permissions": {
    "allow": [
      "Bash(ls:*)",
      "Read"
    ],
    "deny": []
  }`
	writeSettings(t, home, `{
  "hooks": {},
  `+permissionsBlock+`
}`)
	if _, err := ensureGlobalHooks(home, false); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	raw, _ := os.ReadFile(settingsPath(home))
	if !strings.Contains(string(raw), permissionsBlock) {
		t.Fatalf("foreign \"permissions\" block was reformatted or dropped; got:\n%s", raw)
	}
}

// Review fix round 1, finding 2 (Minor): file mode of a pre-existing
// settings.json must be preserved across a write, not silently narrowed to
// os.CreateTemp's default 0600.
func TestEnsureGlobalHooks_PreservesFileMode(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, `{}`)
	if err := os.Chmod(settingsPath(home), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureGlobalHooks(home, false); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	fi, err := os.Stat(settingsPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o644 {
		t.Fatalf("file mode changed: got %o, want %o", fi.Mode().Perm(), 0o644)
	}
}

// Review fix round 1, finding 3 (Minor): a non-object "hooks" value (string,
// array, number, null) must never be replaced or dropped — fail-open the
// same way as malformed JSON.
func TestEnsureGlobalHooks_NonObjectHooksSkips(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, `{"hooks": "not-an-object"}`)
	ch, err := ensureGlobalHooks(home, false)
	if err == nil {
		t.Fatal("expected error on non-object \"hooks\"")
	}
	if !ch.Skipped {
		t.Fatal("expected Skipped=true")
	}
	raw, _ := os.ReadFile(settingsPath(home))
	if string(raw) != `{"hooks": "not-an-object"}` {
		t.Fatal("settings.json was overwritten despite non-object \"hooks\"")
	}
}
