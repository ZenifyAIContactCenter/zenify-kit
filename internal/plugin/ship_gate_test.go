package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShipGate_RunsRulesLint(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(dest, "skills/ship/SKILL.md"))
	if !strings.Contains(string(b), "zenify rules lint") {
		t.Error("ship SKILL.md missing the rules-lint gate")
	}
}
