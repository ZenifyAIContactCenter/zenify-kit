package analyze

import (
	"os"
	"strings"
	"testing"
)

// SC-1: FR-2 không được cite trong plan → orphan FR (CRITICAL); FR-1 có cite → không orphan.
func TestAnalyze_OrphanFR(t *testing.T) {
	spec := "**FR-1.** làm X\n- FR-1.1: chi tiết\n**FR-2.** làm Y\n"
	plan := "### Task 1: X\n_Requirements: FR-1\ncode\n"
	r := Analyze(spec, plan)
	if !hasFinding(r, "orphan-fr", "FR-2", Critical) {
		t.Errorf("thiếu orphan-fr FR-2 (CRITICAL); findings=%+v", r.Findings)
	}
	if hasFinding(r, "orphan-fr", "FR-1", Critical) {
		t.Errorf("FR-1 KHÔNG được là orphan (đã cite)")
	}
}

// SC-2: task không có _Requirements: → orphan-task; ref tới FR-9 không có trong spec → dangling-ref.
func TestAnalyze_OrphanTaskAndDangling(t *testing.T) {
	spec := "**FR-1.** làm X\n"
	plan := "### Task 1: X\n_Requirements: FR-1\n### Task 2: Y\nkhông khai gì\n### Task 3: Z\n_Requirements: FR-9\n"
	r := Analyze(spec, plan)
	if !hasFinding(r, "orphan-task", "", High) {
		t.Errorf("thiếu orphan-task (HIGH); findings=%+v", r.Findings)
	}
	if !hasFinding(r, "dangling-ref", "FR-9", High) {
		t.Errorf("thiếu dangling-ref FR-9 (HIGH); findings=%+v", r.Findings)
	}
}

// SC-3: hai marker [NEEDS CLARIFICATION] → đếm đúng 2, kèm số dòng.
func TestAnalyze_MarkerScan(t *testing.T) {
	spec := "dòng 1\n[NEEDS CLARIFICATION: a]\ndòng 3\n[NEEDS CLARIFICATION: b]\n"
	r := Analyze(spec, "")
	if len(r.Markers) != 2 {
		t.Fatalf("muốn 2 marker, có %d: %+v", len(r.Markers), r.Markers)
	}
	if r.Markers[0].Line != 2 || r.Markers[1].Line != 4 {
		t.Errorf("số dòng marker sai: %+v", r.Markers)
	}
	if n := countKind(r, "marker"); n != 2 {
		t.Errorf("muốn 2 finding marker, có %d", n)
	}
}

// SC-4: ## Brief có 5 mục đánh số → BriefFound=true, BriefFields=5.
func TestAnalyze_StructuralBrief(t *testing.T) {
	spec := "## Brief\n1. a\n2. b\n3. c\n4. d\n5. e\n## Tiếp theo\n6. không tính\n"
	r := Analyze(spec, "")
	if !r.BriefFound {
		t.Fatal("phải tìm thấy ## Brief")
	}
	if r.BriefFields != 5 {
		t.Errorf("muốn 5 mục Brief, có %d", r.BriefFields)
	}
}

// Coverage ở mức top-level: FR-1.1 trong spec, plan cite FR-1 → FR-1 KHÔNG orphan.
func TestAnalyze_TopLevelNormalization(t *testing.T) {
	spec := "**FR-1.** X\n- FR-1.1: a\n- FR-1.2: b\n"
	plan := "### Task 1\n_Requirements: FR-1\n"
	r := Analyze(spec, plan)
	if hasFinding(r, "orphan-fr", "FR-1", Critical) {
		t.Errorf("FR-1 đã cite (qua chính FR-1) — không được orphan")
	}
	for _, f := range r.Findings {
		if f.ID == "FR-1.1" || f.ID == "FR-1.2" {
			t.Errorf("sub-ID KHÔNG được thành orphan riêng: %+v", f)
		}
	}
}

// SeverityCounts phản ánh đúng findings.
func TestAnalyze_SeverityCounts(t *testing.T) {
	spec := "**FR-1.** X\n**FR-2.** Y\n"
	plan := "### Task 1\n_Requirements: FR-1\n"
	r := Analyze(spec, plan)
	if r.SeverityCounts["CRITICAL"] < 1 {
		t.Errorf("muốn ≥1 CRITICAL (FR-2 orphan), có %v", r.SeverityCounts)
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

// SC-7: production source (analyze.go) phải project-agnostic — không hardcode
// giá trị project. (File test này CÓ comment tiếng Việt theo house convention,
// nên chỉ soi analyze.go, không soi chính nó.)
func TestAnalyze_ProductionSourceAgnostic(t *testing.T) {
	b, err := os.ReadFile("analyze.go")
	if err != nil {
		t.Fatalf("đọc analyze.go: %v", err)
	}
	s := string(b)
	for _, forbidden := range []string{"mermaid", "tiếng Việt", "Vietnamese"} {
		if strings.Contains(s, forbidden) {
			t.Errorf("analyze.go KHÔNG được hardcode %q (project-agnostic)", forbidden)
		}
	}
}
