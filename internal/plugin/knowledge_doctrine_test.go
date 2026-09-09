package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKnowledgeDoctrine_Materialized(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "skills/_shared/knowledge-doctrine.md"))
	if err != nil {
		t.Fatalf("knowledge-doctrine.md not materialized: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		"**Last updated:**",
		// SC-1: three layers + placement test
		"L1 — auto memory", "L2 — CLAUDE.md", "L3 — skill-rule",
		"Placement test", "lowest layer",
		// SC-2: promotion
		"Promotion", "MOVE, not a COPY", "M6d",
		// SC-3: retirement references prune-memory
		"Retirement", "prune-memory",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("knowledge-doctrine.md missing %q", want)
		}
	}
	for _, forbidden := range []string{"mermaid", "tiếng Việt", "Vietnamese"} { //znf:allow-lang -- literal being tested for absence, not translatable
		if strings.Contains(s, forbidden) {
			t.Errorf("knowledge-doctrine.md must NOT contain %q (English-only agent file)", forbidden)
		}
	}
}

func TestKnowledgeDoctrine_RuleSystemExtension(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "skills/_shared/knowledge-doctrine.md"))
	if err != nil {
		t.Fatalf("read knowledge-doctrine.md: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		"L4 — mechanical gate", // the new gate layer
		"Team-reach",           // the reach axis
		"flat store",           // memory is one flat per-user store
		"F3 > F2 > F1",         // promotion order for machine-checkable norms
		"Ratify",               // nominate→ratify gate
		"rule-candidates",      // where a candidate is written
	} {
		if !strings.Contains(s, want) {
			t.Errorf("knowledge-doctrine.md missing %q", want)
		}
	}
	// English-only guard stays intact.
	for _, bad := range []string{"mermaid", "Vietnamese"} {
		if strings.Contains(strings.ToLower(s), strings.ToLower(bad)) {
			t.Errorf("knowledge-doctrine.md must not contain %q", bad)
		}
	}
}
