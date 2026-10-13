package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewSkill_Materialized_HasKeyParts(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "skills/review/SKILL.md"))
	if err != nil {
		t.Fatalf("review/SKILL.md not materialized: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		"select-tier",    // calls the tier script
		"T1", "T2", "T3", // 3 tiers
		"PRE", "BUNDLE", "REVIEW", "VERIFY", // 4 seams
		"finding-schema.md", // references the shared schema
	} {
		if !strings.Contains(s, want) {
			t.Errorf("review/SKILL.md missing %q", want)
		}
	}
}

func TestReviewSkill_M4bWiring(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	skill, err := os.ReadFile(filepath.Join(dest, "skills/review/SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	s := string(skill)
	for _, want := range []string{"mechanical-gate", "review-verify", "evidence", "Bash(zenify *)"} {
		if !strings.Contains(s, want) {
			t.Errorf("SKILL.md missing %q (M4b wiring)", want)
		}
	}
}

func TestReviewSkill_M4cWiring(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	skill := filepath.Join(dest, "skills", "review", "SKILL.md")
	b, err := os.ReadFile(skill)
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		"review-bundle", // calls the bundler subcommand
		"BUNDLE",        // the seam has a name
		"2000",          // activation threshold
		"too-large",     // "split the PR" stop branch
		"per-bundle",    // review each bundle
	} {
		if !strings.Contains(s, want) {
			t.Errorf("SKILL.md missing M4c wiring: %q", want)
		}
	}
}

func TestShipStep5_DelegatesToReview(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "skills/ship/SKILL.md"))
	if err != nil {
		t.Fatalf("ship/SKILL.md: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, "znf:review") {
		t.Error("ship step 5 does not yet delegate to znf:review")
	}
	if !strings.Contains(s, "root cause + Loop:/Regression: lines (/fix, /hotfix)") {
		t.Error("ship pack Intent does not carry Loop:/Regression: lines")
	}
}

func TestShipPRBody_MergeRisk(t *testing.T) {
	s := syncedAsset(t, "skills/ship/references/pr-message-language.md")
	assertContainsAll(t, "pr-message-language.md", s, []string{
		"merge risk", "one-way", "two-way", "_Blast-radius", "_Rollback",
		"migration", "shared data store", "end users", "publish",
	})
}

func TestMergeBaseAndDeclined(t *testing.T) {
	ship := syncedAsset(t, "skills/ship/SKILL.md")
	assertContainsAll(t, "ship/SKILL.md", ship, []string{"git merge-base", "Declined:", "$BASE"})
	if strings.Contains(ship, "<base>") {
		t.Error("ship/SKILL.md still has <base> placeholder")
	}
	sdd := syncedAsset(t, "skills/subagent-driven-development/SKILL.md")
	assertContainsAll(t, "sdd/SKILL.md", sdd, []string{"git merge-base", "Ruling:"})
	assertContainsAll(t, "reviewer-doctrine.md", syncedAsset(t, "skills/review/_shared/reviewer-doctrine.md"),
		[]string{"vision document", "Declined:", "claim"})
	assertContainsAll(t, "code-reviewer-template.md", syncedAsset(t, "skills/subagent-driven-development/code-reviewer-template.md"),
		[]string{"vision document", "Declined:"})
	if strings.Contains(syncedAsset(t, "skills/review/_shared/finding-schema.md"), "Declined") {
		t.Error("finding-schema.md must not mention Declined (machine-checked schema)")
	}
}

func TestReviewSkill_FourGatesIntentOpusT3(t *testing.T) {
	s := syncedAsset(t, "skills/review/SKILL.md")
	assertContainsAll(t, "review/SKILL.md", s, []string{
		"PRE", "BUNDLE", "REVIEW", "VERIFY", "contracts / intent", "all five on opus",
		"floors at T2", "reviewer-doctrine.md", "review-verify",
	})
	for _, gone := range []string{
		"review-doctrine", "review-advise-gate", "review-log", "review-changes.js",
		"adviser-prompt", "post-gates", "Agent Workflow", "Workflow tool", "## Advisory", "contracts / types",
	} {
		if strings.Contains(s, gone) {
			t.Errorf("review/SKILL.md still contains %q", gone)
		}
	}
	fs := syncedAsset(t, "skills/review/_shared/finding-schema.md")
	assertContainsAll(t, "finding-schema.md", fs, []string{"intent", "user's stated goal", "review-verify"})
	for _, gone := range []string{"contracts | types", "skeptic"} {
		if strings.Contains(fs, gone) {
			t.Errorf("finding-schema.md still contains %q", gone)
		}
	}
	d := syncedAsset(t, "skills/review/_shared/reviewer-doctrine.md")
	assertContainsAll(t, "reviewer-doctrine.md", d, []string{"CLAUDE.md", ".claude/rules/", "rule's file:line"})
	lc := syncedAsset(t, "skills/review/references/lifecycle-and-t3.md")
	for _, gone := range []string{"POST", "review-advise-gate", "review-log", "review-changes.js"} {
		if strings.Contains(lc, gone) {
			t.Errorf("lifecycle-and-t3.md still contains %q", gone)
		}
	}
}
