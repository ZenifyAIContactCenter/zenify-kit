package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
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

// FR-4: every playwright tool the verifier declares is auto-allowed by the kit,
// and it carries browser_set_storage_state so it can log itself in.
func TestUIVerifierToolsCoveredByKitAllowRules(t *testing.T) {
	s := readAsset(t, "assets/znf/agents/ui-verifier.md")
	var tools []string
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "tools:") {
			for _, f := range strings.Split(strings.TrimPrefix(line, "tools:"), ",") {
				tools = append(tools, strings.TrimSpace(f))
			}
			break
		}
	}
	if len(tools) == 0 {
		t.Fatal("no tools: line in ui-verifier.md")
	}
	allowed := map[string]bool{}
	for _, r := range apply.KitAllowRules {
		allowed[r] = true
	}
	hasState := false
	for _, tool := range tools {
		if tool == "mcp__playwright__browser_set_storage_state" {
			hasState = true
		}
		if strings.HasPrefix(tool, "mcp__playwright__") && !allowed[tool] {
			t.Errorf("tool %q is not in apply.KitAllowRules", tool)
		}
	}
	if !hasState {
		t.Error("ui-verifier tools must include mcp__playwright__browser_set_storage_state")
	}
}

// FR-4: the verifier never types a password; it logs in via `zenify e2e login`.
func TestUIVerifierNeverTypesPassword(t *testing.T) {
	s := readAsset(t, "assets/znf/agents/ui-verifier.md")
	if strings.Contains(s, "$E2E_PASSWORD") {
		t.Error("ui-verifier.md must not mention $E2E_PASSWORD")
	}
	if !strings.Contains(s, "zenify e2e login") {
		t.Error("ui-verifier.md must instruct `zenify e2e login`")
	}
}

func TestShipNotesNoMainSessionLogin(t *testing.T) {
	s := readAsset(t, "assets/znf/skills/ship/references/ui-verification-notes.md")
	if strings.Contains(s, "main session logs in itself") {
		t.Error("ui-verification-notes.md still has the main-session login flow")
	}
}

// The verifier's own Bash calls must be covered by the kit allow rules and must
// not use commands that prompt inside a background subagent.
func TestUIVerifierBashCoveredByKitAllowRules(t *testing.T) {
	s := readAsset(t, "assets/znf/agents/ui-verifier.md")
	if strings.Contains(s, "curl ") || strings.Contains(s, "command -v") || strings.Contains(s, "rm -f") {
		t.Error("ui-verifier.md must not use curl or command -v (uncovered Bash prompts)")
	}
	found := false
	for _, r := range apply.KitAllowRules {
		if r == "Bash(zenify ui-verify *)" {
			found = true
		}
	}
	if !found {
		t.Error("KitAllowRules must contain Bash(zenify ui-verify *)")
	}
}
