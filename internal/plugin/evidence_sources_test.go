package plugin

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
)

// FR-1, FR-3.1, FR-10.3, SC-2: one project-neutral reference teaches the evidence tools.
func TestEvidenceSources(t *testing.T) {
	s := syncedAsset(t, "skills/_shared/evidence-sources.md")

	assertContainsAll(t, "evidence-sources.md", s, []string{
		"## Before the first query", "## Order by symptom", "## Pivots between sources",
		"## Measured dead ends", "## PostHog exec", "## Data handling", "## Output size",
		"## When a source is missing", "## Role dev on production",
		"list_datasources", "list_loki_label_values", "list_tempo_attribute_names",
		"list_prometheus_metric_names", "trace_id", "endTime", "seconds", "count_over_time",
		"span.http.status_code", "search", "info", "call", "jq",
		api.DevSQLRule,
	})

	mongo, redis := api.DevPrdOps()
	if len(mongo) == 0 || len(redis) == 0 {
		t.Fatal("api.DevPrdOps returned an empty list")
	}
	i := strings.Index(s, "## Role dev on production")
	if i < 0 {
		t.Fatal("evidence-sources.md has no ## Role dev on production section")
	}
	role := s[i:]
	if !strings.Contains(role, api.DevPrdSummary()) {
		t.Errorf("Role dev on production section does not carry api.DevPrdSummary() verbatim:\n%s", api.DevPrdSummary())
	}
	for _, op := range append(mongo, redis...) {
		if !strings.Contains(role, op) {
			t.Errorf("Role dev on production section missing op %q", op)
		}
	}

	sc2 := regexp.MustCompile(`ady8|dfh52|ffk50|ccbe|cch-hub|link_trace_id|messaging\.producer|bullmq:|192\.168`)
	if m := sc2.FindString(s); m != "" {
		t.Errorf("evidence-sources.md carries project-specific string %q (SC-2)", m)
	}
	if n := strings.Count(s, "\n"); n > 200 {
		t.Errorf("evidence-sources.md has %d lines, want <= 200", n)
	}
	if h := syncedAsset(t, "skills/_shared/harness-tools.md"); !strings.Contains(h, "evidence-sources") {
		t.Error("harness-tools.md has no row pointing at evidence-sources (FR-1.8)")
	}

	fl := syncedAsset(t, "skills/fix/references/feedback-loop.md")
	if !strings.Contains(fl, "evidence-sources") {
		t.Error("feedback-loop.md does not point at evidence-sources (FR-3.1)")
	}
	if strings.Contains(fl, "## Evidence sources via the broker") {
		t.Error("feedback-loop.md still carries the old broker section (FR-3.1)")
	}
	if strings.Contains(fl, "role=dev") {
		t.Error("feedback-loop.md still carries the role=dev sentence; it moved to evidence-sources (FR-10.3)")
	}
}

// FR-2, FR-4, FR-5, FR-6.1, FR-7.1, SC-1: skills point at the evidence reference instead of raw tools.
func TestSkillsUseEvidenceSources(t *testing.T) {
	dest := t.TempDir()
	if _, err := Sync(dest, filepath.Join(dest, ".manifest.json")); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	err := filepath.WalkDir(filepath.Join(dest, "skills"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), "docker logs") {
			t.Errorf("%s still says docker logs (SC-1)", p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk skills: %v", err)
	}
	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join(dest, rel))
		if err != nil {
			t.Fatalf("%s not materialized: %v", rel, err)
		}
		return string(b)
	}

	if m := regexp.MustCompile(`mongosh|mysql|psql`).FindString(read("skills/ground/SKILL.md")); m != "" {
		t.Errorf("ground/SKILL.md still names raw DB client %q (FR-5.1, SC-1)", m)
	}
	assertContainsAll(t, "ground/SKILL.md", read("skills/ground/SKILL.md"), []string{"never read .env for credentials"})

	h := read("skills/hotfix/SKILL.md")
	assertContainsAll(t, "hotfix/SKILL.md", h, []string{"evidence-sources", "references/post-deploy-check.md", "znf:ui-verifier"})
	if strings.Contains(h, "browser yourself now") {
		t.Error("hotfix/SKILL.md still tells the agent to drive the browser itself (FR-2.3)")
	}
	assertContainsAll(t, "explain-plan/SKILL.md", read("skills/explain-plan/SKILL.md"), []string{"type=ALL", "--prd", "queryPlanner", "Postgres"})
	for _, s := range []string{"ship", "cook", "e2e"} {
		assertContainsAll(t, s+"/SKILL.md", read("skills/"+s+"/SKILL.md"), []string{"--role"})
	}
	for _, s := range []string{"service-integration", "scout", "gate"} {
		assertContainsAll(t, s+"/SKILL.md", read("skills/"+s+"/SKILL.md"), []string{"redis-read"})
	}

	pd := read("skills/hotfix/references/post-deploy-check.md")
	assertContainsAll(t, "post-deploy-check.md", pd, []string{"before", "after", "Backend", "Web", "Queue", "p95", "PostHog", "Loki"})
	sc2 := regexp.MustCompile(`ady8|dfh52|ffk50|ccbe|cch-hub|link_trace_id|messaging\.producer|bullmq:|192\.168`)
	if m := sc2.FindString(pd); m != "" {
		t.Errorf("post-deploy-check.md carries project-specific string %q (SC-2)", m)
	}
}
