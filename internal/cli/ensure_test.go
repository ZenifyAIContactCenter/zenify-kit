package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
)

// storeFixture builds $ZENIFY_HOME/knowledge/.config with one rule so
// ensureWorkspace has something to distribute.
func storeFixture(t *testing.T) (zenifyHome string) {
	t.Helper()
	zenifyHome = t.TempDir()
	cfg := filepath.Join(zenifyHome, "knowledge", ".config", "rules")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(cfg, "00-constitution.md"), []byte("rule\n"), 0o644)
	os.WriteFile(filepath.Join(zenifyHome, "knowledge", ".config", "distribution.txt"), []byte("rules/ .claude/rules/\n"), 0o644)
	t.Setenv("ZENIFY_HOME", zenifyHome)
	return zenifyHome
}

func stampManifest(t *testing.T, home, ver string) {
	t.Helper()
	znf := filepath.Join(home, ".claude", "skills", "znf")
	if err := os.MkdirAll(znf, 0o755); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"entries":{},"version":%q}`, ver)
	if err := os.WriteFile(filepath.Join(znf, ".manifest.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// SC-1 / FR-01.5: first run writes rules + hooks + model and says so; second
// run is a silent no-op on stdout.
func TestEnsureWorkspace_WritesOnceThenSilent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	stampManifest(t, home, version.Current()) // skip plugin resync (slow, tested elsewhere)
	storeFixture(t)
	ws := t.TempDir()

	var o1, e1 bytes.Buffer
	ensureWorkspace(ws, home, &o1, &e1)
	if _, err := os.Stat(filepath.Join(ws, ".claude", "rules", "00-constitution.md")); err != nil {
		t.Fatalf("rule not distributed: %v (stderr=%s)", err, e1.String())
	}
	raw, err := os.ReadFile(filepath.Join(ws, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("workspace settings.json missing: %v", err)
	}
	var s map[string]any
	_ = json.Unmarshal(raw, &s)
	if s["model"] != "claude-opus-4-8" {
		t.Fatalf("model pin missing: %v", s["model"])
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); err != nil {
		t.Fatalf("global hooks not wired: %v", err)
	}
	for _, want := range []string{"wired ", "pinned workspace model", "znf config: đã ghi 1 file"} {
		if !strings.Contains(o1.String(), want) {
			t.Fatalf("stdout missing %q: %s", want, o1.String())
		}
	}

	var o2, e2 bytes.Buffer
	ensureWorkspace(ws, home, &o2, &e2)
	if o2.Len() != 0 {
		t.Fatalf("second run must be silent on stdout, got: %s", o2.String())
	}
}

// SC-7 / FR-01.2: an unreachable store must not stop hooks + model from being
// written, and must leave one 'znf ensure:' line on stderr.
func TestEnsureWorkspace_FailOpenPerStep(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	stampManifest(t, home, version.Current())
	t.Setenv("ZENIFY_HOME", filepath.Join(t.TempDir(), "empty"))
	ws := t.TempDir()

	var o, e bytes.Buffer
	ensureWorkspace(ws, home, &o, &e)
	if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); err != nil {
		t.Fatalf("hooks must still be wired: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ws, ".claude", "settings.json")); err != nil {
		t.Fatalf("model must still be pinned: %v", err)
	}
	if !strings.Contains(e.String(), "znf ensure:") {
		t.Fatalf("expected a 'znf ensure:' stderr line, got: %q", e.String())
	}
}

// Ported from selfheal_test: a stale stamp re-materializes the plugin and
// rewrites the stamp to the running version.
func TestEnsureWorkspace_StaleStampResyncsPlugin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	stampManifest(t, home, "v0.0.1")
	storeFixture(t)

	var o, e bytes.Buffer
	ensureWorkspace(t.TempDir(), home, &o, &e)

	raw, _ := os.ReadFile(filepath.Join(home, ".claude", "skills", "znf", ".manifest.json"))
	var m struct {
		Version string `json:"version"`
	}
	_ = json.Unmarshal(raw, &m)
	if m.Version != version.Current() {
		t.Fatalf("stamp = %q, want %q (resync did not run)", m.Version, version.Current())
	}
}

// Ported from selfheal_test: an up-to-date stamp must NOT resync (the skills
// dir stays empty apart from the manifest we planted).
func TestEnsureWorkspace_UpToDateStampNoResync(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	stampManifest(t, home, version.Current())
	storeFixture(t)

	var o, e bytes.Buffer
	ensureWorkspace(t.TempDir(), home, &o, &e)

	ents, _ := os.ReadDir(filepath.Join(home, ".claude", "skills", "znf"))
	if len(ents) != 1 {
		t.Fatalf("resync ran despite up-to-date stamp: %d entries", len(ents))
	}
}
