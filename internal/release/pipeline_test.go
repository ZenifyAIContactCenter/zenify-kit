package release

import "testing"

// TestPipelineNoteAssociatesBySlugNotSeparateChange lái NGUYÊN pipeline thật (partition →
// Aggregate(feats) → NoteRiskBySlug(notes) → LinkSpec) với một merge-PR commit + một note-commit
// mang _Release-Slug. Trước fix: note-commit lọt vào Aggregate và tự thành Change riêng
// (slug "release", type chore) — feature Change không bao giờ thấy risk của note. Test này PHẢI
// fail nếu chạy trên code trước fix (note không bị lọc khỏi feats, LinkSpec không nhận map).
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
		t.Fatalf("phải gom thành ĐÚNG 1 Change (note-commit KHÔNG được tách thành Change 'release' riêng): %+v", changes)
	}
	ch := changes[0]
	if ch.Slug != "foo" {
		t.Fatalf("slug của Change duy nhất phải là 'foo': %+v", ch)
	}
	if ch.Type != "feat" {
		t.Fatalf("Type phải là 'feat' (từ merge/branch), KHÔNG phải 'chore': %+v", ch)
	}

	noteMap := NoteRiskBySlug(notes)
	risk := LinkSpec(ch, nil, noteMap)
	if risk.BlastRadius != "be+web" || risk.DB != "N/A" || risk.Rollback != "revert PR" {
		t.Fatalf("Change 'foo' phải nhận risk của note qua liên-kết slug: %+v", risk)
	}
	if risk.SpecPath == "" {
		t.Errorf("risk.SpecPath phải khác rỗng để đếm là linked: %+v", risk)
	}
}

// TestPipelineDegradeSafeWithoutNote: cùng cấu trúc nhưng KHÔNG có note-commit — hành vi
// phải giống hệt trước khi tier-0-by-slug tồn tại (risk rỗng khi không note, không spec).
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
		t.Fatalf("không có note-commit nào trong input, notes phải rỗng: %+v", notes)
	}

	changes := Aggregate(feats, nil)
	if len(changes) != 1 || changes[0].Type != "feat" {
		t.Fatalf("path không-note phải giữ nguyên hành vi cũ (1 Change, type feat): %+v", changes)
	}
	risk := LinkSpec(changes[0], nil, NoteRiskBySlug(notes))
	if risk.SpecPath != "" {
		t.Fatalf("không note, không spec → risk phải rỗng (unknown): %+v", risk)
	}
}
