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
	r := LinkSpec(ch, specs)
	if r.SpecPath == "" || r.BlastRadius != "X" {
		t.Fatalf("trailer link: %+v", r)
	}
}

func TestLinkSpecSlugMatch(t *testing.T) {
	specs := []SpecMeta{{Path: "specs/be/2026-08-01-linked-fields-design.md", Slug: "linked-fields", BlastRadius: "Y"}}
	ch := Change{Slug: "linked-fields"}
	r := LinkSpec(ch, specs)
	if r.SpecPath == "" || r.BlastRadius != "Y" {
		t.Fatalf("slug match: %+v", r)
	}
}

func TestLinkSpecNoMatch(t *testing.T) {
	specs := []SpecMeta{{Path: "specs/be/foo-design.md", Slug: "foo"}}
	ch := Change{Slug: "bar"}
	r := LinkSpec(ch, specs)
	if r.SpecPath != "" {
		t.Fatalf("no-match phải rỗng: %+v", r)
	}
}

func TestLinkSpecNoteTrailerWins(t *testing.T) {
	// note-commit mang tag risk → tier-0 thắng, KHÔNG cần file spec.
	ch := Change{Slug: "whatever", Commits: []Commit{
		{Body: "_Release-Note: add X\n_Blast-radius: be+web\n_DB: N/A\n_Rollback: revert PR\n"},
	}}
	rm := LinkSpec(ch, nil)
	if rm.BlastRadius != "be+web" || rm.DB != "N/A" || rm.Rollback != "revert PR" {
		t.Fatalf("tier-0 phải lấy risk từ note-commit: %+v", rm)
	}
	if rm.SpecPath == "" {
		t.Errorf("tier-0 phải set SpecPath khác rỗng (đếm coverage): %+v", rm)
	}
}

func TestLinkSpecNoteTrailerSpecPath(t *testing.T) {
	ch := Change{Slug: "x", Commits: []Commit{
		{Body: "_Blast-radius: hub\n_DB: N/A\n_Rollback: revert\nSpec: specs/zenify-kit/2026-09-07-x-design.md\n"},
	}}
	rm := LinkSpec(ch, nil)
	if rm.SpecPath != "specs/zenify-kit/2026-09-07-x-design.md" {
		t.Errorf("tier-0 lấy SpecPath từ Spec: trailer khi có: %+v", rm)
	}
}
