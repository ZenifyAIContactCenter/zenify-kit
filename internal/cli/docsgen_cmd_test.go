package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/docsgen"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/plugin"

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

// TestDocsGen_CatalogCoversEveryPage keeps the hand-written reference prose
// in step with the binary: every visible command, skill and agent needs a
// catalog fragment, and a fragment that writes its own flags table must
// mention every flag the command declares.
func TestDocsGen_CatalogCoversEveryPage(t *testing.T) {
	var missing, uncovered []string
	root := NewRootCmd()
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if !c.Hidden && (c == root || c.IsAvailableCommand()) {
			rel := "cli/" + strings.ReplaceAll(c.CommandPath(), " ", "_")
			frag, ok := docsgen.LoadFragment(rel)
			if !ok {
				missing = append(missing, rel)
			} else if frag.HasSection("Cờ") { //znf:allow-lang
				for _, f := range docsgen.FlagNames(c) {
					if !strings.Contains(frag.Body, "`--"+f+"`") {
						uncovered = append(uncovered, rel+" --"+f)
					}
				}
			}
			for _, s := range c.Commands() {
				walk(s)
			}
		}
	}
	walk(root)
	skills, err := docsgen.GenSkills(plugin.ZnfFS(), plugin.CodingFS())
	if err != nil {
		t.Fatal(err)
	}
	for rel := range skills {
		rel = strings.TrimSuffix(rel, ".md")
		if _, ok := docsgen.LoadFragment(rel); !ok {
			missing = append(missing, rel)
		}
	}
	sort.Strings(missing)
	sort.Strings(uncovered)
	if len(missing) > 0 {
		t.Errorf("catalog fragment missing for %d page(s):\n  %s", len(missing), strings.Join(missing, "\n  "))
	}
	if len(uncovered) > 0 {
		t.Errorf("hand-written flag table misses %d flag(s):\n  %s", len(uncovered), strings.Join(uncovered, "\n  "))
	}
}
