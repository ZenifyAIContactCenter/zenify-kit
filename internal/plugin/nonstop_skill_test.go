package plugin

import (
	"strings"
	"testing"
)

// FR-5.1..FR-5.9, SC-12, SC-13, SC-15: nonstop plans the day behind an approval gate, runs the
// worker loop, closes the day with carry-over and calibrates its T-shirt estimates.
func TestNonstopSkill(t *testing.T) {
	refs := []string{"daily-plan.md", "estimation.md", "worker-brief.md"}
	skill := syncedAsset(t, "skills/nonstop/SKILL.md")
	all := skill
	ref := map[string]string{}
	for _, r := range refs {
		body := syncedAsset(t, "skills/nonstop/references/"+r)
		if strings.Contains(body, "references/") {
			t.Errorf("nonstop/references/%s must not contain references/", r)
		}
		if !strings.Contains(skill, "references/"+r) {
			t.Errorf("nonstop/SKILL.md must link references/%s", r)
		}
		ref[r] = body
		all += "\n" + body
	}
	// The orchestrator owns ci-watch, routes the day (heartbeat = continue) and packages the review file.
	assertContainsAll(t, "nonstop/SKILL.md", skill, []string{
		"name: nonstop",
		"wait for the user's approval",
		"ZNF_AUTONOMOUS=1",
		"/znf:nonstop continue",
		"Skill(znf:ci-watch)",
		"ls-files --others --exclude-standard",
	})
	// A worker never runs ci-watch: its watcher would wake no one.
	if strings.Contains(ref["worker-brief.md"], "Skill(znf:ci-watch)") {
		t.Errorf("nonstop/references/worker-brief.md must not tell the worker to run Skill(znf:ci-watch)")
	}
	// Workers never see SessionStart: the brief carries the feature line for ship's gate.
	assertContainsAll(t, "nonstop/references/worker-brief.md", ref["worker-brief.md"], []string{"Features: github.issues"})
	// The gate is the Features field only: no Rules sentence may read like an "on" field.
	if strings.Contains(ref["worker-brief.md"], "Features: github.issues on →") {
		t.Errorf("nonstop/references/worker-brief.md must not contain %q", "Features: github.issues on →")
	}
	// The orchestrator's ci-watch only reports; it never hands a worker's PR to znf:fix.
	assertContainsAll(t, "nonstop/SKILL.md", skill, []string{"report-only"})
	// The morning plan carries every open line of the last section.
	assertContainsAll(t, "nonstop/references/daily-plan.md", ref["daily-plan.md"], []string{"non-done"})
	assertContainsAll(t, "nonstop skill + references", all, []string{
		"progress.md",
		"## YYYY-MM-DD",
		"date +%F",
		"carry-over",
		"features: github.issues on",
		"gh pr merge -R",
		"Skill(znf:ci-watch)",
		"ZNF_AUTONOMOUS=1",
		"wait for the user's approval",
		"estimates.md",
		"duration_ms",
		"git diff --shortstat",
		"XL",
		"median",
		"S=1", "M=2", "L=4", "XL=8",
		"gh search issues --assignee=@me --state=open --owner=",
		"deferred until",
		"Issue: #N",
		"READY_FOR_REVIEW",
		"znf:code-reviewer",
	})
}
