package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Project business terms that must never appear in a kit asset (kit stays
// project-agnostic). Superset of the lists in ui_verifier_test.go and
// discipline_test.go.
var researchForbidden = []string{"contact-center", "3csoft", "herdr", "ott-gateway", "db_read", "lumi", "personal-zalo", "notification-hub"}

func syncedAsset(t *testing.T, rel string) string {
	t.Helper()
	dest := t.TempDir()
	if _, err := Sync(dest, filepath.Join(dest, ".manifest.json")); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, rel))
	if err != nil {
		t.Fatalf("%s not materialized: %v", rel, err)
	}
	return string(b)
}

func assertContainsAll(t *testing.T, name, s string, wants []string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(s, w) {
			t.Errorf("%s missing %q", name, w)
		}
	}
}

func assertProjectAgnostic(t *testing.T, name, s string) {
	t.Helper()
	low := strings.ToLower(s)
	for _, f := range researchForbidden {
		if strings.Contains(low, f) {
			t.Errorf("%s must stay project-agnostic, found %q", name, f)
		}
	}
}

// FR-2: researcher agent — sonnet, web-only allowlist, two modes, citation contract.
func TestResearcherAgent_Shipped(t *testing.T) {
	s := syncedAsset(t, "agents/researcher.md")
	assertContainsAll(t, "researcher.md", s, []string{
		"name: researcher",
		"model: sonnet",
		"tools: WebSearch, WebFetch, Read, Write, ToolSearch",
		"mode: verify",
		"According to",
		"not found",
		"VERIFIED",
		"QUOTE-MISMATCH",
		"DEAD-URL",
		"40 lines",
	})
	if strings.Contains(s, "tools: ") && strings.Contains(strings.SplitN(s, "\n---", 2)[0], "Agent") {
		t.Error("researcher.md frontmatter must not grant the Agent tool (workers do not fan out)")
	}
	assertProjectAgnostic(t, "researcher.md", s)
}
