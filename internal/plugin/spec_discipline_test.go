package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpecDiscipline_Constitution(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "skills/_shared/constitution.md"))
	if err != nil {
		t.Fatalf("constitution.md not materialized: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		"**Last updated:**",
		"artifact-style",
		"P1", "P2", "P3", "P4", "P5", "P6", "P7", "P8",
		"Necessity ladder", "ceiling", "trigger",
		"Comprehension floor",
		"Traceability", "_Requirements:",
		"Governance",
		"P9", "Risk-metadata", "_Blast-radius:", "_Rollback:",
		"knowledge-doctrine",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("constitution.md missing %q", want)
		}
	}
	for _, forbidden := range []string{"Sync Impact Report", "Vietnamese", "tiếng Việt", "mermaid"} { //znf:allow-lang -- literal being tested for absence, not translatable
		if strings.Contains(s, forbidden) {
			t.Errorf("constitution.md must NOT contain %q (agnostic/lean)", forbidden)
		}
	}
	if strings.Contains(s, "1.0.0") || strings.Contains(s, "**Version:**") {
		t.Error("constitution.md must NOT carry semver (version = git history)")
	}
}

func TestSpecDiscipline_Template(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "skills/_shared/spec-template.md"))
	if err != nil {
		t.Fatalf("spec-template.md not materialized: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		"Problem", "Approach", "Timing", "Phase", "blast-radius",
		"DB guarantees", "Flow",
		"Mini-brief", "Non-goals", "ceiling", "trigger",
		"FR-1", "FR-1.1", "As a", "so that",
		"SC-1", "Given", "When", "Then",
		"EARS note", "the system SHALL",
		"smallest check", "artifact-style", "[NEEDS CLARIFICATION",
		"Rollback", "_Blast-radius:", "_DB:", "_Rollback:",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("spec-template.md missing %q", want)
		}
	}
	for _, forbidden := range []string{"mermaid", "Vietnamese", "tiếng Việt"} { //znf:allow-lang -- literal being tested for absence, not translatable
		if strings.Contains(s, forbidden) {
			t.Errorf("spec-template.md must NOT hardcode %q", forbidden)
		}
	}
}

func TestSpecDiscipline_BrainstormingWiring(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "skills/brainstorming/SKILL.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		"znf:_shared/constitution",
		"znf:_shared/spec-template",
		"Clarify-lite",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("brainstorming/SKILL.md missing %q", want)
		}
	}
}

func TestSpecDiscipline_PlanWiring(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "skills/writing-plans/SKILL.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		"znf:_shared/artifact-style",
		"znf:_shared/constitution",
		"_Requirements:",      // traceability tag (P8)
		"Simpler alternative", // necessity note (Complexity Tracking)
	} {
		if !strings.Contains(s, want) {
			t.Errorf("writing-plans/SKILL.md missing %q (plan-half)", want)
		}
	}
}

func TestSpecDiscipline_CookWiring(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "skills/cook/SKILL.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	s := string(b)
	// Both assets must be cited at least once in cook (step 2 spec + step 5 plan)
	for _, want := range []string{"znf:_shared/constitution", "znf:_shared/spec-template"} {
		if !strings.Contains(s, want) {
			t.Errorf("cook/SKILL.md missing %q", want)
		}
	}
}
