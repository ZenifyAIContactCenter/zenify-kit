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
		t.Fatalf("knowledge-doctrine.md chưa materialize: %v", err)
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
			t.Errorf("knowledge-doctrine.md thiếu %q", want)
		}
	}
	for _, forbidden := range []string{"mermaid", "tiếng Việt", "Vietnamese"} {
		if strings.Contains(s, forbidden) {
			t.Errorf("knowledge-doctrine.md KHÔNG được chứa %q (English-only agent file)", forbidden)
		}
	}
}
