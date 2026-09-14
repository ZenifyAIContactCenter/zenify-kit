package cli

import (
	"testing"
	"unicode"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/langgate"
	"github.com/spf13/cobra"
)

// FR-1.1: every Vietnamese line in Go sources carries //znf:allow-lang, and no
// English-only cobra Short/Long remains untagged is checked by TestHelpIsVietnamese.
func TestLanggate_InternalGoClean(t *testing.T) {
	vs, err := langgate.Scan([]string{".."}, true) // cwd = internal/cli → ".." = internal/
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	for _, v := range vs {
		t.Errorf("%s:%d: untagged Vietnamese: %q", v.File, v.Line, v.Text)
	}
}

// SC-2: no cobra command still describes itself in English. Heuristic: every
// non-hidden Short must contain at least one Vietnamese letter.
func TestHelpIsVietnamese(t *testing.T) {
	root := NewRootCmd()
	for _, c := range allCommands(root) {
		if c.Hidden || c.Name() == "help" || c.Name() == "completion" || hasHiddenAncestor(c) {
			continue
		}
		if !hasVietnameseLetter(c.Short) {
			t.Errorf("%s: Short is not Vietnamese: %q", c.CommandPath(), c.Short)
		}
		if c.Long != "" && !hasVietnameseLetter(c.Long) {
			t.Errorf("%s: Long is not Vietnamese: %q", c.CommandPath(), c.Long)
		}
	}
}

func hasHiddenAncestor(c *cobra.Command) bool {
	for p := c.Parent(); p != nil; p = p.Parent() {
		if p.Hidden {
			return true
		}
	}
	return false
}

func allCommands(c *cobra.Command) []*cobra.Command {
	out := []*cobra.Command{c}
	for _, sub := range c.Commands() {
		out = append(out, allCommands(sub)...)
	}
	return out
}

func hasVietnameseLetter(s string) bool {
	for _, r := range s {
		if r > 127 && unicode.IsLetter(r) {
			return true
		}
	}
	return false
}
