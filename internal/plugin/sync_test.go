package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
)

func TestSyncMaterializesTree(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	res, err := Sync(dest, man)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, ".claude-plugin", "plugin.json")); err != nil {
		t.Fatalf("plugin.json was not written: %v", err)
	}
	if len(res.Written) == 0 {
		t.Fatal("Written is empty — no file was materialized")
	}
	res2, err := Sync(dest, man)
	if err != nil {
		t.Fatalf("Sync 2nd run: %v", err)
	}
	if len(res2.Written) != 0 {
		t.Fatalf("idempotent sync must have Written=0, got %d", len(res2.Written))
	}
}

func TestSyncNeverEscapesDest(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	m, _ := managed.Load(man)
	for p := range m.Entries {
		if !strings.HasPrefix(p, dest) {
			t.Fatalf("wrote outside dest: %s", p)
		}
	}
}

func TestSyncMaterializesDiscipline(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("sync: %v", err)
	}
	for _, p := range []string{
		"skills/discipline/SKILL.md",
		"hooks/session-start.sh",
	} {
		if _, err := os.Stat(filepath.Join(dest, p)); err != nil {
			t.Errorf("expected materialized %s: %v", p, err)
		}
	}
}

func TestSyncKeepsUserAddedFile(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	// user creates a file at an asset's path themselves BEFORE sync, outside the manifest
	victim := filepath.Join(dest, ".claude-plugin", "plugin.json")
	if err := os.MkdirAll(filepath.Dir(victim), 0o750); err != nil {
		t.Fatal(err)
	}
	userContent := []byte(`{"name":"user-edited"}`)
	if err := os.WriteFile(victim, userContent, 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := Sync(dest, man)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if got, _ := os.ReadFile(victim); string(got) != string(userContent) {
		t.Fatalf("user file was overwritten: %q", got)
	}
	var inKept bool
	for _, p := range res.Kept {
		if p == victim {
			inKept = true
		}
	}
	if !inKept {
		t.Fatalf("victim not found in Kept: %v", res.Kept)
	}
}

func TestSync_MaterializesReviewSchema(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "skills/review/_shared/finding-schema.md")); err != nil {
		t.Fatalf("finding-schema.md not materialized: %v", err)
	}
}

func TestSync_MaterializesReviewWorkflow(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "workflows/review-changes.js")); err != nil {
		t.Fatalf("review-changes.js not materialized: %v", err)
	}
}

func TestSync_MaterializesMechanicalGate(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "skills/review/scripts/mechanical-gate")); err != nil {
		t.Fatalf("mechanical-gate not materialized: %v", err)
	}
}

func TestSync_MaterializesReviewSkill(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	for _, p := range []string{"skills/review/SKILL.md", "skills/review/scripts/select-tier"} {
		if _, err := os.Stat(filepath.Join(dest, p)); err != nil {
			t.Fatalf("%s not materialized: %v", p, err)
		}
	}
}

// FR-6.1: Sync stamps the manifest with the running binary's version.
func TestSync_StampsVersion(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	m, err := managed.Load(man)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if m.Version != version.Current() {
		t.Fatalf("manifest version = %q, want %q", m.Version, version.Current())
	}
}

func TestSync_SchemaHasEvidenceField(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "skills/review/_shared/finding-schema.md"))
	if err != nil {
		t.Fatalf("read finding-schema.md: %v", err)
	}
	if !strings.Contains(string(b), "evidence") {
		t.Errorf("finding-schema.md missing field 'evidence'")
	}
}
