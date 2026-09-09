package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStandardsSkill_Materialized_HasKeyParts(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "skills/standards/SKILL.md"))
	if err != nil {
		t.Fatalf("standards/SKILL.md not materialized: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		"advisory",         // declares non-blocking
		"zenify standards", // calls the mechanical command
		"untested-fr",      // rubric
		"assert",           // judgment pass: does the test actually assert
	} {
		if !strings.Contains(s, want) {
			t.Errorf("standards/SKILL.md missing %q", want)
		}
	}
	if strings.Contains(s, "mermaid") {
		t.Errorf("standards/SKILL.md must NOT contain \"mermaid\" (agnostic)")
	}
}

func TestStandardsSkill_CookWiring(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "skills/cook/SKILL.md"))
	if err != nil {
		t.Fatalf("read cook: %v", err)
	}
	if !strings.Contains(string(b), "znf:standards") {
		t.Errorf("cook/SKILL.md missing znf:standards (Step 6b wiring)")
	}
}
