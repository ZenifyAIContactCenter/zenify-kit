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
