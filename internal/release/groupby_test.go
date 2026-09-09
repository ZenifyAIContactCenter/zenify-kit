package release

import "testing"

// mergeKey/expandKey build the fakeRunner key in the format RangeCommitsGrouped uses.
func mergeKey(from, to string) string {
	return "log --first-parent " + logFormat + " " + from + ".." + to
}

func expandKey(mergeSHA string) string {
	return "log " + logFormat + " " + mergeSHA + "^1.." + mergeSHA + "^2"
}

// rec assembles one record in logFormat: sha\x1fsubject\x1fauthor\x1fbody\x1e.
func rec(sha, subject, author, body string) string {
	return sha + sep + subject + sep + author + sep + body + recSep
}

// SC-11: 1 PR feat/foo expands into feat(x)+fix(y)+note-commit (_Release-Slug: foo + 3 risk tags) →
// must group into EXACTLY ONE Change slug=foo, type=feat (not "other"/hidden), risk enriching that
// same row. Must FAIL on pre-redesign code (the old keyOf did not prioritize PRBranch → the note
// enriched a hidden merge-row).
func TestGroupByPR_NoteEnrichesVisibleFeatureRow(t *testing.T) {
	f := fakeRunner{out: map[string]string{
		mergeKey("origin/release83", "origin/release84"): rec("m00", "Merge pull request #7 from org/feat/foo", "admin", ""),
		expandKey("m00"): rec("c1", "feat(x): add x", "namph", "") +
			rec("c2", "fix(y): fix y", "namph", "") +
			rec("c3", "chore(release): note", "bot", "_Release-Slug: foo\n_Blast-radius: Z\n_DB: N/A\n_Rollback: revert PR"),
	}}
	gcs, err := RangeCommitsGrouped(f, "/x", "origin/release83", "origin/release84")
	if err != nil {
		t.Fatalf("RangeCommitsGrouped err=%v", err)
	}
	var notes, gfeats []Commit
	for _, c := range gcs {
		if IsReleaseNote(c) {
			notes = append(notes, c)
		} else {
			gfeats = append(gfeats, c)
		}
	}
	ch := Aggregate(gfeats, nil)
	if len(ch) != 1 {
		t.Fatalf("want exactly 1 Change (PR grouped into 1 row), got %d: %+v", len(ch), ch)
	}
	if ch[0].Slug != "foo" {
		t.Fatalf("Slug must be 'foo' (the PR's branch-slug), got %q", ch[0].Slug)
	}
	if ch[0].Type != "feat" {
		t.Fatalf("Type must be 'feat' (SHOWS in the Features section), got %q", ch[0].Type)
	}
	noteMap := NoteRiskBySlug(notes)
	risk := LinkSpec(ch[0], nil, noteMap)
	if risk.BlastRadius != "Z" {
		t.Errorf("BlastRadius want 'Z', got %q", risk.BlastRadius)
	}
	if risk.SpecPath != "note" {
		t.Errorf("SpecPath want sentinel 'note', got %q", risk.SpecPath)
	}
}

// SC-12: same PR but with NO note-commit in the expanded range → still exactly 1 Change slug=foo
// type=feat, risk empty (the no-note path invariant).
func TestGroupByPR_NoNoteStillOneVisibleChange(t *testing.T) {
	f := fakeRunner{out: map[string]string{
		mergeKey("origin/release83", "origin/release84"): rec("m00", "Merge pull request #7 from org/feat/foo", "admin", ""),
		expandKey("m00"): rec("c1", "feat(x): add x", "namph", "") +
			rec("c2", "fix(y): fix y", "namph", ""),
	}}
	gcs, err := RangeCommitsGrouped(f, "/x", "origin/release83", "origin/release84")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	var gfeats []Commit
	for _, c := range gcs {
		if !IsReleaseNote(c) {
			gfeats = append(gfeats, c)
		}
	}
	ch := Aggregate(gfeats, nil)
	if len(ch) != 1 || ch[0].Slug != "foo" || ch[0].Type != "feat" {
		t.Fatalf("want 1 Change slug=foo type=feat: %+v", ch)
	}
	risk := LinkSpec(ch[0], nil, NoteRiskBySlug(nil))
	if risk.SpecPath != "" {
		t.Errorf("risk must be empty when there is no note + no spec: %+v", risk)
	}
}

// branchTypeFloor: PR feat/foo whose expanded commits are ALL chore → Type must still be feat thanks to the branch-prefix floor.
func TestGroupByPR_BranchTypeFloorLiftsChoreOnlyPR(t *testing.T) {
	f := fakeRunner{out: map[string]string{
		mergeKey("origin/release83", "origin/release84"): rec("m00", "Merge pull request #7 from org/feat/foo", "admin", ""),
		expandKey("m00"): rec("c1", "chore(deps): bump", "namph", ""),
	}}
	gcs, err := RangeCommitsGrouped(f, "/x", "origin/release83", "origin/release84")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	ch := Aggregate(gcs, nil)
	if len(ch) != 1 || ch[0].Type != "feat" {
		t.Fatalf("the branch-prefix floor must lift Type to feat: %+v", ch)
	}
}

// Degrade: a direct-push commit (mainline non-merge, PRBranch="") with scope feat(z) must still
// create its own Change by scope (M6c2 bucket-by-scope), not be swallowed into any PR.
func TestGroupByPR_DirectPushCommitBucketsByScope(t *testing.T) {
	f := fakeRunner{out: map[string]string{
		mergeKey("origin/release83", "origin/release84"): rec("m00", "Merge pull request #7 from org/feat/foo", "admin", "") +
			rec("d1", "feat(z): z", "namph", ""),
		expandKey("m00"): rec("c1", "feat(x): add x", "namph", ""),
	}}
	gcs, err := RangeCommitsGrouped(f, "/x", "origin/release83", "origin/release84")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	ch := Aggregate(gcs, nil)
	byslug := map[string]Change{}
	for _, c := range ch {
		byslug[c.Slug] = c
	}
	if z, ok := byslug["z"]; !ok || z.Type != "feat" || len(z.Commits) != 1 {
		t.Fatalf("the direct-push commit with scope z must create its own Change: %+v", ch)
	}
	if foo, ok := byslug["foo"]; !ok || len(foo.Commits) != 2 {
		t.Fatalf("PR foo must merge the merge-commit + its expansion: %+v", ch)
	}
}

// No PR-merge in range → degrades exactly like flat RangeCommits (every commit has PRBranch="").
func TestRangeCommitsGrouped_NoMergeDegradesToFlat(t *testing.T) {
	f := fakeRunner{out: map[string]string{
		mergeKey("origin/release83", "origin/release84"): rec("1", "fix: a", "namph", ""),
	}}
	gcs, err := RangeCommitsGrouped(f, "/x", "origin/release83", "origin/release84")
	if err != nil || len(gcs) != 1 || gcs[0].PRBranch != "" {
		t.Fatalf("degrade: gcs=%+v err=%v", gcs, err)
	}
}
