package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
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
	t.Setenv("ZENIFY_HOME", t.TempDir())
	ws := t.TempDir()
	if err := writePluginStamp(ws); err != nil { // also creates the .zenify marker dir
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".zenify", "manifest.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(ws)

	cmd := newSkillsCmd()
	cmd.SetArgs([]string{"sync"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("skills sync: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(ws, ".claude", "settings.local.json"))
	if err != nil {
		t.Fatalf("settings.local.json not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("skills sync must not write ~/.claude/settings.json: %v", err)
	}
	if !strings.Contains(string(raw), "zenify hooks-run docs-sync") {
		t.Fatal("skills sync did not wire znf hooks")
	}
}

// Before up migrates, the kit hooks still live in ~/.claude/settings.json:
// skills sync must not add local copies, or every hook runs twice.
func TestSkillsSync_SkipsHooksWhileUserHooksPresent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZENIFY_HOME", t.TempDir())
	if _, err := apply.EnsureGlobalHooks(apply.UserSettings(home), false); err != nil {
		t.Fatal(err)
	}
	ws := t.TempDir()
	if err := writePluginStamp(ws); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".zenify", "manifest.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(ws)

	cmd := newSkillsCmd()
	cmd.SetArgs([]string{"sync"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("skills sync: %v", err)
	}
	if b, _ := os.ReadFile(apply.WorkspaceLocalSettings(ws)); strings.Contains(string(b), "zenify hooks-run") {
		t.Fatalf("local hooks wired beside the user hooks: %s", b)
	}
	if !strings.Contains(out.String(), "zenify up --apply") {
		t.Errorf("no migrate nudge: %q", out.String())
	}
}
