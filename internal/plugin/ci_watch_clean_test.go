package plugin

import "testing"

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
	})
}
