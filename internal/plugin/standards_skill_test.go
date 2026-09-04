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
		t.Fatalf("standards/SKILL.md chưa materialize: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		"advisory",         // khai không-chặn
		"zenify standards", // gọi command cơ học
		"untested-fr",      // rubric
		"assert",           // pass phán đoán: test có thật sự assert
	} {
		if !strings.Contains(s, want) {
			t.Errorf("standards/SKILL.md thiếu %q", want)
		}
	}
	if strings.Contains(s, "mermaid") {
		t.Errorf("standards/SKILL.md KHÔNG được chứa \"mermaid\" (agnostic)")
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
		t.Errorf("cook/SKILL.md thiếu znf:standards (Step 6b wiring)")
	}
}
