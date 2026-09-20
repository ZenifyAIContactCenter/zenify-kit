package plugin

import (
	"strings"
	"testing"
)

func TestFixSkill_RoundCounterRoutesInvestigator(t *testing.T) {
	s := syncedAsset(t, "skills/fix/SKILL.md")
	assertContainsAll(t, "fix/SKILL.md", s, []string{
		"ROUND", "select-route investigator ROUND=", "\"site\":\"investigator\"", "Think hard before responding.", "references/isolation.md",
	})
	if strings.Contains(s, "model: 'sonnet'") {
		t.Error("fix must take the investigator model from select-route, not a literal")
	}
	if len(s) > 8192 || strings.Count(s, "\n") > 200 {
		t.Fatalf("fix over cap: %d bytes", len(s))
	}
	iso := syncedAsset(t, "skills/fix/references/isolation.md")
	assertContainsAll(t, "isolation.md", iso, []string{"wt new", "--type fix", "--another"})
}

func TestSDD_ImplementerTierFromSelectRoute(t *testing.T) {
	s := syncedAsset(t, "skills/subagent-driven-development/SKILL.md")
	assertContainsAll(t, "sdd/SKILL.md", s, []string{"select-route implementer SPEC=", "FAIL=", "references/model-selection.md"})
	if strings.Contains(s, "rounds 4-5 a tier up") {
		t.Error("round-based tier bump replaced by FAIL on the same test")
	}
	if len(s) > 8192 || strings.Count(s, "\n") > 200 {
		t.Fatalf("sdd over cap: %d bytes", len(s))
	}
	ms := syncedAsset(t, "skills/subagent-driven-development/references/model-selection.md")
	assertContainsAll(t, "model-selection.md", ms, []string{"SPEC=code", "SPEC=prose", "FAIL=2", "FAIL=3", "investigator ROUND=3", "\"site\":\"implementer\""})
	if strings.Contains(ms, "Fix-loop escalation (rounds 4-5)") {
		t.Error("model-selection.md still carries the round-based rule")
	}
}
