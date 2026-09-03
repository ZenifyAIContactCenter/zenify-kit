package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewSkill_Materialized_HasKeyParts(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "skills/review/SKILL.md"))
	if err != nil {
		t.Fatalf("review/SKILL.md chưa materialize: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		"select-tier",    // gọi script tier
		"T1", "T2", "T3", // 3 tier
		"PRE", "BUNDLE", "REVIEW", "VERIFY", "POST", // 5 seam
		"degrade",           // fallback T3->T2
		"finding-schema.md", // tham chiếu schema chung
	} {
		if !strings.Contains(s, want) {
			t.Errorf("review/SKILL.md thiếu %q", want)
		}
	}
}

func TestShipStep5_DelegatesToReview(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "skills/ship/SKILL.md"))
	if err != nil {
		t.Fatalf("ship/SKILL.md: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, "znf:review") {
		t.Error("ship step 5 chưa delegate sang znf:review")
	}
}
