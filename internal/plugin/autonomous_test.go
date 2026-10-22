package plugin

import (
	"strings"
	"testing"
)

// FR-5.1, FR-5.2, FR-5.6, FR-5.7: one shared doc overrides every human gate when ZNF_AUTONOMOUS=1.
func TestAutonomousDoc(t *testing.T) {
	s := syncedAsset(t, "skills/_shared/autonomous.md")
	assertContainsAll(t, "autonomous.md", s, []string{
		"ZNF_AUTONOMOUS=1", "AskUserQuestion",
		"zenify await spec", "zenify await plan", "zenify await data-write",
		"zenify await irreversible", "zenify await needs-human",
		"Ruling: <decision> — <why> — <cost if wrong>",
		"resume <id> approved", "resume <id> rejected:", "zenify await show <id>",
		"--summary", "--file", "--command", "never merge",
	})
	// One table row per human gate of spec FR-5.2, nine in all.
	rows := []string{"`brainstorming` steps 2, 4, 7, 8", "`brainstorming` step 12", "`cook` Step 5b", "`fix` Step 4",
		"`ship` fix-loop", "`_shared/data-scripts.md`", "Four things stop you\": first three", "Four things stop you\": plan too broken", "`discipline`"}
	n := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "| ") && !strings.HasPrefix(line, "| Human gate") {
			n++
		}
	}
	if n != len(rows) {
		t.Errorf("autonomous.md table has %d rows, want %d", n, len(rows))
	}
	for _, r := range rows {
		found := false
		for _, line := range strings.Split(s, "\n") {
			if strings.HasPrefix(line, "| ") && strings.Contains(line, r) {
				found = true
			}
		}
		if !found {
			t.Errorf("autonomous.md has no table row for %q", r)
		}
	}
	assertProjectAgnostic(t, "autonomous.md", s)
}
