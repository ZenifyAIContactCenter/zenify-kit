package analyze

import (
	"os"
	"strings"
	"testing"
)

// SC-1: FR-2 must not be cited in the plan -> orphan FR (CRITICAL); FR-1 cited -> not orphan.
func TestAnalyze_OrphanFR(t *testing.T) {
	spec := "**FR-1.** do X\n- FR-1.1: detail\n**FR-2.** do Y\n"
	plan := "### Task 1: X\n_Requirements: FR-1_\ncode\n"
	r := Analyze(spec, plan)
	if !hasFinding(r, "orphan-fr", "FR-2", Critical) {
		t.Errorf("missing orphan-fr FR-2 (CRITICAL); findings=%+v", r.Findings)
	}
	if hasFinding(r, "orphan-fr", "FR-1", Critical) {
		t.Errorf("FR-1 must NOT be orphan (already cited)")
	}
}

// SC-2: task without _Requirements: -> orphan-task; ref to FR-9 not in spec -> dangling-ref.
func TestAnalyze_OrphanTaskAndDangling(t *testing.T) {
	spec := "**FR-1.** do X\n"
	plan := "### Task 1: X\n_Requirements: FR-1_\n### Task 2: Y\ndeclares nothing\n### Task 3: Z\n_Requirements: FR-9_\n"
	r := Analyze(spec, plan)
	if !hasFinding(r, "orphan-task", "", High) {
		t.Errorf("missing orphan-task (HIGH); findings=%+v", r.Findings)
	}
	if !hasFinding(r, "dangling-ref", "FR-9", High) {
		t.Errorf("missing dangling-ref FR-9 (HIGH); findings=%+v", r.Findings)
	}
}

// Seam fix: writing-plans authors the tag as a markdown bullet with the tag wrapped in
// backticks (`- ` + "`_Requirements: FR-1_`"), NOT a bare line-start tag. That
// template-compliant form must be detected — otherwise a real SDD plan reports every task
// as orphan. Bare line-start tags (used by the other tests) must keep matching too.
func TestAnalyze_RequirementsTagBulletedBacktick(t *testing.T) {
	spec := "**FR-1.** do X\n"
	// exactly the writing-plans Task Structure format: bullet under **Interfaces:**, backtick-wrapped.
	plan := "### Task 1: X\n**Interfaces:**\n- `_Requirements: FR-1_`\ncode\n"
	r := Analyze(spec, plan)
	if hasFinding(r, "orphan-fr", "FR-1", Critical) {
		t.Errorf("FR-1 cited via bullet+backtick tag — must NOT be orphan; findings=%+v", r.Findings)
	}
	if hasFinding(r, "orphan-task", "", High) {
		t.Errorf("task HAS tag (bullet+backtick) — must NOT be orphan-task; findings=%+v", r.Findings)
	}
}

// SC-3: two [NEEDS CLARIFICATION] markers -> count exactly 2, with line numbers.
func TestAnalyze_MarkerScan(t *testing.T) {
	spec := "line 1\n[NEEDS CLARIFICATION: a]\nline 3\n[NEEDS CLARIFICATION: b]\n"
	r := Analyze(spec, "")
	if len(r.Markers) != 2 {
		t.Fatalf("want 2 markers, got %d: %+v", len(r.Markers), r.Markers)
	}
	if r.Markers[0].Line != 2 || r.Markers[1].Line != 4 {
		t.Errorf("wrong marker line numbers: %+v", r.Markers)
	}
	if n := countKind(r, "marker"); n != 2 {
		t.Errorf("want 2 marker findings, got %d", n)
	}
}

// SC-4: ## Brief with 5 numbered items -> BriefFound=true, BriefFields=5.
func TestAnalyze_StructuralBrief(t *testing.T) {
	spec := "## Brief\n1. a\n2. b\n3. c\n4. d\n5. e\n## Next\n6. not counted\n"
	r := Analyze(spec, "")
	if !r.BriefFound {
		t.Fatal("must find ## Brief")
	}
	if r.BriefFields != 5 {
		t.Errorf("want 5 Brief items, got %d", r.BriefFields)
	}
}

// Coverage at the top level: FR-1.1 in spec, plan cites FR-1 -> FR-1 is NOT orphan.
func TestAnalyze_TopLevelNormalization(t *testing.T) {
	spec := "**FR-1.** X\n- FR-1.1: a\n- FR-1.2: b\n"
	plan := "### Task 1\n_Requirements: FR-1_\n"
	r := Analyze(spec, plan)
	if hasFinding(r, "orphan-fr", "FR-1", Critical) {
		t.Errorf("FR-1 already cited (via FR-1 itself) — must not be orphan")
	}
	for _, f := range r.Findings {
		if f.ID == "FR-1.1" || f.ID == "FR-1.2" {
			t.Errorf("sub-ID must NOT become its own orphan: %+v", f)
		}
	}
}

// SeverityCounts must reflect the findings correctly.
func TestAnalyze_SeverityCounts(t *testing.T) {
	spec := "**FR-1.** X\n**FR-2.** Y\n"
	plan := "### Task 1\n_Requirements: FR-1_\n"
	r := Analyze(spec, plan)
	if r.SeverityCounts["CRITICAL"] < 1 {
		t.Errorf("want >=1 CRITICAL (FR-2 orphan), got %v", r.SeverityCounts)
	}
}

// helpers test
func hasFinding(r Result, kind, id string, sev Severity) bool {
	for _, f := range r.Findings {
		if f.Kind == kind && f.Severity == sev && (id == "" || f.ID == id) {
			return true
		}
	}
	return false
}

func countKind(r Result, kind string) int {
	n := 0
	for _, f := range r.Findings {
		if f.Kind == kind {
			n++
		}
	}
	return n
}

// SC-7: production source (analyze.go) must be project-agnostic — must not
// hardcode project-specific values. (This check only inspects analyze.go
// itself, not this test file.)
func TestAnalyze_ProductionSourceAgnostic(t *testing.T) {
	b, err := os.ReadFile("analyze.go")
	if err != nil {
		t.Fatalf("read analyze.go: %v", err)
	}
	s := string(b)
	for _, forbidden := range []string{"mermaid", "tiếng Việt", "Vietnamese"} { //znf:allow-lang
		if strings.Contains(s, forbidden) {
			t.Errorf("analyze.go must NOT hardcode %q (project-agnostic)", forbidden)
		}
	}
}

// SC-1: Brief with all 3 tags non-empty -> 0 missing-* findings.
func TestAnalyze_RiskMetadataAllPresent(t *testing.T) {
	spec := "## Brief\n_Blast-radius: single-repo — foo\n_DB: N/A\n_Rollback: revert the diff\n## Goals\n"
	r := Analyze(spec, "")
	for _, k := range []string{"missing-blast-radius", "missing-db-guarantee", "missing-rollback"} {
		if countKind(r, k) != 0 {
			t.Errorf("tags complete but still got %s: %+v", k, r.Findings)
		}
	}
}

// SC-2: missing _Rollback: -> exactly 1 missing-rollback (HIGH); blast/DB tagged -> not flagged.
func TestAnalyze_MissingRollback(t *testing.T) {
	spec := "## Brief\n_Blast-radius: x\n_DB: N/A\n## Goals\n"
	r := Analyze(spec, "")
	if !hasFinding(r, "missing-rollback", "", High) {
		t.Errorf("missing missing-rollback (HIGH); findings=%+v", r.Findings)
	}
	if countKind(r, "missing-rollback") != 1 {
		t.Errorf("want exactly 1 missing-rollback, got %d", countKind(r, "missing-rollback"))
	}
	if hasFinding(r, "missing-blast-radius", "", High) || hasFinding(r, "missing-db-guarantee", "", High) {
		t.Errorf("blast/DB tagged — must not be flagged: %+v", r.Findings)
	}
}

// SC-3: _Blast-radius: empty content -> missing-blast-radius.
func TestAnalyze_EmptyBlastRadius(t *testing.T) {
	spec := "## Brief\n_Blast-radius:   \n_DB: N/A\n_Rollback: revert\n## Goals\n"
	r := Analyze(spec, "")
	if !hasFinding(r, "missing-blast-radius", "", High) {
		t.Errorf("_Blast-radius empty must be flagged; findings=%+v", r.Findings)
	}
}

// SC-4: _DB: N/A not flagged; _DB non-empty even without the keyword -> still NOT flagged mechanically.
func TestAnalyze_DBPresenceOnly(t *testing.T) {
	na := Analyze("## Brief\n_Blast-radius: x\n_DB: N/A\n_Rollback: y\n## Goals\n", "")
	if countKind(na, "missing-db-guarantee") != 0 {
		t.Errorf("_DB: N/A must not be flagged: %+v", na.Findings)
	}
	partial := Analyze("## Brief\n_Blast-radius: x\n_DB: query-plan only\n_Rollback: y\n## Goals\n", "")
	if countKind(partial, "missing-db-guarantee") != 0 {
		t.Errorf("_DB non-empty (even without the keyword) must NOT be flagged mechanically: %+v", partial.Findings)
	}
}

// SC-5: no ## Brief -> 0 risk-metadata findings + FR->task coverage does not regress.
func TestAnalyze_NoBriefNoRiskFindings(t *testing.T) {
	spec := "**FR-1.** X\n**FR-2.** Y\n"
	plan := "### Task 1\n_Requirements: FR-1_\n"
	r := Analyze(spec, plan)
	for _, k := range []string{"missing-blast-radius", "missing-db-guarantee", "missing-rollback"} {
		if countKind(r, k) != 0 {
			t.Errorf("no ## Brief but still got %s: %+v", k, r.Findings)
		}
	}
	if !hasFinding(r, "orphan-fr", "FR-2", Critical) {
		t.Errorf("regression: lost orphan-fr FR-2; findings=%+v", r.Findings)
	}
}

// Tag detected via backtick-wrapping (a bullet `- ` around `_DB: N/A`) — mirrors the _Requirements: seam.
func TestAnalyze_RiskTagBacktickWrapped(t *testing.T) {
	spec := "## Brief\n- `_Blast-radius: x`\n- `_DB: N/A`\n- `_Rollback: revert`\n## Goals\n"
	r := Analyze(spec, "")
	for _, k := range []string{"missing-blast-radius", "missing-db-guarantee", "missing-rollback"} {
		if countKind(r, k) != 0 {
			t.Errorf("backtick-wrapped tag must be recognized, %s wrongly flagged: %+v", k, r.Findings)
		}
	}
}
