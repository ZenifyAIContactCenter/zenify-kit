package release

import "testing"

// Tag trên dòng riêng, wrap backtick — đúng convention spec thật (field số ở dòng trên).
// Raw-string không chứa được backtick nên dùng interpreted string với \n.
const briefSpec = "# X design\n## Brief\n1. Problem. p\n5. Service.\n   `_Blast-radius: be + web break`\n6. DB.\n   `_DB: N/A`\n8. Rollback.\n   `_Rollback: revert + watch errors`\n"

func TestParseSpecBriefTags(t *testing.T) {
	m := ParseSpecBrief("specs/be/2026-08-01-linked-fields-design.md", []byte(briefSpec))
	if m.BlastRadius != "be + web break" || m.DB != "N/A" || m.Rollback != "revert + watch errors" {
		t.Fatalf("tag parse: %+v", m)
	}
	if m.Slug != "linked-fields" && m.Slug != "linked fields" {
		// slug lấy token giữa ngày và -design
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
		t.Fatalf("no-match phải rỗng: %+v", r)
	}
}

func TestNoteRiskBySlugAndLinkSpecTier0(t *testing.T) {
	// note-commit mang _Release-Slug: + risk tag → NoteRiskBySlug gom vào map theo slug
	// chuẩn-hoá; LinkSpec tra map theo ch.Slug (KHÔNG còn quét ch.Commits).
	notes := []Commit{
		{Body: "_Release-Slug: whatever\n_Release-Note: add X\n_Blast-radius: be+web\n_DB: N/A\n_Rollback: revert PR\n"},
	}
	m := NoteRiskBySlug(notes)
	ch := Change{Slug: "whatever"}
	rm := LinkSpec(ch, nil, m)
	if rm.BlastRadius != "be+web" || rm.DB != "N/A" || rm.Rollback != "revert PR" {
		t.Fatalf("tier-0 phải lấy risk từ note map: %+v", rm)
	}
	if rm.SpecPath == "" {
		t.Errorf("tier-0 phải set SpecPath khác rỗng (đếm coverage): %+v", rm)
	}
}

func TestNoteRiskBySlugSpecPath(t *testing.T) {
	notes := []Commit{
		{Body: "_Release-Slug: x\n_Blast-radius: hub\n_DB: N/A\n_Rollback: revert\nSpec: specs/zenify-kit/2026-09-07-x-design.md\n"},
	}
	m := NoteRiskBySlug(notes)
	rm := LinkSpec(Change{Slug: "x"}, nil, m)
	if rm.SpecPath != "specs/zenify-kit/2026-09-07-x-design.md" {
		t.Errorf("tier-0 lấy SpecPath từ Spec: trailer khi có: %+v", rm)
	}
}

func TestNoteRiskBySlugNormalizesKey(t *testing.T) {
	// slug trong trailer chưa chuẩn-hoá (vd branch thô "Feat/Foo_Bar") vẫn khớp Change.Slug
	// đã NormalizeKey qua Aggregate.
	notes := []Commit{
		{Body: "_Release-Slug: Feat/Foo_Bar\n_Blast-radius: be\n"},
	}
	m := NoteRiskBySlug(notes)
	rm := LinkSpec(Change{Slug: "foo-bar"}, nil, m)
	if rm.BlastRadius != "be" {
		t.Fatalf("slug phải chuẩn-hoá để khớp: %+v (map=%+v)", rm, m)
	}
}

func TestNoteRiskBySlugSkipsNoRiskTag(t *testing.T) {
	notes := []Commit{{Body: "_Release-Slug: foo\n_Release-Note: nothing risky\n"}}
	m := NoteRiskBySlug(notes)
	if len(m) != 0 {
		t.Errorf("note không có risk tag nào không được vào map: %+v", m)
	}
}

func TestIsReleaseNote(t *testing.T) {
	if !IsReleaseNote(Commit{Body: "_Release-Slug: x\n"}) {
		t.Errorf("commit mang trailer phải nhận diện là release note")
	}
	if IsReleaseNote(Commit{Body: "feat: bình thường\n"}) {
		t.Errorf("commit thường không phải release note")
	}
}
