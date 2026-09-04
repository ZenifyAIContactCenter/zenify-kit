package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPolyrepoDoctrine_WritingPlans(t *testing.T) {
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
	for _, want := range []string{"Waits for:", "contract-frozen", "independent"} {
		if !strings.Contains(s, want) {
			t.Errorf("writing-plans/SKILL.md thiếu %q (polyrepo DAG)", want)
		}
	}
}

func TestPolyrepoDoctrine_Cook(t *testing.T) {
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
	for _, want := range []string{"Cross-worktree parallelism", "contract-frozen"} {
		if !strings.Contains(s, want) {
			t.Errorf("cook/SKILL.md thiếu %q (polyrepo pointer)", want)
		}
	}
}
