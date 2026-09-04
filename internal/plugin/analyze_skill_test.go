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
		t.Fatalf("analyze/SKILL.md chưa materialize: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		"znf:_shared/constitution",   // cite constitution
		"znf:_shared/spec-template",  // cite template
		"zenify analyze",             // gọi command cơ học
		"advisory",                   // khai không-chặn
		"SC-testable",                // pass phán đoán P3
		"necessity",                  // pass phán đoán P6
		"db-3",                       // pass phán đoán P7
	} {
		if !strings.Contains(s, want) {
			t.Errorf("analyze/SKILL.md thiếu %q", want)
		}
	}
	// Agnostic: skill KHÔNG mandate mermaid.
	for _, forbidden := range []string{"mermaid"} {
		if strings.Contains(s, forbidden) {
			t.Errorf("analyze/SKILL.md KHÔNG được chứa %q (agnostic)", forbidden)
		}
	}
}
