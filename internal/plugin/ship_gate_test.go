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
	s := string(b)
	if !strings.Contains(s, "zenify rules lint") {
		t.Error("ship SKILL.md missing the rules-lint gate")
	}
	// The gate that /ship actually runs is scoped to the distributed rules,
	// not the full kit tree. (The doc separately names --include-go when
	// explaining the deferral, so assert the active command is the scoped one
	// rather than the mere absence of the flag anywhere in the file.)
	if !strings.Contains(s, "zenify rules lint ~/.zenify/knowledge/.config/rules") {
		t.Error("ship gate must run the rules-lint scoped to the distributed rules dir")
	}
}
