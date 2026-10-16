package plugin

import (
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
