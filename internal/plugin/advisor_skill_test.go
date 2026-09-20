package plugin

import (
	"strings"
	"testing"
)

func TestAdvisorSkill_Shipped(t *testing.T) {
	s := syncedAsset(t, "skills/advisor/SKILL.md")
	assertContainsAll(t, "advisor/SKILL.md", s, []string{
		"name: advisor", "argument-hint:", "hỏi fable", "ý kiến thứ hai", "second opinion", "@fable", "ultrathink", "nghĩ kỹ hơn",
		"select-route manual", "Agent(general-purpose)", "route-log record", "\"site\":\"manual\"", "trigger",
		"Strong | Worth exploring | Speculative", "at most 40 lines",
	})
	for _, banned := range []string{"context: fork", "model:", "disable-model-invocation", "effort:"} {
		if strings.Contains(s, banned) {
			t.Errorf("advisor must not declare %q", banned)
		}
	}
	// the strong-model word may appear only on the description line
	for i, line := range strings.Split(s, "\n") {
		if strings.Contains(strings.ToLower(line), "fable") && !strings.HasPrefix(line, "description:") {
			t.Errorf("line %d names the strong model outside description: %q", i+1, line)
		}
	}
	assertProjectAgnostic(t, "advisor/SKILL.md", s)
	if len(s) > 8192 || strings.Count(s, "\n") > 200 {
		t.Fatalf("advisor over cap: %d bytes", len(s))
	}
}
