package plugin

import (
	"strings"
	"testing"
)

func TestImplementerPromptMandatesTDDAndFullSuite(t *testing.T) {
	s := syncedAsset(t, "skills/subagent-driven-development/implementer-prompt.md")
	assertContainsAll(t, "implementer-prompt", s, []string{"full suite", "did not cause", "skill-routing"})
	for _, bad := range []string{"if task says to", "if TDD was required"} {
		if strings.Contains(strings.ToLower(s), strings.ToLower(bad)) {
			t.Errorf("implementer-prompt still contains %q", bad)
		}
	}
}

func TestCookHasPlanApprovalGate(t *testing.T) {
	s := syncedAsset(t, "skills/cook/SKILL.md")
	assertContainsAll(t, "cook", s, []string{"approve the plan", "three real user gates", "13 steps"})
	for _, bad := range []string{"Do not ask", "tasks with real code", "haiku"} {
		if strings.Contains(s, bad) {
			t.Errorf("cook/SKILL.md still contains %q", bad)
		}
	}
}
