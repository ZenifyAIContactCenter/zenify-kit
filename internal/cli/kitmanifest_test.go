package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

const tinyManifest = "org: acme\nrepos:\n  - name: only\n    path: repos/only\n"

// SC-5: no --manifest and no manifest/repos.yaml under cwd → embedded default.
func TestLoadKitManifest_FallsBackToEmbed(t *testing.T) {
	chdir(t, t.TempDir())
	m, src, err := loadKitManifest("", filepath.Join(t.TempDir(), "none.yaml"))
	if err != nil {
		t.Fatalf("loadKitManifest: %v", err)
	}
	if src != "embedded" || len(m.Repos) == 0 {
		t.Fatalf("want embedded manifest with repos, got src=%q repos=%d", src, len(m.Repos))
	}
}

// SC-5: manifest/repos.yaml under cwd wins over the embed (kit dev loop).
func TestLoadKitManifest_DiskFileWins(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "manifest"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest", "repos.yaml"), []byte(tinyManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)
	m, src, err := loadKitManifest("", filepath.Join(dir, "none.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if src != filepath.Join("manifest", "repos.yaml") || len(m.Repos) != 1 || m.Repos[0].Name != "only" {
		t.Fatalf("disk file must win: src=%q repos=%+v", src, m.Repos)
	}
}

// FR-05.2: an explicit --manifest always wins, and a bad path is an error (no silent fallback).
func TestLoadKitManifest_ExplicitWins(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.yaml")
	if err := os.WriteFile(p, []byte(tinyManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	m, src, err := loadKitManifest(p, filepath.Join(dir, "none.yaml"))
	if err != nil || src != p || m.Repos[0].Name != "only" {
		t.Fatalf("explicit path must be used: err=%v src=%q", err, src)
	}
	if _, _, err := loadKitManifest(filepath.Join(dir, "missing.yaml"), ""); err == nil {
		t.Fatal("explicit missing path must error, not fall back")
	}
}
