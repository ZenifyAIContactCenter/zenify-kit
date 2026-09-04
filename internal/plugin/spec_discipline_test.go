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
