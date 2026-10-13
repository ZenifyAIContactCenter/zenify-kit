package plugin

import (
	"strings"
	"testing"
)

// FR-7 / SC-4: brainstorming dispatches the spec reviewer and no longer routes an architect.
func TestBrainstorming_Skill(t *testing.T) {
	s := syncedAsset(t, "skills/brainstorming/SKILL.md")
	assertContainsAll(t, "brainstorming/SKILL.md", s, []string{
		"Agent(znf:spec-reviewer)", "spec review ran on:", "Think hard before responding.",
	})
	for _, banned := range []string{"architect-gate.md", "architect-memo.md", "select-route", "Agent(architect)"} {
		if strings.Contains(s, banned) {
			t.Errorf("brainstorming/SKILL.md must not contain %q", banned)
		}
	}
	if len(s) > 8192 || strings.Count(s, "\n") > 200 {
		t.Fatalf("brainstorming over cap: %d bytes, %d lines", len(s), strings.Count(s, "\n"))
	}
}

// FR-1 / FR-2 / SC-1 / SC-2: questions come in rounds, and the design dialogue carries the deep-module tests.
func TestBrainstorming_AsksInRounds(t *testing.T) {
	s := syncedAsset(t, "skills/brainstorming/SKILL.md")
	d := syncedAsset(t, "skills/brainstorming/references/design-dialogue.md")
	for name, body := range map[string]string{"SKILL.md": s, "design-dialogue.md": d} {
		low := strings.ToLower(body)
		for _, banned := range []string{"one at a time", "one question per message", "one per message"} {
			if strings.Contains(low, banned) {
				t.Errorf("%s must not contain %q", name, banned)
			}
		}
	}
	assertContainsAll(t, "brainstorming/SKILL.md", s, []string{"ask the user to choose between options", "at most 4", "at most five"})
	if strings.Contains(d, "Design for isolation and clarity") {
		t.Errorf("design-dialogue.md must not contain the removed isolation heading")
	}
	if strings.Contains(s, "AskUserQuestion") {
		t.Errorf("brainstorming/SKILL.md must not name AskUserQuestion")
	}
	assertContainsAll(t, "design-dialogue.md", d, []string{"deletion test", "test surface", "one adapter", "invariants", "different constraint"})
}
