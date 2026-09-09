package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandoffDoctrine_Materialized(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "skills/_shared/handoff-doctrine.md"))
	if err != nil {
		t.Fatalf("handoff-doctrine.md not materialized: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		"**Last updated:**",
		// SC-1: two kinds
		"session-handoff", "milestone-record",
		// SC-1: reference-not-copy (twin of MOVE-not-COPY)
		"Reference, not copy", "MOVE",
		// SC-1: retirement + a boundary trigger term
		"Retirement", "compaction",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("handoff-doctrine.md missing %q", want)
		}
	}
	for _, forbidden := range []string{"mermaid", "tiếng Việt", "Vietnamese"} { //znf:allow-lang -- literal being tested for absence, not translatable
		if strings.Contains(s, forbidden) {
			t.Errorf("handoff-doctrine.md must NOT contain %q (English-only agent file)", forbidden)
		}
	}
}

func TestHandoffDoctrine_Wired(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	// SC-2: constitution ## Governance cites the handoff doctrine.
	con, err := os.ReadFile(filepath.Join(dest, "skills/_shared/constitution.md"))
	if err != nil {
		t.Fatalf("constitution.md not materialized: %v", err)
	}
	if !strings.Contains(string(con), "handoff-doctrine") {
		t.Error("constitution.md ## Governance missing pointer 'handoff-doctrine' (SC-2)")
	}
	// SC-3: the branch-finish seam cites the doctrine — a real firing context.
	fin, err := os.ReadFile(filepath.Join(dest, "skills/finishing-a-development-branch/SKILL.md"))
	if err != nil {
		t.Fatalf("finishing-a-development-branch/SKILL.md not materialized: %v", err)
	}
	if !strings.Contains(string(fin), "handoff-doctrine") {
		t.Error("finishing-a-development-branch/SKILL.md missing cite 'handoff-doctrine' (SC-3)")
	}
}
