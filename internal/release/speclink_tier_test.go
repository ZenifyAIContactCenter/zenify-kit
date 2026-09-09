package release

import "testing"

func specFor(slug string) SpecMeta {
	return SpecMeta{Path: "specs/zenify-kit/2026-09-09-" + slug + "-design.md", Slug: NormalizeKey(slug)}
}

func TestLinkSpecTier_TrailerBeatsFuzzy(t *testing.T) {
	specs := []SpecMeta{specFor("linked-fields")}
	ch := Change{Slug: "linked-fields", Commits: []Commit{{
		Subject: "feat: x",
		Body:    "Spec: specs/zenify-kit/2026-09-09-linked-fields-design.md",
	}}}
	rm, tier := LinkSpecTier(ch, specs, nil)
	if tier != TierTrailer {
		t.Fatalf("tier = %d, want TierTrailer(%d)", tier, TierTrailer)
	}
	if rm.SpecPath != specs[0].Path {
		t.Fatalf("SpecPath = %q, want %q", rm.SpecPath, specs[0].Path)
	}
}

func TestLinkSpecTier_FuzzySlug(t *testing.T) {
	specs := []SpecMeta{specFor("linked-fields")}
	ch := Change{Slug: "linked-fields-extra", Commits: []Commit{{Subject: "feat: x", Body: ""}}}
	_, tier := LinkSpecTier(ch, specs, nil)
	if tier != TierSlugFuzzy {
		t.Fatalf("tier = %d, want TierSlugFuzzy(%d)", tier, TierSlugFuzzy)
	}
}

func TestLinkSpecTier_ExactSlug(t *testing.T) {
	specs := []SpecMeta{specFor("linked-fields")}
	ch := Change{Slug: "linked-fields", Commits: []Commit{{Subject: "feat: x", Body: ""}}}
	_, tier := LinkSpecTier(ch, specs, nil)
	if tier != TierSlugExact {
		t.Fatalf("tier = %d, want TierSlugExact(%d)", tier, TierSlugExact)
	}
}

func TestLinkSpecTier_NoteWins(t *testing.T) {
	notes := map[string]RiskMeta{"linked-fields": {SpecPath: "note", BlastRadius: "x"}}
	ch := Change{Slug: "linked-fields", Commits: []Commit{{Body: "Spec: whatever"}}}
	rm, tier := LinkSpecTier(ch, nil, notes)
	if tier != TierNote {
		t.Fatalf("tier = %d, want TierNote(%d)", tier, TierNote)
	}
	if rm.BlastRadius != "x" {
		t.Fatalf("rm.BlastRadius = %q, want x", rm.BlastRadius)
	}
}

func TestLinkSpecTier_None(t *testing.T) {
	_, tier := LinkSpecTier(Change{Slug: "unrelated"}, []SpecMeta{specFor("other")}, nil)
	if tier != TierNone {
		t.Fatalf("tier = %d, want TierNone(%d)", tier, TierNone)
	}
}
