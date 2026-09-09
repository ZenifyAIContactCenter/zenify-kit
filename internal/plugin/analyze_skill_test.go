package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzeSkill_Materialized_HasKeyParts(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "skills/analyze/SKILL.md"))
	if err != nil {
		t.Fatalf("analyze/SKILL.md not materialized: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		"znf:_shared/constitution",  // cites constitution
		"znf:_shared/spec-template", // cites template
		"zenify analyze",            // calls the mechanical command
		"advisory",                  // declares non-blocking
		"SC-testable",               // judgment pass P3
		"necessity",                 // judgment pass P6
		"db-3",                      // judgment pass P7
		"_Blast-radius:",            // risk-metadata judgment pass (M6c1)
	} {
		if !strings.Contains(s, want) {
			t.Errorf("analyze/SKILL.md missing %q", want)
		}
	}
	// Agnostic: skill must NOT mandate mermaid.
	for _, forbidden := range []string{"mermaid"} {
		if strings.Contains(s, forbidden) {
			t.Errorf("analyze/SKILL.md must NOT contain %q (agnostic)", forbidden)
		}
	}
}

func TestAnalyzeSkill_CookWiring(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "skills/cook/SKILL.md"))
	if err != nil {
		t.Fatalf("read cook: %v", err)
	}
	s := string(b)
	for _, want := range []string{"znf:analyze", "advisory"} {
		if !strings.Contains(s, want) {
			t.Errorf("cook/SKILL.md missing %q (analyze wiring)", want)
		}
	}
}
