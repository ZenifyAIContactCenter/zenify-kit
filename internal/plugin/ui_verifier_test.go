package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// FR-7: the kit ships the ui-verifier agent cook/ship dispatch (sonnet,
// project-agnostic, single shared Playwright browser).
func TestUIVerifierAgent_Shipped(t *testing.T) {
	dest := t.TempDir()
	if _, err := Sync(dest, filepath.Join(dest, ".manifest.json")); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "agents", "ui-verifier.md"))
	if err != nil {
		t.Fatalf("agents/ui-verifier.md not materialized: %v", err)
	}
	s := string(b)
	for _, want := range []string{"name: ui-verifier", "model: sonnet", "single shared instance", "mcp__playwright__browser_snapshot"} {
		if !strings.Contains(s, want) {
			t.Errorf("ui-verifier.md missing %q", want)
		}
	}
	// "zenify" itself is allowed: the agent optionally shells out to the kit's own
	// `zenify ui-verify record` CLI, guarded on `command -v zenify` so a project without
	// it on PATH simply skips the step — that guard is what keeps it project-agnostic.
	// "contact-center"/"3csoft" would be actual project business terms and stay forbidden.
	for _, forbidden := range []string{"contact-center", "3csoft"} {
		if strings.Contains(strings.ToLower(s), forbidden) {
			t.Errorf("ui-verifier.md must stay project-agnostic, found %q", forbidden)
		}
	}
	// ship must dispatch the plugin agent, not a personal path
	ship, _ := os.ReadFile(filepath.Join(dest, "skills", "ship", "SKILL.md"))
	if strings.Contains(string(ship), "~/.claude/agents/ui-verifier.md") {
		t.Error("ship/SKILL.md still points at the personal agent path")
	}
	if !strings.Contains(string(ship), "znf:ui-verifier") {
		t.Error("ship/SKILL.md does not name znf:ui-verifier")
	}
}
