package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		t.Fatal("thiếu subcommand sync")
	}
}

func TestSkillsInstallSubset(t *testing.T) {
	dest := t.TempDir()
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"skills", "install", "--repo", "contact-center-web", "--dest", dest})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "react-patterns", "SKILL.md")); err != nil {
		t.Fatalf("react-patterns chưa materialize: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "nestjs-patterns")); !os.IsNotExist(err) {
		t.Fatalf("web KHÔNG được có nestjs-patterns")
	}
	if !strings.Contains(out.String(), "vercel-labs/agent-skills") {
		t.Fatalf("output thiếu khuyến nghị leg-2 vercel-labs/agent-skills: %s", out.String())
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
