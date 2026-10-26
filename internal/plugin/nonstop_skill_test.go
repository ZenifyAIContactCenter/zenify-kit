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
	for _, r := range refs {
		body := syncedAsset(t, "skills/nonstop/references/"+r)
		if strings.Contains(body, "references/") {
			t.Errorf("nonstop/references/%s must not contain references/", r)
		}
		if !strings.Contains(skill, "references/"+r) {
			t.Errorf("nonstop/SKILL.md must link references/%s", r)
		}
		all += "\n" + body
	}
	assertContainsAll(t, "nonstop/SKILL.md", skill, []string{
		"name: nonstop",
		"wait for the user's approval",
		"ZNF_AUTONOMOUS=1",
	})
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
