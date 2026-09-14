package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func runDocsGen(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := NewRootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(append([]string{"docs", "gen"}, args...))
	err := cmd.Execute()
	return out.String() + errb.String(), err
}

func TestDocsGen_WritesEveryVisibleCommandThenCheckPasses(t *testing.T) {
	dir := t.TempDir()
	if _, err := runDocsGen(t, "--out", dir); err != nil {
		t.Fatal(err)
	}
	// one page per visible command in the real tree
	var visible int
	for _, c := range allCommands(NewRootCmd()) {
		if !c.Hidden && c.IsAvailableCommand() && !anyHidden(c) {
			visible++
		}
	}
	pages, _ := filepath.Glob(filepath.Join(dir, "cli", "zenify*.md"))
	if len(pages) != visible {
		t.Fatalf("cli pages = %d, visible commands = %d", len(pages), visible)
	}
	for _, must := range []string{"cli/index.md", "skills/index.md", "skills/cook.md", "agents/scout.md", "hooks.md"} {
		if _, err := os.Stat(filepath.Join(dir, must)); err != nil {
			t.Errorf("missing %s", must)
		}
	}
	if _, err := runDocsGen(t, "--out", dir, "--check"); err != nil {
		t.Fatalf("check on fresh output must pass: %v", err)
	}
}

func TestDocsGen_CheckFailsOnDrift(t *testing.T) {
	dir := t.TempDir()
	_, _ = runDocsGen(t, "--out", dir)
	p := filepath.Join(dir, "cli", "zenify_wt.md")
	_ = os.WriteFile(p, []byte("drift"), 0o644)
	out, err := runDocsGen(t, "--out", dir, "--check")
	if err == nil {
		t.Fatal("expected error on drift")
	}
	if !strings.Contains(out, "cli/zenify_wt.md") {
		t.Fatalf("drifted file not named in output: %q", out)
	}
}

func anyHidden(c *cobra.Command) bool {
	for p := c.Parent(); p != nil; p = p.Parent() {
		if p.Hidden {
			return true
		}
	}
	return false
}
