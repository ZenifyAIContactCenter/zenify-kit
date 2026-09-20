package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Project business terms that must never appear in a kit asset (kit stays
// project-agnostic). Superset of the lists in ui_verifier_test.go and
// discipline_test.go.
var researchForbidden = []string{"contact-center", "3csoft", "herdr", "ott-gateway", "db_read", "lumi", "personal-zalo", "notification-hub"}

func syncedAsset(t *testing.T, rel string) string {
	t.Helper()
	dest := t.TempDir()
	if _, err := Sync(dest, filepath.Join(dest, ".manifest.json")); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, rel))
	if err != nil {
		t.Fatalf("%s not materialized: %v", rel, err)
	}
	return string(b)
}

func assertContainsAll(t *testing.T, name, s string, wants []string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(s, w) {
			t.Errorf("%s missing %q", name, w)
		}
	}
}

func assertProjectAgnostic(t *testing.T, name, s string) {
	t.Helper()
	low := strings.ToLower(s)
	for _, f := range researchForbidden {
		if strings.Contains(low, f) {
			t.Errorf("%s must stay project-agnostic, found %q", name, f)
		}
	}
}

// FR-2: researcher agent — sonnet, web-only allowlist, two modes, citation contract.
func TestResearcherAgent_Shipped(t *testing.T) {
	s := syncedAsset(t, "agents/researcher.md")
	assertContainsAll(t, "researcher.md", s, []string{
		"name: researcher",
		"model: sonnet",
		"tools: WebSearch, WebFetch, Read, Write, ToolSearch",
		"mode: verify",
		"According to",
		"not found",
		"VERIFIED",
		"QUOTE-MISMATCH",
		"DEAD-URL",
		"40 lines",
	})
	if strings.Contains(s, "tools: ") && strings.Contains(strings.SplitN(s, "\n---", 2)[0], "Agent") {
		t.Error("researcher.md frontmatter must not grant the Agent tool (workers do not fan out)")
	}
	assertProjectAgnostic(t, "researcher.md", s)
}

// FR-1: research skill — forked lead, sonnet workers, haiku verifier, file output, 40-line return.
func TestResearchSkill_Shipped(t *testing.T) {
	s := syncedAsset(t, "skills/research/SKILL.md")
	assertContainsAll(t, "research/SKILL.md", s, []string{
		"name: research",
		"context: fork",
		"background: false",
		"$ARGUMENTS",
		"znf:researcher",
		"model: sonnet",
		"model: haiku",
		"mode: verify",
		"znf-research-",
		"verify.md",
		"docs/reference/",
		"not found",
		"40 lines",
		"references/output-contract.md",
		"references/scale-and-cost.md",
		"\n## References",
	})
	if strings.Contains(s, "AskUserQuestion") {
		t.Error("research/SKILL.md runs forked: it must not reach for AskUserQuestion")
	}
	assertProjectAgnostic(t, "research/SKILL.md", s)
	for _, ref := range []string{"references/output-contract.md", "references/scale-and-cost.md"} {
		r := syncedAsset(t, "skills/research/"+ref)
		if strings.Contains(r, "references/") {
			t.Errorf("%s must not link into references/ (one-level rule)", ref)
		}
		assertProjectAgnostic(t, ref, r)
	}
}

// FR-3: brainstorming owns the research trigger — one checklist step, linked reference, budget kept.
func TestBrainstormingResearchCheck(t *testing.T) {
	s := syncedAsset(t, "skills/brainstorming/SKILL.md")
	assertContainsAll(t, "brainstorming/SKILL.md", s, []string{
		"**Research check**",
		"references/research-checklist.md",
		"Skill(znf:research)",
		// guarded literals from spec_discipline_test.go — a prose cut must not lose them
		"znf:_shared/constitution",
		"znf:_shared/spec-template",
		"Clarify-lite",
	})
	if len(s) > maxSkillBytes {
		t.Errorf("brainstorming/SKILL.md is %d bytes; budget %d", len(s), maxSkillBytes)
	}
	c := syncedAsset(t, "skills/brainstorming/references/research-checklist.md")
	assertContainsAll(t, "research-checklist.md", c, []string{"1.", "2.", "3.", "4.", "5.", "znf:ground", "znf:scout", "docs/reference"})
	if strings.Contains(c, "references/") {
		t.Error("research-checklist.md must not link into references/ (one-level rule)")
	}
	assertProjectAgnostic(t, "research-checklist.md", c)
}
