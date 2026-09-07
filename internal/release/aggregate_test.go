package release

import "testing"

func TestAggregateGroupsBranchAndScope(t *testing.T) {
	cs := []Commit{
		{SHA: "1", Subject: "feat(alpha): a1", Type: "feat", Author: "namph"},
		{SHA: "2", Subject: "feat(alpha): a2", Type: "feat", Author: "hungnk"},
		{SHA: "3", Subject: "Merge pull request #7 from x/feat/alpha", Type: "other", Merge: true, Branch: "feat/alpha", Author: "namph"},
		{SHA: "4", Subject: "fix(beta): b1", Type: "fix"},
		{SHA: "5", Subject: "fix(beta): b2", Type: "fix"},
	}
	ch := Aggregate(cs, nil)
	if len(ch) != 2 {
		t.Fatalf("muốn 2 change, được %d: %+v", len(ch), ch)
	}
	byslug := map[string]Change{}
	for _, c := range ch {
		byslug[c.Slug] = c
	}
	a := byslug["alpha"]
	if a.Type != "feat" || len(a.Commits) != 3 || a.PRNum != "7" || a.Title != "Alpha" {
		t.Errorf("alpha gom sai: %+v", a)
	}
	// Authors: distinct theo thứ tự gặp đầu, bỏ rỗng.
	if len(a.Authors) != 2 || a.Authors[0] != "namph" || a.Authors[1] != "hungnk" {
		t.Errorf("authors distinct first-seen: %+v", a.Authors)
	}
	// Desc: subject non-merge đầu, cắt prefix conventional-commit.
	if a.Desc != "a1" {
		t.Errorf("desc phải cắt prefix type(scope): được %q", a.Desc)
	}
	if byslug["beta"].Type != "fix" || len(byslug["beta"].Commits) != 2 {
		t.Errorf("beta: %+v", byslug["beta"])
	}
}

func TestAggregateMiscBucketNoCommitLost(t *testing.T) {
	cs := []Commit{
		{SHA: "1", Subject: "random no scope", Type: "other"},
		{SHA: "2", Subject: "chore: bump", Type: "chore"},
		{SHA: "3", Subject: "feat(x): y", Type: "feat"},
	}
	ch := Aggregate(cs, nil)
	total := 0
	for _, c := range ch {
		total += len(c.Commits)
	}
	if total != 3 {
		t.Fatalf("mất commit: tổng=%d", total)
	}
}

func TestAggregateHotfixAndNotOnStaging(t *testing.T) {
	cs := []Commit{
		{SHA: "9", Subject: "Merge pull request #9 from x/hotfix/urgent", Type: "other", Merge: true, Branch: "hotfix/urgent", Author: "admin"},
	}
	ch := Aggregate(cs, map[string]bool{"9": true})
	if len(ch) != 1 || !ch[0].IsHotfix || ch[0].Type != "hotfix" || !ch[0].NotOnStaging {
		t.Fatalf("hotfix/notOnStaging: %+v", ch)
	}
	// Chỉ có merge commit → dùng tác giả merge làm best-available (không để trống Dev).
	if len(ch[0].Authors) != 1 || ch[0].Authors[0] != "admin" {
		t.Errorf("pure-merge change phải fallback về tác giả merge: %+v", ch[0].Authors)
	}
}

func TestAggregateAuthorsExcludeMergeClicker(t *testing.T) {
	// hungnk viết code; admin chỉ bấm merge → Dev KHÔNG được liệt admin.
	cs := []Commit{
		{SHA: "1", Subject: "feat(x): a1", Type: "feat", Author: "hungnk"},
		{SHA: "2", Subject: "feat(x): a2", Type: "feat", Author: "hungnk"},
		{SHA: "3", Subject: "Merge pull request #5 from o/feat/x", Type: "other", Merge: true, Branch: "feat/x", Author: "admin"},
	}
	ch := Aggregate(cs, nil)
	if len(ch) != 1 {
		t.Fatalf("muốn 1 change: %+v", ch)
	}
	if len(ch[0].Authors) != 1 || ch[0].Authors[0] != "hungnk" {
		t.Errorf("người bấm merge (admin) không được vào Dev: %+v", ch[0].Authors)
	}
}
