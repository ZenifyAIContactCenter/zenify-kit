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
					Authors: []string{"namph", "hungnk"}, Desc: "add linked field type",
					Risk: RiskMeta{SpecPath: "specs/be/x-design.md", BlastRadius: "be+web", DB: "N/A", Rollback: "revert",
						Note: "thêm loại trường liên kết cho form ticket"}}, //znf:allow-lang
				{Title: "Report tz", Slug: "report-tz", Type: "fix", Authors: []string{"namph"},
					Commits: []Commit{{Subject: "fix(report): tz offset"}, {Subject: "test: tz case"}}},
				{Title: "Urgent", Slug: "urgent", Type: "hotfix", Commits: make([]Commit, 1), IsHotfix: true, NotOnStaging: true},
				{Title: "Khác (chore)", Slug: "misc:chore", Type: "chore", Commits: make([]Commit, 3)}, //znf:allow-lang
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
		"# Release 84", "Quyết định nhanh", "notification", // not shipped //znf:allow-lang
		"migration → BE", "**/chat_*", // shared + deploy order
		"Shared-collection: **/chat_*",                    // FR-3.4 per-repo risk-proxy
		"| Thay đổi | Mô tả | # | Dev | Spec | Staging |", // table header (Mô tả column from _Release-Note) //znf:allow-lang
		"### Features", "Linked fields", // feature title in the Thay đổi cell //znf:allow-lang
		"thêm loại trường liên kết cho form ticket", // Mô tả column from Risk.Note //znf:allow-lang
		"namph, hungnk", // Dev column
		"### Fixes", "Report tz",
		"### Hotfixes", "⚠ chưa sync", // hotfix staging flag in the table cell //znf:allow-lang
		"1/3", // spec coverage
		// Risk block below the table, only for changes WITH a spec — each tag one **Label:** line.
		"#### Rủi ro (thay đổi có spec)", //znf:allow-lang
		"**#12 — Linked fields**",
		"- **Blast-radius:** be+web",
		"- **DB:** N/A",
		"- **Rollback:** revert",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q\n---\n%s", want, out)
		}
	}
	if strings.Contains(out, "### Chores") {
		t.Errorf("non-verbose must NOT print the Chores section (noise for go/no-go)")
	}
	if strings.Contains(out, "— go/no-go") {
		t.Errorf("H1 must not carry the '— go/no-go' gloss")
	}
	if strings.Contains(out, "fix(report): tz offset") {
		t.Errorf("non-verbose must NOT print each change's commit list")
	}
	// Desc (the commit subject) is no longer pasted into the table cell — only with --verbose.
	if strings.Contains(out, "add linked field type") {
		t.Errorf("non-verbose must NOT print Desc in the Thay đổi cell") //znf:allow-lang
	}
	// The Spec column is a flag, no longer stuffed with the old "unknown — no spec" prose.
	if strings.Contains(out, "unknown — no spec") {
		t.Errorf("the table must NOT contain the old risk prose 'unknown — no spec'")
	}
}

func TestRenderVerboseExpandsChore(t *testing.T) {
	out := Render(sampleReport(), true)
	if !strings.Contains(out, "Khác (chore)") { //znf:allow-lang
		t.Errorf("verbose must list chores")
	}
	if !strings.Contains(out, "fix(report): tz offset") {
		t.Errorf("verbose must print each change's commit list (FR-5.2)")
	}
}

// LOW-1 fix: the unreleased view does NOT print "Sinh <time.Now>" (avoids a no-op churn commit on
// every ship, keeping it deterministic against git-state, SC-6); a cut R<N>.md STILL keeps "Sinh".
func TestRenderUnreleasedOmitsTimestamp(t *testing.T) {
	un := Render(Report{N: 84, Unreleased: true, GeneratedAt: "2026-09-08 10:00", SharedCrossRepo: map[string][]string{}}, false)
	if strings.Contains(un, "Sinh ") {
		t.Errorf("the unreleased view must NOT print a timestamp (churn): %s", un)
	}
	if !strings.Contains(un, "# Release đang hình thành: R84 (chưa deploy)") { //znf:allow-lang
		t.Errorf("unreleased header missing: %s", un)
	}
	cut := Render(Report{N: 84, GeneratedAt: "2026-09-08 10:00", SharedCrossRepo: map[string][]string{}}, false)
	if !strings.Contains(cut, "Sinh 2026-09-08 10:00") {
		t.Errorf("a cut R<N>.md view must keep 'Sinh': %s", cut)
	}
}

// The risk block must exclude chore/other (their table row is verbose-gated) — otherwise
// non-verbose would have a risk block pointing to a change that appears in no table. Mirror release.go:175.
func TestRenderRiskDetailExcludesChore(t *testing.T) {
	rep := Report{N: 84, SharedCrossRepo: map[string][]string{}, Repos: []RepoReport{{
		Name: "be", Changes: []Change{
			{Title: "Choreish", Slug: "cho", Type: "chore", PRNum: "9", Commits: make([]Commit, 1),
				Risk: RiskMeta{SpecPath: "note", BlastRadius: "x"}},
		},
	}}}
	out := Render(rep, false)
	if strings.Contains(out, "#### Rủi ro") { //znf:allow-lang
		t.Errorf("a chore with a spec must NOT create a risk block: %s", out)
	}
}

// Risk block: a Title-only header when there's no PR, and oneLine trims trailing emphasis (** leaking
// from the "**_Label:**" tag). These two branches had no test before.
func TestRenderRiskDetailTrimAndHeaderNoPR(t *testing.T) {
	rep := Report{N: 84, SharedCrossRepo: map[string][]string{}, Repos: []RepoReport{{
		Name: "be", Changes: []Change{
			{Title: "No PR change", Slug: "npr", Type: "feat", Commits: make([]Commit, 1),
				Risk: RiskMeta{SpecPath: "specs/x.md", BlastRadius: "** be leaked *", DB: "", Rollback: "revert"}},
		},
	}}}
	out := Render(rep, false)
	if !strings.Contains(out, "**No PR change**") || strings.Contains(out, "#—") || strings.Contains(out, "# — No PR change") {
		t.Errorf("the no-PR header must be a bare **Title**: %s", out)
	}
	if !strings.Contains(out, "- **Blast-radius:** be leaked") {
		t.Errorf("oneLine must trim trailing '**': %s", out)
	}
	// The raw form "** be leaked *" (leading/trailing emphasis) must be gone after trimming.
	if strings.Contains(out, "** be leaked *") || strings.Contains(out, "be leaked *") {
		t.Errorf("value still has untrimmed trailing emphasis: %s", out)
	}
	if !strings.Contains(out, "- **DB:** —") {
		t.Errorf("an empty value must become '—': %s", out)
	}
}

func TestRenderEmptyReportNoPanic(t *testing.T) {
	out := Render(Report{N: 84, GeneratedAt: "x", SharedCrossRepo: map[string][]string{}}, false)
	if !strings.Contains(out, "# Release 84") {
		t.Errorf("empty render missing header: %s", out)
	}
}
