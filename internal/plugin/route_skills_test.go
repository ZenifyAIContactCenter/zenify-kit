package plugin

import (
	"io/fs"
	"strings"
	"testing"
)

func TestFixSkill_RoundCounterRoutesInvestigator(t *testing.T) {
	s := syncedAsset(t, "skills/fix/SKILL.md")
	assertContainsAll(t, "fix/SKILL.md", s, []string{
		"ROUND", "model: sonnet", "model: opus", "effort: xhigh", "Think hard before responding.", "references/isolation.md",
	})
	for _, gone := range []string{"select-route", "\"site\":\"investigator\""} {
		if strings.Contains(s, gone) {
			t.Errorf("fix/SKILL.md still carries %q", gone)
		}
	}
	if len(s) > 8192 || strings.Count(s, "\n") > 200 {
		t.Fatalf("fix over cap: %d bytes", len(s))
	}
	iso := syncedAsset(t, "skills/fix/references/isolation.md")
	assertContainsAll(t, "isolation.md", iso, []string{"wt new", "--type fix", "--another"})
}

func TestFixSkill_FeedbackLoopAndRegression(t *testing.T) {
	s := syncedAsset(t, "skills/fix/SKILL.md")
	assertContainsAll(t, "fix/SKILL.md", s, []string{
		"Loop:", "Regression:", "no seam", "references/feedback-loop.md", "[DEBUG-", "<REDACTED>",
	})
	if strings.Contains(s, "superpowers") || strings.Contains(s, "not Step 0") {
		t.Error("fix/SKILL.md still carries the old isolation timing or superpowers dependency")
	}
	fl := syncedAsset(t, "skills/fix/references/feedback-loop.md")
	assertContainsAll(t, "feedback-loop.md", fl, []string{
		"Adapted from mattpocock/skills diagnosing-bugs (MIT)", "<REDACTED>", "git bisect run", "KEY=value", "[DEBUG-", "baseline",
	})
	iso := syncedAsset(t, "skills/fix/references/isolation.md")
	if strings.Contains(iso, "not Step 0") {
		t.Error("isolation.md still says the worktree waits for Step 5")
	}
}

func TestSDD_ImplementerModelFromPlanLine(t *testing.T) {
	s := syncedAsset(t, "skills/subagent-driven-development/SKILL.md")
	assertContainsAll(t, "sdd/SKILL.md", s, []string{"**Model:**", "FAIL=2", "references/model-selection.md"})
	if strings.Contains(s, "SPEC=") {
		t.Error("SDD SKILL.md must not route on SPEC=")
	}
	if strings.Contains(s, "rounds 4-5 a tier up") {
		t.Error("round-based tier bump replaced by FAIL on the same test")
	}
	if len(s) > 8192 || strings.Count(s, "\n") > 200 {
		t.Fatalf("sdd over cap: %d bytes", len(s))
	}
	ms := syncedAsset(t, "skills/subagent-driven-development/references/model-selection.md")
	assertContainsAll(t, "model-selection.md", ms, []string{"**Model:**", "FAIL=2", "FAIL=3"})
	for name, body := range map[string]string{"sdd/SKILL.md": s, "model-selection.md": ms} {
		for _, gone := range []string{"select-route", "\"site\":\"implementer\"", "never the effort"} {
			if strings.Contains(body, gone) {
				t.Errorf("%s still carries %q", name, gone)
			}
		}
	}
	for _, gone := range []string{"inside a fenced code block", "SPEC=code"} {
		if strings.Contains(ms, gone) {
			t.Errorf("model-selection.md still carries %q", gone)
		}
	}
	if strings.Contains(ms, "Fix-loop escalation (rounds 4-5)") {
		t.Error("model-selection.md still carries the round-based rule")
	}
}

func TestCookSkill_ImplementerModelFromPlanLine(t *testing.T) {
	s := syncedAsset(t, "skills/cook/SKILL.md")
	assertContainsAll(t, "cook/SKILL.md", s, []string{"**Model:**"})
	if len(s) > 8192 {
		t.Fatalf("cook over cap: %d bytes", len(s))
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
	assertContainsAll(t, "writing-plans/SKILL.md", s, []string{"Think hard before responding.", "unmeasured", "**Model:** sonnet"})
	if len(s) > 8192 || strings.Count(s, "\n") > 200 {
		t.Fatalf("writing-plans over cap: %d bytes", len(s))
	}
	assertContainsAll(t, "writing-plans/SKILL.md", s, []string{"What a Step Contains", "Review Focus", "Proportion"})
	if strings.Contains(s, "## No Placeholders") {
		t.Error("writing-plans/SKILL.md: ## No Placeholders was replaced by What a Step Contains")
	}
	if _, err := fs.Stat(assets, "assets/znf/skills/writing-plans/SKILL.md"); err != nil {
		t.Fatalf("assets prefix wrong, SKILL.md must exist: %v", err)
	}
	if _, err := fs.Stat(assets, "assets/znf/skills/writing-plans/plan-document-reviewer-prompt.md"); err == nil {
		t.Error("plan-document-reviewer-prompt.md must be removed")
	}
	dr := syncedAsset(t, "skills/writing-plans/references/decomposition-rationale.md")
	assertContainsAll(t, "decomposition-rationale.md", dr, []string{"Necessity Note", "What is built", "Why it is needed", "Simpler alternative rejected because"})
}

func TestHotfixSkill_PointsAtFixLoop(t *testing.T) {
	s := syncedAsset(t, "skills/hotfix/SKILL.md")
	assertContainsAll(t, "hotfix/SKILL.md", s, []string{"/fix` Step 5", "/fix` Step 6"})
	if len(s) > 8192 || strings.Count(s, "\n") > 200 {
		t.Fatalf("hotfix over cap: %d bytes", len(s))
	}
	w := syncedAsset(t, "skills/hotfix/references/why-these-rules.md")
	assertContainsAll(t, "hotfix why-these-rules", w, []string{"<REDACTED>", "read-only"})
}
