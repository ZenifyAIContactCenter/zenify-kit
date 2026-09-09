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
					Risk: RiskMeta{SpecPath: "specs/be/x-design.md", BlastRadius: "be+web", DB: "N/A", Rollback: "revert"}},
				{Title: "Report tz", Slug: "report-tz", Type: "fix", Authors: []string{"namph"},
					Commits: []Commit{{Subject: "fix(report): tz offset"}, {Subject: "test: tz case"}}},
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
		"Shared-collection: **/chat_*",             // FR-3.4 per-repo risk-proxy
		"| Thay đổi | # | Dev | Spec | Staging |", // bảng header (cột Spec cờ gọn)
		"### Features", "Linked fields",           // feature title trong ô Thay đổi
		"namph, hungnk",                           // Dev column
		"### Fixes", "Report tz",
		"### Hotfixes", "⚠ chưa sync", // hotfix cờ staging trong ô bảng
		"1/3",                          // spec coverage
		// Khối rủi ro dưới bảng, chỉ cho thay đổi CÓ spec — mỗi tag một dòng **Label:**.
		"#### Rủi ro (thay đổi có spec)",
		"**#12 — Linked fields**",
		"- **Blast-radius:** be+web",
		"- **DB:** N/A",
		"- **Rollback:** revert",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render thiếu %q\n---\n%s", want, out)
		}
	}
	if strings.Contains(out, "### Chores") {
		t.Errorf("non-verbose KHÔNG được in section Chores (nhiễu cho go/no-go)")
	}
	if strings.Contains(out, "— go/no-go") {
		t.Errorf("H1 không được gắn gloss '— go/no-go'")
	}
	if strings.Contains(out, "fix(report): tz offset") {
		t.Errorf("non-verbose KHÔNG được in commit list mỗi thay đổi")
	}
	// Desc (subject commit) KHÔNG còn dán vào ô bảng — chỉ ở --verbose.
	if strings.Contains(out, "add linked field type") {
		t.Errorf("non-verbose KHÔNG được in Desc trong ô Thay đổi")
	}
	// Cột Spec là cờ, KHÔNG còn nhồi prose "unknown — no spec" vào ô bảng.
	if strings.Contains(out, "unknown — no spec") {
		t.Errorf("bảng KHÔNG được chứa prose risk cũ 'unknown — no spec'")
	}
}

func TestRenderVerboseExpandsChore(t *testing.T) {
	out := Render(sampleReport(), true)
	if !strings.Contains(out, "Khác (chore)") {
		t.Errorf("verbose phải liệt kê chore")
	}
	if !strings.Contains(out, "fix(report): tz offset") {
		t.Errorf("verbose phải in commit list mỗi thay đổi (FR-5.2)")
	}
}

// LOW-1 fix: view unreleased KHÔNG in "Sinh <time.Now>" (tránh churn commit no-op mỗi ship,
// giữ deterministic theo git-state, SC-6); view cắt R<N>.md VẪN giữ "Sinh".
func TestRenderUnreleasedOmitsTimestamp(t *testing.T) {
	un := Render(Report{N: 84, Unreleased: true, GeneratedAt: "2026-09-08 10:00", SharedCrossRepo: map[string][]string{}}, false)
	if strings.Contains(un, "Sinh ") {
		t.Errorf("view unreleased KHÔNG được in timestamp (churn): %s", un)
	}
	if !strings.Contains(un, "# Release đang hình thành (sau R84)") {
		t.Errorf("unreleased thiếu header: %s", un)
	}
	cut := Render(Report{N: 84, GeneratedAt: "2026-09-08 10:00", SharedCrossRepo: map[string][]string{}}, false)
	if !strings.Contains(cut, "Sinh 2026-09-08 10:00") {
		t.Errorf("view cắt R<N>.md phải giữ 'Sinh': %s", cut)
	}
}

// Khối rủi ro phải loại chore/other (dòng bảng của chúng bị verbose-gate) — nếu không
// non-verbose sẽ có khối rủi ro trỏ tới thay đổi không hiện ở bảng nào. Mirror release.go:175.
func TestRenderRiskDetailExcludesChore(t *testing.T) {
	rep := Report{N: 84, SharedCrossRepo: map[string][]string{}, Repos: []RepoReport{{
		Name: "be", Changes: []Change{
			{Title: "Choreish", Slug: "cho", Type: "chore", PRNum: "9", Commits: make([]Commit, 1),
				Risk: RiskMeta{SpecPath: "note", BlastRadius: "x"}},
		},
	}}}
	out := Render(rep, false)
	if strings.Contains(out, "#### Rủi ro") {
		t.Errorf("chore có spec KHÔNG được tạo khối rủi ro: %s", out)
	}
}

// Khối rủi ro: header Title-only khi không có PR, và oneLine trim emphasis rìa (** leak
// từ tag "**_Label:**"). Hai nhánh này trước đó không có test.
func TestRenderRiskDetailTrimAndHeaderNoPR(t *testing.T) {
	rep := Report{N: 84, SharedCrossRepo: map[string][]string{}, Repos: []RepoReport{{
		Name: "be", Changes: []Change{
			{Title: "No PR change", Slug: "npr", Type: "feat", Commits: make([]Commit, 1),
				Risk: RiskMeta{SpecPath: "specs/x.md", BlastRadius: "** be leaked *", DB: "", Rollback: "revert"}},
		},
	}}}
	out := Render(rep, false)
	if !strings.Contains(out, "**No PR change**") || strings.Contains(out, "#—") || strings.Contains(out, "# — No PR change") {
		t.Errorf("header không-PR phải là **Title** trần: %s", out)
	}
	if !strings.Contains(out, "- **Blast-radius:** be leaked") {
		t.Errorf("oneLine phải trim '**' rìa: %s", out)
	}
	// Dạng raw "** be leaked *" (leading/trailing emphasis) phải biến mất sau trim.
	if strings.Contains(out, "** be leaked *") || strings.Contains(out, "be leaked *") {
		t.Errorf("value vẫn còn emphasis rìa chưa trim: %s", out)
	}
	if !strings.Contains(out, "- **DB:** —") {
		t.Errorf("value rỗng phải thành '—': %s", out)
	}
}

func TestRenderEmptyReportNoPanic(t *testing.T) {
	out := Render(Report{N: 84, GeneratedAt: "x", SharedCrossRepo: map[string][]string{}}, false)
	if !strings.Contains(out, "# Release 84") {
		t.Errorf("empty render missing header: %s", out)
	}
}
