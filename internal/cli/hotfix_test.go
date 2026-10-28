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
	dir := seedRepoCfg(t, `{"abbrev":"lumi","baseRef":"origin/staging","hotfix":{"baseStrategy":"standalone"}}`)
	out, err := resolveHotfixBaseRef(dir)
	if err != nil {
		t.Fatal(err)
	}
	if out != "origin/staging" {
		t.Fatalf("standalone → %q, want origin/staging", out)
	}
}

func TestHotfixBaserefCustom(t *testing.T) {
	dir := seedRepoCfg(t, `{"abbrev":"x","hotfixBaseRef":"origin/main","hotfix":{"baseStrategy":"custom"}}`)
	out, err := resolveHotfixBaseRef(dir)
	if err != nil {
		t.Fatal(err)
	}
	if out != "origin/main" {
		t.Fatalf("custom → %q, want origin/main", out)
	}
}

func TestHotfixBaserefReleaseLatestRetired(t *testing.T) {
	dir := seedRepoCfg(t, `{"abbrev":"x","hotfix":{"baseStrategy":"release-latest"}}`)
	if out, err := resolveHotfixBaseRef(dir); err == nil {
		t.Fatalf("release-latest must fail instead of guessing a release branch, got %q", out)
	}
}
