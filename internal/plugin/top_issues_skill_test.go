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
		// F1: trace logs read as error-level messages only; raw lines carry unmasked PII.
		"Trace error messages",
		"Never read `.logs.lines` whole",
		"Edit or create a file",
		"`<REDACTED>`",
		"with the service, route, time window, trace id",
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
		"| 4 | P1 | P1 | P2 |", "| 3 | P1 | P2 | P3 |", "| 2 | P2 | P3 | P3 |",
		"≥ 20%", "≥ 5%", "≥ 1000 errors", "≥ 200 errors", "Widespread = ≥ 10 tenants", "**2h**",
		"1. Higher Impact first.",
		"2. A money-path issue before",
		"3. Audience: External > Tenant users > Internal.",
		"4. More tenants affected first",
		"5. Trend: New > Rising > Stable > Falling.",
		"6. Larger absolute error count",
		"7. `service + route` in alphabetical order.",
		"Sort all issues by Severity, then the tie-breaks; keep the first 5",
		"Take the first that applies, in this order",
		// F2: a level-parsed prev of 0 may be a log-format change, not a new issue.
		"may be a log-format change",
		`|= "<message prefix>"`,
	})

	q := syncedAsset(t, "skills/top-issues/references/queries.md")
	assertContainsAll(t, "top-issues/references/queries.md", q, []string{
		"offset", "count_over_time() by (span.<tenant_attr>)", "query_loki_patterns", "1h",
		// F1
		"## Trace error messages",
		`select(.level=="error" or .level=="fatal") | .msg | split("\n")[0][0:120]`,
		// F2
		`|= "<message prefix>" [<W>] offset <W>`,
		// F3: group on a new label, never on the parsed msg field.
		"topk(5, sum by (m) (count_over_time(",
		"never on the parsed `msg` field itself",
		// M3
		"drill fallback slot",
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
