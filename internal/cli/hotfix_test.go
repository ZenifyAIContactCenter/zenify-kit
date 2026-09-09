package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// seedRepoCfg writes .claude/worktree.json for a fake repo, returning repoRoot.
func seedRepoCfg(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".claude", "worktree.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestHotfixBaserefStandalone(t *testing.T) {
	dir := seedRepoCfg(t, `{"abbrev":"lumi","hotfix":{"baseStrategy":"standalone"}}`)
	out, err := resolveHotfixBaseRef(dir, nil) // standalone doesn't need git
	if err != nil {
		t.Fatal(err)
	}
	if out != "origin/staging" {
		t.Fatalf("standalone → %q, want origin/staging", out)
	}
}

func TestHotfixBaserefCustom(t *testing.T) {
	dir := seedRepoCfg(t, `{"abbrev":"x","hotfixBaseRef":"origin/release99","hotfix":{"baseStrategy":"custom"}}`)
	out, err := resolveHotfixBaseRef(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out != "origin/release99" {
		t.Fatalf("custom → %q, want origin/release99", out)
	}
}
