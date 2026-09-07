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
	if _, err := EnsureGlobalHooks(home, false); err != nil {
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
	if _, err := EnsureGlobalHooks(home, false); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(settingsPath(home))
	ch, err := EnsureGlobalHooks(home, false)
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
	ch, err := EnsureGlobalHooks(home, false)
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
	ch, err := EnsureGlobalHooks(home, false)
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
	if _, err := EnsureGlobalHooks(home, false); err != nil {
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
	ch, err := EnsureGlobalHooks(home, true)
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

// Review fix round 1, finding 1 (Important) / round 2 rewrite: foreign
// top-level settings values must never be edited, reordered, or dropped —
// only the "hooks" subtree is decoded/normalized. An earlier version of this
// test seeded a "permissions" block that was already in canonical,
// alphabetically-sorted 2-space form, so it passed even against a
// map[string]any round-trip (which alphabetizes keys) — it asserted nothing
// that actually distinguishes the two approaches. This version seeds a
// foreign object with deliberately NON-alphabetical key order (zebra, alpha,
// mango) plus a nested array, and checks both content and order survive.
// Reverting EnsureGlobalHooks to decode/re-encode the whole root as
// map[string]any (instead of keeping foreign top-level values as
// json.RawMessage) would re-sort these keys to alpha, mango, zebra on
// marshal and break this test.
func TestEnsureGlobalHooks_PreservesForeignValueContentAndKeyOrder(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, `{
  "hooks": {},
  "permissions": {
    "zebra": 1,
    "alpha": 2,
    "mango": [3, 1, 4]
  }
}`)
	if _, err := EnsureGlobalHooks(home, false); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	raw, _ := os.ReadFile(settingsPath(home))
	s := string(raw)

	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("output not valid json: %v", err)
	}
	var perms map[string]json.RawMessage
	if err := json.Unmarshal(root["permissions"], &perms); err != nil {
		t.Fatalf("\"permissions\" missing or invalid: %v", err)
	}

	// Content: scalar values and nested array order unchanged.
	if string(perms["zebra"]) != "1" || string(perms["alpha"]) != "2" {
		t.Fatalf("scalar content changed: zebra=%s alpha=%s", perms["zebra"], perms["alpha"])
	}
	var mango []int
	if err := json.Unmarshal(perms["mango"], &mango); err != nil || len(mango) != 3 || mango[0] != 3 || mango[1] != 1 || mango[2] != 4 {
		t.Fatalf("nested array content/order changed: %v (err=%v)", mango, err)
	}

	// Key ORDER within the "permissions" object: source declared zebra, then
	// alpha, then mango. Assert the output preserves that exact order.
	iZebra := strings.Index(s, `"zebra"`)
	iAlpha := strings.Index(s, `"alpha"`)
	iMango := strings.Index(s, `"mango"`)
	if iZebra < 0 || iAlpha < 0 || iMango < 0 {
		t.Fatalf("expected keys missing from output:\n%s", s)
	}
	if iZebra >= iAlpha || iAlpha >= iMango {
		t.Fatalf("foreign object key order not preserved (want zebra<alpha<mango, got offsets %d,%d,%d):\n%s", iZebra, iAlpha, iMango, s)
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
	if _, err := EnsureGlobalHooks(home, false); err != nil {
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
	ch, err := EnsureGlobalHooks(home, false)
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
