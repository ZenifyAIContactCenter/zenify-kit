package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2eSkill_Materialized(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "skills/e2e/SKILL.md"))
	if err != nil {
		t.Fatalf("e2e/SKILL.md not materialized: %v", err)
	}
	if !strings.Contains(string(b), "@domain-assert") {
		t.Error("e2e/SKILL.md must teach the @domain-assert marker")
	}
}

func TestE2eSkill_CookAndShipWiring(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	// Step 5 (Plan) decides whether a task needs an E2E journey, alongside the visual browser-run decision.
	cook, err := os.ReadFile(filepath.Join(dest, "skills/cook/SKILL.md"))
	if err != nil {
		t.Fatalf("cook/SKILL.md not materialized: %v", err)
	}
	if !strings.Contains(string(cook), "decide the E2E journey") {
		t.Error("cook/SKILL.md missing the e2e hook at Step 5 ('decide the E2E journey')")
	}
	// The ship lint step runs `zenify e2e lint` when the repo carries `.znf/e2e/`.
	ship, err := os.ReadFile(filepath.Join(dest, "skills/ship/SKILL.md"))
	if err != nil {
		t.Fatalf("ship/SKILL.md not materialized: %v", err)
	}
	if !strings.Contains(string(ship), "zenify e2e lint") {
		t.Error("ship/SKILL.md missing the 'zenify e2e lint' hook")
	}
}
