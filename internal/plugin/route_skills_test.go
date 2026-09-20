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

func TestForkSkills_DeclareSonnet(t *testing.T) {
	for _, name := range []string{"ground", "analyze", "standards", "explain-plan", "research"} {
		s := syncedAsset(t, "skills/"+name+"/SKILL.md")
		fm := strings.SplitN(s, "\n---", 2)[0]
		if !strings.Contains(fm, "\nmodel: sonnet\n") && !strings.HasSuffix(fm, "\nmodel: sonnet") {
			t.Errorf("%s: fork skill must declare model: sonnet in frontmatter", name)
		}
		if !strings.Contains(fm, "context: fork") {
			t.Errorf("%s: expected context: fork", name)
		}
	}
}

func TestAnalyzeSkill_RecordsPlanMetrics(t *testing.T) {
	s := syncedAsset(t, "skills/analyze/SKILL.md")
	assertContainsAll(t, "analyze/SKILL.md", s, []string{"Bash(zenify route-log *)", "zenify route-log record-plan --plan"})
}

func TestScoutAgent_ConsumerCountLine(t *testing.T) {
	s := syncedAsset(t, "agents/scout.md")
	assertContainsAll(t, "scout.md", s, []string{"consumers: N", "last line"})
}

func TestWritingPlans_SteeringAtTaskSplit(t *testing.T) {
	s := syncedAsset(t, "skills/writing-plans/SKILL.md")
	assertContainsAll(t, "writing-plans/SKILL.md", s, []string{"Think hard before responding.", "unmeasured"})
	if len(s) > 8192 || strings.Count(s, "\n") > 200 {
		t.Fatalf("writing-plans over cap: %d bytes", len(s))
	}
	dr := syncedAsset(t, "skills/writing-plans/references/decomposition-rationale.md")
	assertContainsAll(t, "decomposition-rationale.md", dr, []string{"Necessity Note", "What is built", "Why it is needed", "Simpler alternative rejected because"})
}
