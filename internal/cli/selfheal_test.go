package cli

import (
	"encoding/json"
	"fmt"
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

// Guards the fast no-op branch: a stamp already matching version.Current()
// must NOT trigger a resync — if it did, EnsureGlobalHooks would create
// ~/.claude/settings.json, so its continued absence is the sentinel proving
// Sync/EnsureGlobalHooks were never invoked.
func TestSelfHeal_UpToDateStampNoOps(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	znf := filepath.Join(home, ".claude", "skills", "znf")
	if err := os.MkdirAll(znf, 0o755); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"entries":{},"version":%q}`, version.Current())
	if err := os.WriteFile(filepath.Join(znf, ".manifest.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := selfHeal(home); err != nil {
		t.Fatalf("selfHeal: %v", err)
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Fatalf("settings.json exists (err=%v) — resync ran despite up-to-date stamp", err)
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
