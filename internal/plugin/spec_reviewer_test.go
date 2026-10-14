package plugin

import (
	"strings"
	"testing"
)

// FR-6 / SC-5: spec-reviewer agent — opus, read-only Bash, severity labels, 40-line return.
func TestSpecReviewerAgent_Shipped(t *testing.T) {
	s := syncedAsset(t, "agents/spec-reviewer.md")
	assertContainsAll(t, "spec-reviewer.md", s, []string{
		"name: spec-reviewer",
		"model: opus",
		"tools: Read, Grep, Glob, Bash, Write",
		"at most 40 lines",
		"model: <",
		"HIGH",
		"MEDIUM",
		"LOW",
	})
	front := strings.SplitN(s, "\n---", 2)[0]
	for _, line := range strings.Split(front, "\n") {
		if !strings.HasPrefix(line, "tools:") {
			continue
		}
		for _, tok := range strings.Split(strings.TrimPrefix(line, "tools:"), ",") {
			if strings.TrimSpace(tok) == "Agent" {
				t.Error("spec-reviewer must not dispatch: Agent in tools")
			}
		}
	}
	assertProjectAgnostic(t, "spec-reviewer.md", s)
	if len(s) > 8192 {
		t.Errorf("spec-reviewer.md is %d bytes; keep the persona under the skill cap", len(s))
	}
}
