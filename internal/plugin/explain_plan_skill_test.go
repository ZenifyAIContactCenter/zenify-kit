package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplainPlanSkill_Materialized_HasKeyParts(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "skills/explain-plan/SKILL.md"))
	if err != nil {
		t.Fatalf("explain-plan/SKILL.md chưa materialize: %v", err)
	}
	s := string(b)
	// SC-1: rubric tokens phải có mặt.
	for _, want := range []string{
		"advisory",                  // advisory / không chặn (FR-1.4)
		"COLLSCAN",                  // rubric Mongo
		`explain("executionStats")`, // cách chạy explain Mongo
		"Seq Scan",                  // rubric SQL
		"db_read",                   // tool chạy explain
	} {
		if !strings.Contains(s, want) {
			t.Errorf("explain-plan/SKILL.md thiếu %q", want)
		}
	}
	// SC-2: agnostic — không mermaid, không tên collection project cụ thể.
	for _, forbidden := range []string{"mermaid", "chat_rooms", "tickets"} {
		if strings.Contains(s, forbidden) {
			t.Errorf("explain-plan/SKILL.md KHÔNG được chứa %q (agnostic/public repo)", forbidden)
		}
	}
}

func TestExplainPlanSkill_ShipWiring(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "skills/ship/SKILL.md"))
	if err != nil {
		t.Fatalf("read ship: %v", err)
	}
	s := string(b)
	// SC-3: ship delegate vào skill VÀ giữ COLLSCAN làm trigger-pointer.
	for _, want := range []string{"znf:explain-plan", "COLLSCAN"} {
		if !strings.Contains(s, want) {
			t.Errorf("ship/SKILL.md thiếu %q (explain-plan wiring)", want)
		}
	}
}

func TestExplainPlanSkill_GroundWiring(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "skills/ground/SKILL.md"))
	if err != nil {
		t.Fatalf("read ground: %v", err)
	}
	if !strings.Contains(string(b), "znf:explain-plan") {
		t.Errorf("ground/SKILL.md thiếu znf:explain-plan (shift-left wiring)")
	}
}
