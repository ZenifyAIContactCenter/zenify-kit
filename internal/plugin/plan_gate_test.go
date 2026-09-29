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
	assertContainsAll(t, "cook", s, []string{"approve the plan", "two real user gates", "13 steps"})
	for _, bad := range []string{"Do not ask", "tasks with real code", "haiku"} {
		if strings.Contains(s, bad) {
			t.Errorf("cook/SKILL.md still contains %q", bad)
		}
	}
}

func TestWritingPlans_HandoffAsksApprovalBeforeSDD(t *testing.T) {
	s := syncedAsset(t, "skills/writing-plans/SKILL.md")
	i := strings.Index(s, "## Execution Handoff")
	if i < 0 {
		t.Fatal("writing-plans/SKILL.md: no Execution Handoff section")
	}
	h := s[i:]
	if j := strings.Index(h[3:], "\n## "); j >= 0 {
		h = h[:j+3]
	}
	assertContainsAll(t, "writing-plans Execution Handoff", h, []string{"approve the plan", "Step 5b"})
	if strings.Contains(h, "Executing via Subagent-Driven Development") {
		t.Error("Execution Handoff still hands straight to SDD")
	}
}
