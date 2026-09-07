package release

import (
	"strings"
	"testing"
)

func sampleReport() Report {
	return Report{
		N: 84, GeneratedAt: "2026-09-05 16:10",
		ShippingRepos: []string{"be"},
		TotalFeat:     1, TotalFix: 1, TotalHotfix: 1, HotfixesNotSynced: 1,
		Migrations: []string{"be"}, SpecLinked: 1, SpecTotal: 3,
		Repos: []RepoReport{{
			Name: "be", PrevRelease: 83, CutDate: "2026-08-26",
			HasMigration: true, HasTestTouch: true, SharedHits: []string{"**/chat_*"},
			Changes: []Change{
				{Title: "Linked fields", Slug: "linked-fields", Type: "feat", PRNum: "12", Commits: make([]Commit, 20),
					Risk: RiskMeta{SpecPath: "specs/be/x-design.md", BlastRadius: "be+web", DB: "N/A", Rollback: "revert"}},
				{Title: "Report tz", Slug: "report-tz", Type: "fix", Commits: make([]Commit, 2)},
				{Title: "Urgent", Slug: "urgent", Type: "hotfix", Commits: make([]Commit, 1), IsHotfix: true, NotOnStaging: true},
				{Title: "Khác (chore)", Slug: "misc:chore", Type: "chore", Commits: make([]Commit, 3)},
			},
		}},
		NotShipped:      []string{"notification"},
		SharedCrossRepo: map[string][]string{"**/chat_*": {"be", "chatting"}},
		DeployOrderNote: true,
	}
}

func TestRenderHeadlineAndSections(t *testing.T) {
	out := Render(sampleReport(), false)
	for _, want := range []string{
		"# Release 84", "Quyết định nhanh", "notification", // không ship
		"migration → BE", "**/chat_*", // shared + deploy order
		"Shared-collection: **/chat_*",            // FR-3.4 per-repo risk-proxy
		"### Features", "Linked fields", "be+web", // feature + risk từ spec
		"### Fixes", "Report tz",
		"### Hotfixes", "CHƯA trên staging", // hotfix cờ
		"unknown — no spec", // fix không spec
		"1/3",               // spec coverage
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render thiếu %q\n---\n%s", want, out)
		}
	}
	if strings.Contains(out, "### Chores\n") && !strings.Contains(out, "ẩn 1") {
		t.Errorf("chore phải gập với count khi !verbose")
	}
}

func TestRenderVerboseExpandsChore(t *testing.T) {
	out := Render(sampleReport(), true)
	if !strings.Contains(out, "Khác (chore)") {
		t.Errorf("verbose phải liệt kê chore")
	}
}

func TestRenderEmptyReportNoPanic(t *testing.T) {
	out := Render(Report{N: 84, GeneratedAt: "x", SharedCrossRepo: map[string][]string{}}, false)
	if !strings.Contains(out, "# Release 84") {
		t.Errorf("empty render missing header: %s", out)
	}
}
