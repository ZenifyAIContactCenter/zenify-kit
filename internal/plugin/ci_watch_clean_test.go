package plugin

import (
	"strings"
	"testing"
)

// FR-4.1..FR-4.5, SC-9: after green CI, ci-watch waits for the PR to be mergeable and prints the merge command.
func TestCiWatchWaitsForClean(t *testing.T) {
	assertContainsAll(t, "ci-watch/SKILL.md", syncedAsset(t, "skills/ci-watch/SKILL.md"), []string{
		"mergeStateStatus",
		"reviewThreads",
		"gh pr merge -R",
		"gh pr checks",
		"Bash(gh api graphql*)",
		"isResolved",
		"## Step 4",
		"Skill(znf:fix)",
		"CI round <n>/2",
		"pushes nothing itself",
		"report-only",
		"statusCheckRollup", // pending counted from JSON, not `gh pr checks`' exit code
		"--paginate",        // every page of review threads
		"pageInfo",
		// a project with a declared review check gets the merge command only after that check is green on the head
		"A declared `PR review check:` (`znf:pr-review`) not green on the PR head → print no merge command",
	})
}

// The harness Bash tool wraps `grep` in a ugrep shell function whose `-qv` exit code differs from
// GNU grep, which made the wait loop exit 0 while runs were still in_progress.
func TestCiWatchLoopBypassesGrepWrapper(t *testing.T) {
	body := syncedAsset(t, "skills/ci-watch/SKILL.md")
	assertContainsAll(t, "ci-watch/SKILL.md", body, []string{
		"| command grep -c .",
		"| command grep -qvx completed",
		"| command grep -qvxE 'success|skipped|neutral'",
	})
	for _, bare := range []string{"| grep -c .", "| grep -qvx"} {
		if strings.Contains(body, bare) {
			t.Errorf("ci-watch/SKILL.md: bare %q goes through the harness grep wrapper; use `command grep`", bare)
		}
	}
}
