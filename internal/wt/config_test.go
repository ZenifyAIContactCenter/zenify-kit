package wt

import (
	"os"
	"path/filepath"
	"testing"
)

func writeWorktreeJSON(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, ".claude")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "worktree.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_ArrayPortRangeAndDefaults(t *testing.T) {
	root := t.TempDir()
	writeWorktreeJSON(t, root, `{"abbrev":"cch","baseRef":"origin/staging","portRange":[3250,3299],"deps":"clone"}`)
	c, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if c.Abbrev != "cch" || c.BaseRef != "origin/staging" {
		t.Fatalf("scalar wrong: %+v", c)
	}
	if c.PortRange != [2]int{3250, 3299} {
		t.Fatalf("array portRange wrong: %v", c.PortRange)
	}
	// defaults for missing keys
	if c.WorktreeDir != ".worktrees/" || c.PortEnv != "PORT" || c.User != "namph" {
		t.Fatalf("defaults wrong: %+v", c)
	}
}

func TestLoad_StringPortRangeLegacy(t *testing.T) {
	root := t.TempDir()
	writeWorktreeJSON(t, root, `{"portRange":"3100 3999"}`)
	c, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if c.PortRange != [2]int{3100, 3999} {
		t.Fatalf("string portRange wrong: %v", c.PortRange)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("want error for missing worktree.json")
	}
}

func TestLoad_NullPortRangeUsesDefault(t *testing.T) {
	root := t.TempDir()
	writeWorktreeJSON(t, root, `{"portRange":null}`)
	c, err := Load(root)
	if err != nil {
		t.Fatalf("explicit null portRange must fall back to default, got err: %v", err)
	}
	if c.PortRange != [2]int{3100, 3999} {
		t.Fatalf("null portRange wrong: %v", c.PortRange)
	}
}
