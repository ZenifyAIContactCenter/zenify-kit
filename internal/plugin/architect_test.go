package plugin

import (
	"strings"
	"testing"
)

func TestArchitectAgent_Shipped(t *testing.T) {
	s := syncedAsset(t, "agents/architect.md")
	assertContainsAll(t, "architect.md", s, []string{
		"name: architect", "model: opus", "tools: Read, Grep, Glob, Bash, Agent, Write",
		"architect-memo.md", "changed_decision:", "at most 40 lines", "znf:researcher", "model: sonnet",
		"Do not write code", "Simplicity", "Anti-Abstraction", "Integration-First", "innovation token",
	})
	for _, banned := range []string{"effort:", "fable", "Fable"} {
		if strings.Contains(s, banned) {
			t.Errorf("architect.md must not contain %q", banned)
		}
	}
	assertProjectAgnostic(t, "architect.md", s)
	if len(s) > 8192 {
		t.Errorf("architect.md is %d bytes; keep the persona under the skill cap", len(s))
	}
}

func TestBrainstorming_ArchitectGate(t *testing.T) {
	s := syncedAsset(t, "skills/brainstorming/SKILL.md")
	assertContainsAll(t, "brainstorming/SKILL.md", s, []string{
		"select-route architect", "architect-gate.md", "architect-memo.md", "Agent(architect)",
		"Think hard before responding.", "changed_decision",
	})
	if len(s) > 8192 || strings.Count(s, "\n") > 200 {
		t.Fatalf("brainstorming over cap: %d bytes", len(s))
	}
	memo := syncedAsset(t, "skills/brainstorming/references/architect-memo.md")
	assertContainsAll(t, "architect-memo.md", memo, []string{
		"Context and Scope", "Goals and Non-Goals", "The Actual Design", "Alternatives Considered", "Cross-Cutting Concerns",
		"Simplicity Gate", "Anti-Abstraction Gate", "Integration-First Gate", "Innovation tokens", "Polyrepo checklist",
		"Strong | Worth exploring | Speculative", "changed_decision", "deliberately NOT",
	})
	gate := syncedAsset(t, "skills/brainstorming/references/architect-gate.md")
	assertContainsAll(t, "architect-gate.md", gate, []string{"REPOS", "SHARED", "NEW_CONTRACT", "CRITICAL", "route-log record", "site\":\"architect"})
}
