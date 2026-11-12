package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShipGate_RunsRulesLint(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(dest, "skills/ship/SKILL.md"))
	s := string(b)
	if !strings.Contains(s, "zenify rules lint") {
		t.Error("ship SKILL.md missing the rules-lint gate")
	}
	// Two active gates: the distributed-rules gate runs on every /ship (any repo),
	// and the full-tree gate runs when shipping the kit repo itself (the
	// language-retrofit milestone translated the kit's Go + skill assets, so this
	// is enforced now rather than deferred).
	if !strings.Contains(s, "zenify rules lint ~/.zenify/knowledge/.config/rules") {
		t.Error("ship gate must run the rules-lint scoped to the distributed rules dir")
	}
	if !strings.Contains(s, "zenify rules lint --include-go") {
		t.Error("ship gate must run the full-tree --include-go gate for the kit repo")
	}
}

// FR-03.2: ship step 4 under ZNF_AUTONOMOUS=1 keeps its UI path in a reference (SKILL.md is at its byte budget).
func TestShipGate_AutonomousUI(t *testing.T) {
	skill := syncedAsset(t, "skills/ship/SKILL.md")
	if !strings.Contains(skill, "references/autonomous-ui.md") {
		t.Error("ship SKILL.md must link references/autonomous-ui.md")
	}
	ref := syncedAsset(t, "skills/ship/references/autonomous-ui.md")
	assertContainsAll(t, "autonomous-ui.md", ref, []string{"ZNF_SANDBOX_WEB_PORT", "exit 8"})
}

// FR-2: ship step 4 runs the tests affected by the branch; the rules live in a reference (SKILL.md is at its byte budget).
func TestShipTestScope(t *testing.T) {
	skill := syncedAsset(t, "skills/ship/SKILL.md")
	if !strings.Contains(skill, "references/test-scope.md") {
		t.Error("ship SKILL.md must link references/test-scope.md")
	}
	ref := syncedAsset(t, "skills/ship/references/test-scope.md")
	assertContainsAll(t, "test-scope.md", ref, []string{
		`"$(zenify wt base)"`, "--changed", "--changedSince", "git config --worktree --get wt.type",
		"hotfix", "full suite", "timeout", "background", "count", "ship fix loop",
	})
	if strings.Contains(ref, "references/") {
		t.Error("test-scope.md must not contain the string references/")
	}
}

// FR-4 / SC-5: after opening the PR, ship hands the project's declared bot-review loop to znf:pr-review,
// which also runs by hand on an existing PR. The loop names no bot and no tool: both come from the project's line.
func TestShipPRReviewLoop(t *testing.T) {
	skill := syncedAsset(t, "skills/ship/SKILL.md")
	assertContainsAll(t, "ship SKILL.md", skill, []string{
		"`Skill(znf:pr-review)`", "`--watch` → `Skill(znf:ci-watch)`",
	})
	if strings.Contains(skill, "pr-review-loop") {
		t.Error("ship SKILL.md still points at the removed pr-review-loop reference")
	}
	if _, err := assets.ReadFile("assets/znf/skills/ship/references/pr-review-loop.md"); err == nil {
		t.Error("ship/references/pr-review-loop.md must be gone: the loop lives in pr-review/SKILL.md")
	}
	loop := syncedAsset(t, "skills/pr-review/SKILL.md")
	assertContainsAll(t, "pr-review/SKILL.md", loop, []string{
		"PR review check:", "CLAUDE.md", "headRefOid", "conclusion", "only that check", "ONE commit",
		"MEDIUM", "decline syntax", "git fetch", "30 minutes", "never force-push", "Skill(znf:review)",
		"3 rounds", "green", "ci-watch", "no such line",
		"Watch applies", "CI round", "Every stop below ends only this loop", "voids 2 and 3", "do not push",
		// by hand on an existing PR
		"By hand", "headRefName", "`znf:ship` step 2",
		// two exchange paths; the verdict stays the check's conclusion on both
		"Tool path", "Comment path", "post nothing on the PR", "Call a tool the project declares",
		"references/comment-threads.md", "or a tool result",
		// verify before fixing; never a merge command before the verdict
		"Read the code each one points to", "print a merge command only when the check is green",
	})
	for _, path := range []string{"skills/pr-review/SKILL.md", "skills/ci-watch/SKILL.md", "skills/_shared/harness-tools.md"} {
		body := strings.ToLower(syncedAsset(t, path))
		for _, banned := range []string{"zenie", "zenieai", "review_wait", "review_decline", "review_request"} {
			if strings.Contains(body, banned) {
				t.Errorf("%s must not contain %q (case insensitive)", path, banned)
			}
		}
		if path != "skills/_shared/harness-tools.md" && strings.Contains(body, "mcp__") {
			t.Errorf("%s must not contain %q (case insensitive)", path, "mcp__")
		}
	}
	threads := syncedAsset(t, "skills/pr-review/references/comment-threads.md")
	assertContainsAll(t, "comment-threads.md", threads, []string{
		"app.slug", "reviewThreads", "addPullRequestReviewThreadReply", "resolveReviewThread", "never resolve it",
		"On the tool path nothing is posted",
	})
	for _, banned := range []string{"references/", "zenie", "zenieai", "review_wait", "review_decline", "review_request", "mcp__"} {
		if strings.Contains(strings.ToLower(threads), banned) {
			t.Errorf("comment-threads.md must not contain %q", banned)
		}
	}
	assertContainsAll(t, "harness-tools.md", syncedAsset(t, "skills/_shared/harness-tools.md"), []string{
		"Call a tool the project declares",
	})
}
