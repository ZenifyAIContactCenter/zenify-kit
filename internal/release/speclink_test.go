package release

import "testing"

// Tag on its own line, wrapped in backticks — matches the real spec convention (numbered field above).
// A raw string can't hold a backtick, so we use an interpreted string with \n.
const briefSpec = "# X design\n## Brief\n1. Problem. p\n5. Service.\n   `_Blast-radius: be + web break`\n6. DB.\n   `_DB: N/A`\n8. Rollback.\n   `_Rollback: revert + watch errors`\n"

func TestParseSpecBriefTags(t *testing.T) {
	m := ParseSpecBrief("specs/be/2026-08-01-linked-fields-design.md", []byte(briefSpec))
	if m.BlastRadius != "be + web break" || m.DB != "N/A" || m.Rollback != "revert + watch errors" {
		t.Fatalf("tag parse: %+v", m)
	}
	if m.Slug != "linked-fields" && m.Slug != "linked fields" {
		// slug is the token between the date and -design
		t.Fatalf("slug: %q", m.Slug)
	}
}

func TestLinkSpecTrailerWins(t *testing.T) {
	specs := []SpecMeta{{Path: "specs/be/2026-08-01-linked-fields-design.md", Slug: "linked-fields", BlastRadius: "X"}}
	ch := Change{Slug: "other", Commits: []Commit{{Body: "some\nSpec: specs/be/2026-08-01-linked-fields-design.md\n"}}}
	r := LinkSpec(ch, specs, nil)
	if r.SpecPath == "" || r.BlastRadius != "X" {
		t.Fatalf("trailer link: %+v", r)
	}
}

func TestLinkSpecSlugMatch(t *testing.T) {
	specs := []SpecMeta{{Path: "specs/be/2026-08-01-linked-fields-design.md", Slug: "linked-fields", BlastRadius: "Y"}}
	ch := Change{Slug: "linked-fields"}
	r := LinkSpec(ch, specs, nil)
	if r.SpecPath == "" || r.BlastRadius != "Y" {
		t.Fatalf("slug match: %+v", r)
	}
}

func TestLinkSpecNoMatch(t *testing.T) {
	specs := []SpecMeta{{Path: "specs/be/foo-design.md", Slug: "foo"}}
	ch := Change{Slug: "bar"}
	r := LinkSpec(ch, specs, nil)
	if r.SpecPath != "" {
		t.Fatalf("no-match must be empty: %+v", r)
	}
}

func TestNoteRiskBySlugAndLinkSpecTier0(t *testing.T) {
	// A note-commit carrying _Release-Slug: + a risk tag → NoteRiskBySlug gathers it into the map
	// keyed by normalized slug; LinkSpec looks up the map by ch.Slug (it no longer scans ch.Commits).
	notes := []Commit{
		{Body: "_Release-Slug: whatever\n_Release-Note: add X\n_Blast-radius: be+web\n_DB: N/A\n_Rollback: revert PR\n"},
	}
	m := NoteRiskBySlug(notes)
	ch := Change{Slug: "whatever"}
	rm := LinkSpec(ch, nil, m)
	if rm.BlastRadius != "be+web" || rm.DB != "N/A" || rm.Rollback != "revert PR" {
		t.Fatalf("tier-0 must take its risk from the note map: %+v", rm)
	}
	if rm.SpecPath == "" {
		t.Errorf("tier-0 must set a non-empty SpecPath (to count toward coverage): %+v", rm)
	}
}

func TestNoteRiskBySlugSpecPath(t *testing.T) {
	notes := []Commit{
		{Body: "_Release-Slug: x\n_Blast-radius: hub\n_DB: N/A\n_Rollback: revert\nSpec: specs/zenify-kit/2026-09-07-x-design.md\n"},
	}
	m := NoteRiskBySlug(notes)
	rm := LinkSpec(Change{Slug: "x"}, nil, m)
	if rm.SpecPath != "specs/zenify-kit/2026-09-07-x-design.md" {
		t.Errorf("tier-0 must take SpecPath from the Spec: trailer when present: %+v", rm)
	}
}

func TestNoteRiskBySlugNormalizesKey(t *testing.T) {
	// A slug in the trailer that isn't normalized yet (e.g. a raw branch "Feat/Foo_Bar") still
	// matches a Change.Slug that has already gone through NormalizeKey via Aggregate.
	notes := []Commit{
		{Body: "_Release-Slug: Feat/Foo_Bar\n_Blast-radius: be\n"},
	}
	m := NoteRiskBySlug(notes)
	rm := LinkSpec(Change{Slug: "foo-bar"}, nil, m)
	if rm.BlastRadius != "be" {
		t.Fatalf("slug must be normalized to match: %+v (map=%+v)", rm, m)
	}
}

func TestNoteRiskBySlugSkipsNoRiskTag(t *testing.T) {
	notes := []Commit{{Body: "_Release-Slug: foo\n_Release-Note: nothing risky\n"}}
	m := NoteRiskBySlug(notes)
	if len(m) != 0 {
		t.Errorf("a note with no risk tag at all must not enter the map: %+v", m)
	}
}

// The read side that was missing from the _Release-Note mechanism: the one-line description
// (written by /ship via --note) must be pulled into RiskMeta.Note to render the "Mô tả" column. //znf:allow-lang
// buildNoteMessage ALWAYS fills in the default 3 risk tags, so a real note always has risk → Note
// always comes along with it.
func TestNoteRiskCarriesReleaseNote(t *testing.T) {
	notes := []Commit{{Body: "_Release-Slug: foo\n_Release-Note: thêm loại trường liên kết\n_Blast-radius: be+web\n_DB: N/A\n_Rollback: revert PR\n"}} //znf:allow-lang
	m := NoteRiskBySlug(notes)
	rm := LinkSpec(Change{Slug: "foo"}, nil, m)
	if rm.Note != "thêm loại trường liên kết" { //znf:allow-lang
		t.Errorf("RiskMeta.Note must be pulled from the _Release-Note trailer: %+v", rm)
	}
}

func TestIsReleaseNote(t *testing.T) {
	if !IsReleaseNote(Commit{Subject: "chore(release): note x", Body: "_Release-Slug: x\n"}) {
		t.Errorf("a commit with the right subject + trailer must be recognized as a release note")
	}
	if IsReleaseNote(Commit{Subject: "feat: normal", Body: "feat: normal\n"}) {
		t.Errorf("a regular commit is not a release note")
	}
}

func TestIsReleaseNoteRequiresSubjectNotJustTrailer(t *testing.T) {
	// A squash-merge can swallow the _Release-Slug: trailer into a feat-commit's body — the
	// subject is NOT "chore(release): note", so it must NOT be mistaken for a note (and lost from the changelog).
	squashed := Commit{Subject: "feat(foo): implement foo", Body: "_Release-Slug: foo\n_Blast-radius: be\n"}
	if IsReleaseNote(squashed) {
		t.Errorf("a feat-commit that accidentally carries the _Release-Slug: trailer must NOT be treated as a note-commit: %+v", squashed)
	}
}
