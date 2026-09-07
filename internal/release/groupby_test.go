package release

import "testing"

// mergeKey/expandKey xây key fakeRunner đúng format RangeCommitsGrouped dùng.
func mergeKey(from, to string) string {
	return "log --first-parent " + logFormat + " " + from + ".." + to
}

func expandKey(mergeSHA string) string {
	return "log " + logFormat + " " + mergeSHA + "^1.." + mergeSHA + "^2"
}

// rec ráp một record theo logFormat: sha\x1fsubject\x1fauthor\x1fbody\x1e.
func rec(sha, subject, author, body string) string {
	return sha + sep + subject + sep + author + sep + body + recSep
}

// SC-11: 1 PR feat/foo bung feat(x)+fix(y)+note-commit (_Release-Slug: foo + 3 risk tag) →
// gom về ĐÚNG MỘT Change slug=foo, type=feat (không "other"/ẩn), risk enrich đúng dòng đó.
// Phải FAIL trên code trước redesign (keyOf cũ không ưu tiên PRBranch → note enrich merge-row ẩn).
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
		t.Fatalf("muốn đúng 1 Change (PR gộp về 1 dòng), được %d: %+v", len(ch), ch)
	}
	if ch[0].Slug != "foo" {
		t.Fatalf("Slug phải là 'foo' (branch-slug PR), được %q", ch[0].Slug)
	}
	if ch[0].Type != "feat" {
		t.Fatalf("Type phải 'feat' (HIỆN ở mục Tính năng), được %q", ch[0].Type)
	}
	noteMap := NoteRiskBySlug(notes)
	risk := LinkSpec(ch[0], nil, noteMap)
	if risk.BlastRadius != "Z" {
		t.Errorf("BlastRadius muốn 'Z', được %q", risk.BlastRadius)
	}
	if risk.SpecPath != "note" {
		t.Errorf("SpecPath muốn sentinel 'note', được %q", risk.SpecPath)
	}
}

// SC-12: cùng PR nhưng KHÔNG có note-commit trong khoảng bung → vẫn đúng 1 Change slug=foo
// type=feat, risk rỗng (bất biến no-note path).
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
		t.Fatalf("muốn 1 Change slug=foo type=feat: %+v", ch)
	}
	risk := LinkSpec(ch[0], nil, NoteRiskBySlug(nil))
	if risk.SpecPath != "" {
		t.Errorf("risk phải rỗng khi không có note + không có spec: %+v", risk)
	}
}

// branchTypeFloor: PR feat/foo mà commit bung CHỈ toàn chore → vẫn Type=feat nhờ sàn branch-prefix.
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
		t.Fatalf("sàn branch-prefix phải nâng Type lên feat: %+v", ch)
	}
}

// Degrade: commit đẩy thẳng (mainline non-merge, PRBranch="") có scope feat(z) vẫn tạo Change
// riêng theo scope (bucket-theo-scope M6c2), không bị nuốt vào PR nào.
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
		t.Fatalf("commit đẩy thẳng scope z phải tạo Change riêng: %+v", ch)
	}
	if foo, ok := byslug["foo"]; !ok || len(foo.Commits) != 2 {
		t.Fatalf("PR foo phải gộp merge+bung: %+v", ch)
	}
}

// Không có PR-merge trong khoảng → degrade y hệt RangeCommits phẳng (mọi commit PRBranch="").
func TestRangeCommitsGrouped_NoMergeDegradesToFlat(t *testing.T) {
	f := fakeRunner{out: map[string]string{
		mergeKey("origin/release83", "origin/release84"): rec("1", "fix: a", "namph", ""),
	}}
	gcs, err := RangeCommitsGrouped(f, "/x", "origin/release83", "origin/release84")
	if err != nil || len(gcs) != 1 || gcs[0].PRBranch != "" {
		t.Fatalf("degrade: gcs=%+v err=%v", gcs, err)
	}
}
