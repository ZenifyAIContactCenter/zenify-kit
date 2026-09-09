package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplainPlanSkill_Materialized_HasKeyParts(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "skills/explain-plan/SKILL.md"))
	if err != nil {
		t.Fatalf("explain-plan/SKILL.md not materialized: %v", err)
	}
	s := string(b)
	// SC-1: rubric tokens must be present.
	for _, want := range []string{
		"advisory",                  // advisory / non-blocking (FR-1.4)
		"COLLSCAN",                  // Mongo rubric
		`explain("executionStats")`, // how to run explain on Mongo
		"Seq Scan",                  // SQL rubric
		"db_read",                   // tool that runs explain
	} {
		if !strings.Contains(s, want) {
			t.Errorf("explain-plan/SKILL.md missing %q", want)
		}
	}
	// SC-2: agnostic — no mermaid, no project-specific collection names.
	for _, forbidden := range []string{"mermaid", "chat_rooms", "tickets"} {
		if strings.Contains(s, forbidden) {
			t.Errorf("explain-plan/SKILL.md must NOT contain %q (agnostic/public repo)", forbidden)
		}
	}
}

func TestExplainPlanSkill_ShipWiring(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "skills/ship/SKILL.md"))
	if err != nil {
		t.Fatalf("read ship: %v", err)
	}
	s := string(b)
	// SC-3: ship delegates to the skill AND keeps COLLSCAN as the trigger pointer.
	for _, want := range []string{"znf:explain-plan", "COLLSCAN"} {
		if !strings.Contains(s, want) {
			t.Errorf("ship/SKILL.md missing %q (explain-plan wiring)", want)
		}
	}
}

func TestExplainPlanSkill_GroundWiring(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "skills/ground/SKILL.md"))
	if err != nil {
		t.Fatalf("read ground: %v", err)
	}
	if !strings.Contains(string(b), "znf:explain-plan") {
		t.Errorf("ground/SKILL.md missing znf:explain-plan (shift-left wiring)")
	}
}
