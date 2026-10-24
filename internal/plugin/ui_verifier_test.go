package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
)

// FR-7: the kit ships the ui-verifier agent cook/ship dispatch (sonnet,
// project-agnostic, drives its own `zenify browser` session).
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
	for _, want := range []string{"name: ui-verifier", "model: sonnet", "zenify browser", "state-load", "close", "Run it as the whole command, nothing appended"} {
		if !strings.Contains(s, want) {
			t.Errorf("ui-verifier.md missing %q", want)
		}
	}
	for _, bad := range []string{"mcp__", "single shared instance"} {
		if strings.Contains(s, bad) {
			t.Errorf("ui-verifier.md must not contain %q", bad)
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

// FR-4: every tool the verifier declares is a built-in or auto-allowed by the kit.
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
	allowed := map[string]bool{"Bash": true, "Read": true, "ToolSearch": true}
	for _, r := range apply.KitAllowRules {
		allowed[r] = true
	}
	for _, tool := range tools {
		if !allowed[tool] {
			t.Errorf("tool %q is neither Bash/Read/ToolSearch nor in apply.KitAllowRules", tool)
		}
	}
}

// No asset may still describe the retired shared-MCP-browser model.
func TestNoSharedBrowserRule(t *testing.T) {
	banned := []string{"single shared", "never two browser", "mcp__playwright", "browser_take_screenshot", "browser_set_storage_state"}
	err := filepath.WalkDir("assets/znf", func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		for _, w := range banned {
			if strings.Contains(strings.ToLower(string(b)), w) {
				t.Errorf("%s still contains %q", p, w)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
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
	for _, want := range []string{"Bash(zenify browser *)", "Bash(zenify ui-verify *)"} {
		found := false
		for _, r := range apply.KitAllowRules {
			if r == want {
				found = true
			}
		}
		if !found {
			t.Errorf("KitAllowRules must contain %s", want)
		}
	}
}
