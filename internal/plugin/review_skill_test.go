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

func TestReviewSkill_M4bWiring(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	skill, err := os.ReadFile(filepath.Join(dest, "skills/review/SKILL.md"))
	if err != nil {
		t.Fatalf("đọc SKILL.md: %v", err)
	}
	s := string(skill)
	for _, want := range []string{"mechanical-gate", "review-verify", "evidence", "Bash(zenify *)"} {
		if !strings.Contains(s, want) {
			t.Errorf("SKILL.md thiếu %q (M4b wiring)", want)
		}
	}
	wf, err := os.ReadFile(filepath.Join(dest, "workflows/review-changes.js"))
	if err != nil {
		t.Fatalf("đọc review-changes.js: %v", err)
	}
	if !strings.Contains(string(wf), "evidence") {
		t.Errorf("review-changes.js chưa yêu cầu evidence")
	}
}

func TestReviewSkill_M4cWiring(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	skill := filepath.Join(dest, "skills", "review", "SKILL.md")
	b, err := os.ReadFile(skill)
	if err != nil {
		t.Fatalf("đọc SKILL.md: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		"review-bundle", // gọi subcommand bundler
		"BUNDLE",        // seam có tên
		"2000",          // ngưỡng kích hoạt
		"too-large",     // nhánh dừng "tách PR"
		"per-bundle",    // review từng bundle
	} {
		if !strings.Contains(s, want) {
			t.Errorf("SKILL.md thiếu wiring M4c: %q", want)
		}
	}
}

func TestReviewSkill_M4dWiring(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("sync: %v", err)
	}
	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join(dest, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		return string(b)
	}
	skill := read("skills/review/SKILL.md")
	for _, want := range []string{"review-doctrine", "DOCTRINE", "reviewer-doctrine.md", "args.doctrine"} {
		if !strings.Contains(skill, want) {
			t.Errorf("SKILL.md thiếu %q", want)
		}
	}
	doc := read("skills/review/_shared/reviewer-doctrine.md")
	if !strings.Contains(strings.ToLower(doc), "claim") {
		t.Error("reviewer-doctrine.md thiếu wording no-claim")
	}
	wf := read("workflows/review-changes.js")
	if !strings.Contains(wf, "args.doctrine") || !strings.Contains(wf, "DOCTRINE") {
		t.Error("review-changes.js thiếu wiring args.doctrine")
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
