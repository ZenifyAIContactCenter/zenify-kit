package release

import (
	"path"
	"regexp"
	"strings"
)

// specSlugRe extracts the topic token from a spec file name: <date>-<topic>-design.md → <topic>.
var specSlugRe = regexp.MustCompile(`^(?:\d{4}-\d{2}-\d{2}-)?(.+?)-design$`)

// Tag regex + tagValue MIRROR internal/analyze/analyze.go:69-71 (M6c1) VERBATIM so speclink
// extracts the same tag values that analyze validates. Cannot import (different package,
// unexported) → the duplicated pattern is intentional; if analyze changes format, change this
// too. `(?m)` to scan multiple lines.
var blastRe = regexp.MustCompile("(?m)^\\s*(?:[-*+]\\s+)?[`*]*_Blast-radius:\\s*(.*)$")
var dbRe = regexp.MustCompile("(?m)^\\s*(?:[-*+]\\s+)?[`*]*_DB:\\s*(.*)$")
var rollbackRe = regexp.MustCompile("(?m)^\\s*(?:[-*+]\\s+)?[`*]*_Rollback:\\s*(.*)$")

// Capture group is non-greedy with an optional trailing run of `*_` stripped from the match
// (not just via tagValue, which only trims backtick/asterisk): unlike the other Brief tags,
// _Supersedes: is commonly written wrapped in markdown italic (a closing "_"), and a literal
// greedy (.*)$ would swallow that closing underscore into the captured slug.
var supersedesRe = regexp.MustCompile("(?m)^\\s*(?:[-*+]\\s+)?[`*]*_Supersedes:\\s*(.*?)[`*_]*$")

// noteDescRe captures the _Release-Note trailer (a one-line description written by /ship via
// `release-note --note`). Same shape as the 3 risk tags so it tolerates [-*] bullets / emphasis.
// This is the READ side that was missing from the _Release-Note mechanism (the write-side already
// exists in cli/release_note.go); if absent → Note is empty.
var noteDescRe = regexp.MustCompile("(?m)^\\s*(?:[-*+]\\s+)?[`*]*_Release-Note:\\s*(.*)$")

// tagValue mirrors analyze.go: trim trailing backtick/emphasis + surrounding whitespace.
func tagValue(s string) string {
	return strings.TrimSpace(strings.TrimRight(strings.TrimSpace(s), "`*"))
}

func firstGroup(re *regexp.Regexp, s string) string {
	if m := re.FindStringSubmatch(s); m != nil {
		return tagValue(m[1])
	}
	return ""
}

// ParseSpecBrief reads an already-loaded spec file → SpecMeta (slug from the file name + the 3 Brief tags).
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
		Supersedes:  firstGroup(supersedesRe, s),
	}
}

var trailerRe = regexp.MustCompile(`(?m)^Spec:\s*(\S+)\s*$`)

// specTrailer extracts the path from a "Spec: <path>" trailer in a commit body ("" if none).
func specTrailer(bodies string) string {
	if m := trailerRe.FindStringSubmatch(bodies); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// noteRisk reads risk metadata directly from a note-commit's body (tier-0). Returns (RiskMeta,
// true) if AT LEAST one of the three tags is present. SpecPath = the "Spec:" trailer if present,
// else the sentinel "note" (non-empty so Build counts it as "has a spec"; humanRisk only checks
// empty/non-empty, it never prints the path).
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
	return RiskMeta{SpecPath: sp, BlastRadius: b, DB: d, Rollback: rb, Note: firstGroup(noteDescRe, bodies)}, true
}

// releaseSlugRe extracts the linking slug from a note-commit's "_Release-Slug: <slug>" trailer.
var releaseSlugRe = regexp.MustCompile(`(?m)^_Release-Slug:\s*(\S+)\s*$`)

// noteSubjectRe: only the dedicated note-commit created by release-note carries this subject.
var noteSubjectRe = regexp.MustCompile(`^chore\(release\): note\b`)

// IsReleaseNote: a release-note-commit = the DEDICATED subject chore(release): note + the
// _Release-Slug: trailer. BOTH are required so a feat-commit that accidentally swallowed the
// trailer (e.g. a squash-merge) isn't mistaken for a note and filtered out of the changelog.
func IsReleaseNote(c Commit) bool {
	return noteSubjectRe.MatchString(c.Subject) && releaseSlugRe.MatchString(c.Body)
}

// NoteRiskBySlug scans the note-commits → map[normalizedSlug]RiskMeta. Only added when noteRisk
// is ok (≥1 of the 3 risk tags present). The slug is normalized with NormalizeKey to match Change.Slug.
func NoteRiskBySlug(notes []Commit) map[string]RiskMeta {
	m := map[string]RiskMeta{}
	for _, c := range notes {
		sm := releaseSlugRe.FindStringSubmatch(c.Body)
		if sm == nil {
			continue
		}
		if rm, ok := noteRisk(c.Body); ok {
			m[NormalizeKey(sm[1])] = rm
		}
	}
	return m
}

// LinkSpecTier is LinkSpec plus the tier that matched. Match order MUST stay note → trailer →
// slug-exact → slug-fuzzy (a precedence bug-fixed once in 8c73cf8); the existing LinkSpec tests
// are the guard against reordering.
func LinkSpecTier(ch Change, specs []SpecMeta, notes map[string]RiskMeta) (RiskMeta, LinkTier) {
	// (0) tier-0: risk from a note-commit linked by slug.
	if rm, ok := notes[NormalizeKey(ch.Slug)]; ok {
		return rm, TierNote
	}
	var b strings.Builder
	for _, c := range ch.Commits {
		b.WriteString(c.Body)
		b.WriteString("\n")
	}
	bodies := b.String()
	// (1) the Spec: trailer in the body.
	if tp := specTrailer(bodies); tp != "" {
		for _, s := range specs {
			if s.Path == tp || strings.HasSuffix(s.Path, tp) || strings.HasSuffix(tp, s.Path) {
				return risk(s), TierTrailer
			}
		}
	}
	// (2) fuzzy slug-match.
	want := NormalizeKey(ch.Slug)
	for _, s := range specs {
		if s.Slug == want {
			return risk(s), TierSlugExact
		}
	}
	for _, s := range specs {
		if want != "" && (strings.Contains(s.Slug, want) || strings.Contains(want, s.Slug)) {
			return risk(s), TierSlugFuzzy
		}
	}
	return RiskMeta{}, TierNone
}

// LinkSpec keeps its original signature (11 call sites depend on it) and discards the tier.
func LinkSpec(ch Change, specs []SpecMeta, notes map[string]RiskMeta) RiskMeta {
	rm, _ := LinkSpecTier(ch, specs, notes)
	return rm
}

func risk(s SpecMeta) RiskMeta {
	return RiskMeta{SpecPath: s.Path, BlastRadius: s.BlastRadius, DB: s.DB, Rollback: s.Rollback}
}
