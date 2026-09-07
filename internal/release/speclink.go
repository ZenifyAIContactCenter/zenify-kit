package release

import (
	"path"
	"regexp"
	"strings"
)

// specSlugRe rút token chủ đề từ tên file spec: <date>-<topic>-design.md → <topic>.
var specSlugRe = regexp.MustCompile(`^(?:\d{4}-\d{2}-\d{2}-)?(.+?)-design$`)

// Tag regex + tagValue MIRROR internal/analyze/analyze.go:69-71 (M6c1) VERBATIM so speclink
// extracts the same tag values that analyze validates. Cannot import (khác package, unexported)
// → duplicate pattern có chủ đích; nếu analyze đổi format, đổi cả đây. `(?m)` để quét nhiều dòng.
var blastRe = regexp.MustCompile("(?m)^\\s*(?:[-*+]\\s+)?[`*]*_Blast-radius:\\s*(.*)$")
var dbRe = regexp.MustCompile("(?m)^\\s*(?:[-*+]\\s+)?[`*]*_DB:\\s*(.*)$")
var rollbackRe = regexp.MustCompile("(?m)^\\s*(?:[-*+]\\s+)?[`*]*_Rollback:\\s*(.*)$")

// tagValue mirror analyze.go: trim trailing backtick/emphasis + space quanh value.
func tagValue(s string) string {
	return strings.TrimSpace(strings.TrimRight(strings.TrimSpace(s), "`*"))
}

func firstGroup(re *regexp.Regexp, s string) string {
	if m := re.FindStringSubmatch(s); m != nil {
		return tagValue(m[1])
	}
	return ""
}

// ParseSpecBrief đọc một file spec đã load → SpecMeta (slug từ tên file + 3 tag Brief).
func ParseSpecBrief(p string, content []byte) SpecMeta {
	base := strings.TrimSuffix(path.Base(p), ".md")
	slug := base
	if m := specSlugRe.FindStringSubmatch(base); m != nil {
		slug = m[1]
	}
	s := string(content)
	return SpecMeta{
		Path:        p,
		Slug:        NormalizeKey(slug),
		BlastRadius: firstGroup(blastRe, s),
		DB:          firstGroup(dbRe, s),
		Rollback:    firstGroup(rollbackRe, s),
	}
}

var trailerRe = regexp.MustCompile(`(?m)^Spec:\s*(\S+)\s*$`)

// specTrailer rút path từ trailer "Spec: <path>" trong body commit ("" nếu không có).
func specTrailer(bodies string) string {
	if m := trailerRe.FindStringSubmatch(bodies); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// noteRisk đọc risk-metadata trực tiếp từ body note-commit (tier-0). Trả (RiskMeta, true)
// nếu có ÍT NHẤT một trong ba tag. SpecPath = trailer "Spec:" nếu có, else sentinel "note"
// (khác rỗng để Build đếm là "có spec"; humanRisk chỉ kiểm rỗng/khác-rỗng, không in path).
func noteRisk(bodies string) (RiskMeta, bool) {
	b := firstGroup(blastRe, bodies)
	d := firstGroup(dbRe, bodies)
	rb := firstGroup(rollbackRe, bodies)
	if b == "" && d == "" && rb == "" {
		return RiskMeta{}, false
	}
	sp := specTrailer(bodies)
	if sp == "" {
		sp = "note"
	}
	return RiskMeta{SpecPath: sp, BlastRadius: b, DB: d, Rollback: rb}, true
}

// LinkSpec: tier-0 note-trailer → tier-1 Spec-trailer → tier-2 slug-match → rỗng.
// Trả RiskMeta (SpecPath="" = unknown).
func LinkSpec(ch Change, specs []SpecMeta) RiskMeta {
	var b strings.Builder
	for _, c := range ch.Commits {
		b.WriteString(c.Body)
		b.WriteString("\n")
	}
	bodies := b.String()
	// (0) tier-0: risk trực tiếp từ note-commit.
	if rm, ok := noteRisk(bodies); ok {
		return rm
	}
	// (1) trailer Spec: trong body.
	if tp := specTrailer(bodies); tp != "" {
		for _, s := range specs {
			if s.Path == tp || strings.HasSuffix(s.Path, tp) || strings.HasSuffix(tp, s.Path) {
				return risk(s)
			}
		}
	}
	// (2) fuzzy slug-match.
	want := NormalizeKey(ch.Slug)
	for _, s := range specs {
		if s.Slug == want {
			return risk(s)
		}
	}
	for _, s := range specs {
		if want != "" && (strings.Contains(s.Slug, want) || strings.Contains(want, s.Slug)) {
			return risk(s)
		}
	}
	return RiskMeta{}
}

func risk(s SpecMeta) RiskMeta {
	return RiskMeta{SpecPath: s.Path, BlastRadius: s.BlastRadius, DB: s.DB, Rollback: s.Rollback}
}
