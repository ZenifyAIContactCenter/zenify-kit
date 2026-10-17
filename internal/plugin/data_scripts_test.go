package plugin

import (
	"regexp"
	"strings"
	"testing"
)

// FR-5.2, FR-5.3, SC-8: one project-neutral reference teaches agents to run data scripts themselves.
func TestDataScripts_ContentAndNeutrality(t *testing.T) {
	s := syncedAsset(t, "skills/_shared/data-scripts.md")

	assertContainsAll(t, "data-scripts.md", s, []string{
		"zenify data run", "--dry-run", "--restore", "before-image", "docs/scripts/<repo>/",
		"never ask the user to run", "never read .zenify/.env", "app .env", `"ok"`,
		"lead_only", "exit 7", "same command minus --dry-run", "trip-wire",
		"broker_misconfigured",
	})

	forbidden := regexp.MustCompile(`ady8|dfh52|ffk50|ccbe|cch-hub|link_trace_id|messaging\.producer|bullmq:|192\.168|3csoft|zenify\.vn`)
	if m := forbidden.FindString(s); m != "" {
		t.Errorf("data-scripts.md carries project-specific string %q", m)
	}
	if n := strings.Count(s, "\n"); n > 200 {
		t.Errorf("data-scripts.md has %d lines, want <= 200", n)
	}

	for _, rel := range []string{
		"skills/fix/references/feedback-loop.md",
		"skills/hotfix/references/why-these-rules.md",
		"skills/_shared/knowledge-doctrine.md",
	} {
		p := syncedAsset(t, rel)
		if !strings.Contains(p, "_shared/data-scripts.md") {
			t.Errorf("%s does not point at _shared/data-scripts.md", rel)
		}
		for _, old := range []string{"script the user runs", "the agent never runs it"} {
			if strings.Contains(p, old) {
				t.Errorf("%s still says %q", rel, old)
			}
		}
	}
	if h := syncedAsset(t, "skills/_shared/harness-tools.md"); !strings.Contains(h, "zenify data run") {
		t.Error("harness-tools.md has no zenify data run row")
	}
}
