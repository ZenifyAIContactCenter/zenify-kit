package release

import "testing"

// TestPipelineNoteAssociatesBySlugNotSeparateChange drives the WHOLE real pipeline (partition →
// Aggregate(feats) → NoteRiskBySlug(notes) → LinkSpec) with one merge-PR commit + one note-commit
// carrying _Release-Slug. Before the fix: the note-commit leaked into Aggregate and became its own
// Change (slug "release", type chore) — the feature Change never saw the note's risk. This test
// MUST fail against pre-fix code (note not filtered out of feats, LinkSpec not given the map).
func TestPipelineNoteAssociatesBySlugNotSeparateChange(t *testing.T) {
	merge := Commit{SHA: "aaa", Subject: "Merge pull request #7 from org/namph/feat/foo", Author: "namph"}
	if b := ParseMergeBranch(merge.Subject); b != "" {
		merge.Merge, merge.Branch = true, b
	}
	merge.Type = ClassifyType(merge.Subject)
	feat := Commit{SHA: "bbb", Subject: "feat(foo): implement foo", Author: "namph", Type: ClassifyType("feat(foo): implement foo")}
	note := Commit{
		SHA: "ccc", Subject: "chore(release): note foo", Author: "namph",
		Type: ClassifyType("chore(release): note foo"),
		Body: "_Release-Slug: foo\n_Release-Note: add foo\n_Blast-radius: be+web\n_DB: N/A\n_Rollback: revert PR\n",
	}

	all := []Commit{merge, feat, note}
	var feats, notes []Commit
	for _, c := range all {
		if IsReleaseNote(c) {
			notes = append(notes, c)
		} else {
			feats = append(feats, c)
		}
	}

	changes := Aggregate(feats, nil)
	if len(changes) != 1 {
		t.Fatalf("must group into EXACTLY 1 Change (the note-commit must NOT split off its own 'release' Change): %+v", changes)
	}
	ch := changes[0]
	if ch.Slug != "foo" {
		t.Fatalf("the single Change's slug must be 'foo': %+v", ch)
	}
	if ch.Type != "feat" {
		t.Fatalf("Type must be 'feat' (from the merge/branch), NOT 'chore': %+v", ch)
	}

	noteMap := NoteRiskBySlug(notes)
	risk := LinkSpec(ch, nil, noteMap)
	if risk.BlastRadius != "be+web" || risk.DB != "N/A" || risk.Rollback != "revert PR" {
		t.Fatalf("Change 'foo' must receive the note's risk via the slug link: %+v", risk)
	}
	if risk.SpecPath == "" {
		t.Errorf("risk.SpecPath must be non-empty to count as linked: %+v", risk)
	}
}

// TestPipelineDegradeSafeWithoutNote: same structure but with NO note-commit — behavior must
// match exactly what it was before tier-0-by-slug existed (risk empty when there is no note, no spec).
func TestPipelineDegradeSafeWithoutNote(t *testing.T) {
	merge := Commit{SHA: "aaa", Subject: "Merge pull request #7 from org/namph/feat/foo", Author: "namph"}
	if b := ParseMergeBranch(merge.Subject); b != "" {
		merge.Merge, merge.Branch = true, b
	}
	merge.Type = ClassifyType(merge.Subject)
	feat := Commit{SHA: "bbb", Subject: "feat(foo): implement foo", Author: "namph", Type: ClassifyType("feat(foo): implement foo")}

	all := []Commit{merge, feat}
	var feats, notes []Commit
	for _, c := range all {
		if IsReleaseNote(c) {
			notes = append(notes, c)
		} else {
			feats = append(feats, c)
		}
	}
	if len(notes) != 0 {
		t.Fatalf("no note-commit in the input, notes must be empty: %+v", notes)
	}

	changes := Aggregate(feats, nil)
	if len(changes) != 1 || changes[0].Type != "feat" {
		t.Fatalf("the no-note path must keep the old behavior (1 Change, type feat): %+v", changes)
	}
	risk := LinkSpec(changes[0], nil, NoteRiskBySlug(notes))
	if risk.SpecPath != "" {
		t.Fatalf("no note, no spec → risk must be empty (unknown): %+v", risk)
	}
}
