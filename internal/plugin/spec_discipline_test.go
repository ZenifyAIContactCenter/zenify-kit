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
		t.Fatalf("constitution.md chưa materialize: %v", err)
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
	} {
		if !strings.Contains(s, want) {
			t.Errorf("constitution.md thiếu %q", want)
		}
	}
	for _, forbidden := range []string{"Sync Impact Report", "Vietnamese", "tiếng Việt", "mermaid"} {
		if strings.Contains(s, forbidden) {
			t.Errorf("constitution.md KHÔNG được chứa %q (agnostic/lean)", forbidden)
		}
	}
	if strings.Contains(s, "1.0.0") || strings.Contains(s, "**Version:**") {
		t.Error("constitution.md KHÔNG được mang semver (version = git history)")
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
		t.Fatalf("spec-template.md chưa materialize: %v", err)
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
	} {
		if !strings.Contains(s, want) {
			t.Errorf("spec-template.md thiếu %q", want)
		}
	}
	for _, forbidden := range []string{"mermaid", "Vietnamese", "tiếng Việt"} {
		if strings.Contains(s, forbidden) {
			t.Errorf("spec-template.md KHÔNG được hardcode %q", forbidden)
		}
	}
}
