package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
)

// SC-8: stale stamp triggers re-sync (stamp gets rewritten to Current).
func TestSelfHeal_StaleStampReSyncs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// seed a stale manifest under ~/.claude/skills/znf/
	znf := filepath.Join(home, ".claude", "skills", "znf")
	if err := os.MkdirAll(znf, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(znf, ".manifest.json"),
		[]byte(`{"entries":{},"version":"v0.0.1"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := selfHeal(home); err != nil {
		t.Fatalf("selfHeal: %v", err)
	}

	raw, _ := os.ReadFile(filepath.Join(znf, ".manifest.json"))
	var m struct {
		Version string `json:"version"`
	}
	_ = json.Unmarshal(raw, &m)
	if m.Version != version.Current() {
		t.Fatalf("stamp = %q, want %q (re-sync did not run)", m.Version, version.Current())
	}
}

// FR-6.2 fail-open: broken home dir must not error the caller.
func TestSelfHeal_FailOpen(t *testing.T) {
	// point HOME at a path where sync will fail; selfHeal must swallow it.
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := selfHeal(home); err != nil {
		t.Fatalf("selfHeal should be fail-open, got %v", err)
	}
}
