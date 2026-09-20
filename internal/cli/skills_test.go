package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
)

func TestNewSkillsCmdHasSync(t *testing.T) {
	c := newSkillsCmd()
	if c.Use != "skills" {
		t.Fatalf("Use=%q, want skills", c.Use)
	}
	var found bool
	for _, sub := range c.Commands() {
		if sub.Use == "sync" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing sync subcommand")
	}
}

// TestSkillsInstallPrunesLegacyCopy: `skills install` no longer materializes
// coding skills (they ship inside the znf plugin, 2026-09-20) — it prunes a
// leftover per-repo copy recorded in a previous manifest.
func TestSkillsInstallPrunesLegacyCopy(t *testing.T) {
	dest := t.TempDir()
	legacy := filepath.Join(dest, "react-patterns", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("old content"), 0o600); err != nil {
		t.Fatal(err)
	}
	man := &managed.Manifest{Entries: map[string]managed.Entry{}}
	if err := man.Record(legacy); err != nil {
		t.Fatal(err)
	}
	if err := man.Save(filepath.Join(dest, ".manifest.json")); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"skills", "install", "--repo", "contact-center-web", "--dest", dest})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy react-patterns copy must be pruned: %v", err)
	}
	if !strings.Contains(out.String(), "vercel-labs/agent-skills") {
		t.Fatalf("output missing leg-2 recommendation vercel-labs/agent-skills: %s", out.String())
	}
}

func TestSkillsInstallNoManifest(t *testing.T) {
	dest := t.TempDir()
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"skills", "install", "--repo", "contact-center-web", "--dest", dest})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "không có manifest") { //znf:allow-lang
		t.Fatalf("output missing no-manifest message: %s", out.String())
	}
}

func TestSkillsSync_WiresHooks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cmd := newSkillsCmd()
	cmd.SetArgs([]string{"sync"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("skills sync: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("settings.json not written: %v", err)
	}
	if !strings.Contains(string(raw), "zenify hooks-run docs-sync") {
		t.Fatal("skills sync did not wire znf hooks")
	}
}
