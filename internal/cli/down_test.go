package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
)

// SC-25: default preview does NOT write. SC-26: non-goals stay untouched.
func TestDown_DryRun_NoWritesAndNonGoalsUntouched(t *testing.T) {
	ws := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	// global settings.json has 1 znf hook.
	gs := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(gs), 0o750); err != nil {
		t.Fatal(err)
	}
	gsBody := []byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"zenify hooks-run docs-sync"}]}]}}`)
	if err := os.WriteFile(gs, gsBody, 0o600); err != nil {
		t.Fatal(err)
	}

	// manifest repos.yaml minimally 1 repo "svc" + repo dir with exclude + settings.
	repoDir := filepath.Join(ws, "svc")
	excl := filepath.Join(repoDir, ".git", "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(excl), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(excl, []byte(".worktrees/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(repoDir, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte(`{"env":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// ownership manifest records settings as owned.
	zdir := filepath.Join(ws, ".zenify")
	if err := os.MkdirAll(zdir, 0o750); err != nil {
		t.Fatal(err)
	}
	owned := &managed.Manifest{Entries: map[string]managed.Entry{}}
	_ = owned.Record(settings)
	if err := owned.Save(filepath.Join(zdir, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	// non-goals: cloned code file + docs store.
	code := filepath.Join(repoDir, "main.go")
	if err := os.WriteFile(code, []byte("package main"), 0o600); err != nil {
		t.Fatal(err)
	}
	repos := filepath.Join(ws, "manifest", "repos.yaml")
	if err := os.MkdirAll(filepath.Dir(repos), 0o750); err != nil {
		t.Fatal(err)
	}
	// minimal valid input for manifest.LoadWithOverlay — 1 repo path "svc".
	if err := os.WriteFile(repos, []byte("org: TestOrg\nrepos:\n  - name: svc\n    path: svc\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := newDownCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"--workspace", ws, "--manifest", repos}) // NO --apply → preview
	if err := cmd.Execute(); err != nil {
		t.Fatalf("down preview: %v", err)
	}

	// SC-25: must not write — global settings.json, exclude, settings.local.json unchanged.
	if b, _ := os.ReadFile(gs); !bytes.Equal(b, gsBody) { //nolint:gosec // G304 -- test-local path under t.TempDir, not externally-tainted
		t.Error("preview must NOT modify global settings.json")
	}
	if b, _ := os.ReadFile(excl); string(b) != ".worktrees/\n" { //nolint:gosec // G304 -- test-local path under t.TempDir, not externally-tainted
		t.Error("preview must NOT modify exclude")
	}
	if _, err := os.Stat(settings); err != nil {
		t.Error("preview must NOT delete settings.local.json")
	}
	// SC-26: non-goals stay untouched.
	if _, err := os.Stat(code); err != nil {
		t.Error("cloned code file must stay untouched")
	}
	if _, err := os.Stat(filepath.Join(zdir, "manifest.json")); err != nil {
		t.Error(".zenify/manifest.json must stay untouched")
	}
}
