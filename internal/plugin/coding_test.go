package plugin

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

var forbiddenTokens = []string{
	"3csoft", "contact-center", "ott-gateway", "db_read", "lumi",
	"personal-zalo", "notification-hub", "models.mongo", "sendOK",
	"VITE_", "namph", "zenify",
}

// The seven former leg-1 coding skills, now shipped inside the znf plugin
// (2026-09-20). skill dir -> lowercase anchors that must appear in SKILL.md.
var codingAnchors = map[string][]string{
	"mongo-data-safety":        {"tenant", "strict", "distinct"},
	"sql-data-safety":          {"tenant", "parameter", "pool"},
	"mongoose-modeling":        {"schema", "index", "backfill"},
	"nestjs-patterns":          {"module", "guard", "controller"},
	"express-service-patterns": {"middleware", "controller", "envelope"},
	"react-patterns":           {"component", "hook", "form"},
	"service-integration":      {"idempoten", "publish", "contract"},
}

// TestCodingSkillsAreAgnostic: the stack skills stay project-agnostic (no
// workspace token) and keep their anchors, in SKILL.md and in references/.
func TestCodingSkillsAreAgnostic(t *testing.T) {
	for skill, anchors := range codingAnchors {
		dir := path.Join(embedRoot, "skills", skill)
		body := readAsset(t, path.Join(dir, "SKILL.md"))
		low := strings.ToLower(body)
		for _, a := range anchors {
			if !strings.Contains(low, a) {
				t.Errorf("skill %s missing anchor %q", skill, a)
			}
		}
		texts := []string{low}
		if refs, err := fs.ReadDir(assets, path.Join(dir, "references")); err == nil {
			for _, r := range refs {
				texts = append(texts, strings.ToLower(readAsset(t, path.Join(dir, "references", r.Name()))))
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
		for _, tok := range forbiddenTokens {
			for _, txt := range texts {
				if strings.Contains(txt, strings.ToLower(tok)) {
					t.Errorf("skill %s contains forbidden token %q", skill, tok)
				}
			}
		}
	}
}

// SC-2: Sync materializes the seven under ~/.claude/skills/znf/skills/.
func TestSyncMaterializesCodingSkills(t *testing.T) {
	dest := t.TempDir()
	man := filepath.Join(dest, ".manifest.json")
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	for skill := range codingAnchors {
		if _, err := os.Stat(filepath.Join(dest, "skills", skill, "SKILL.md")); err != nil {
			t.Errorf("%s not materialized by Sync: %v", skill, err)
		}
	}
}
