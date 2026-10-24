package plugin

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

// FR-1..FR-6.2, SC-2, SC-7: top-issues surveys and ranks, hands one issue to fix, never fixes, and
// carries no workspace-specific name.
func TestTopIssuesSkill(t *testing.T) {
	s := syncedAsset(t, "skills/top-issues/SKILL.md")
	assertContainsAll(t, "top-issues/SKILL.md", s, []string{
		"name: top-issues",
		"allowed-tools: Read Write Bash(zenify catalog*) Bash(zenify obs*)",
		"`Skill(znf:fix)`",
		"/znf:hotfix",
		"_shared/evidence-sources.md",
		"references/scoring.md",
		"references/queries.md",
		"docs/exports/_cross/",
		"no alert rules",
		"40 MCP calls",
	})
	for _, bad := range []string{"disable-model-invocation", "Skill(znf:hotfix)", "edit the code", "apply the fix"} {
		if strings.Contains(s, bad) {
			t.Errorf("top-issues/SKILL.md must not contain %q", bad)
		}
	}

	sc := syncedAsset(t, "skills/top-issues/references/scoring.md")
	assertContainsAll(t, "top-issues/references/scoring.md", sc, []string{
		"Money path", "External", "Tenant users", "Internal",
		"Local", "Several", "Widespread",
		"unknown",
		"| 5 | P0 | P1 | P1 |", "| 1 | P3 | P3 | P3 |",
		"GET",
	})

	q := syncedAsset(t, "skills/top-issues/references/queries.md")
	assertContainsAll(t, "top-issues/references/queries.md", q, []string{
		"offset", "count_over_time() by (span.<tenant_attr>)", "query_loki_patterns", "1h",
	})

	dest := t.TempDir()
	if _, err := Sync(dest, filepath.Join(dest, ".manifest.json")); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	forbidden := regexp.MustCompile(`ady8|dfh52|ffk50|ccbe|cch-hub|link_trace_id|messaging\.producer|bullmq:|192\.168|docker logs`)
	uidLike := regexp.MustCompile(`\b[a-z0-9]{14}\b`)
	files := 0
	err := filepath.WalkDir(filepath.Join(dest, "skills", "top-issues"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files++
		body := string(b)
		if m := forbidden.FindString(body); m != "" {
			t.Errorf("%s carries workspace-specific string %q (SC-2)", p, m)
		}
		if strings.Contains(body, `=~".+"`) {
			t.Errorf("%s carries the match-everything Loki selector", p)
		}
		// A 14-char English word ("implementation") is fine; a datasource uid mixes in digits.
		for _, tok := range uidLike.FindAllString(body, -1) {
			if strings.ContainsAny(tok, "0123456789") {
				t.Errorf("%s carries uid-like token %q (SC-2)", p, tok)
			}
		}
		for _, r := range body {
			if unicode.IsLetter(r) && r > 127 {
				t.Errorf("%s carries non-ASCII letter %q; skill files are English only", p, r)
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk top-issues: %v", err)
	}
	if files < 3 {
		t.Errorf("top-issues ships %d files, want SKILL.md + 2 references", files)
	}
}
