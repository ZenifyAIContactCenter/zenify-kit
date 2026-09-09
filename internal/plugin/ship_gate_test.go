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
	// Two active gates: the distributed-rules gate runs on every /ship (any repo),
	// and the full-tree gate runs when shipping the kit repo itself (the
	// language-retrofit milestone translated the kit's Go + skill assets, so this
	// is enforced now rather than deferred).
	if !strings.Contains(s, "zenify rules lint ~/.zenify/knowledge/.config/rules") {
		t.Error("ship gate must run the rules-lint scoped to the distributed rules dir")
	}
	if !strings.Contains(s, "zenify rules lint --include-go") {
		t.Error("ship gate must run the full-tree --include-go gate for the kit repo")
	}
}
