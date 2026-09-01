package wt

import (
	"testing"
)

func TestIndexUpsert_AddsAndDedups(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repo := "/Users/x/repo-a"
	if err := IndexUpsert(repo, "foo", 1, "h", 1); err != nil {
		t.Fatal(err)
	}
	if err := IndexUpsert(repo, "foo", 1, "h", 2); err != nil { // idempotent
		t.Fatal(err)
	}
	if err := IndexUpsert(repo, "bar", 1, "h", 3); err != nil {
		t.Fatal(err)
	}
	idx, err := ReadIndex()
	if err != nil {
		t.Fatal(err)
	}
	got := idx[repo]
	if len(got) != 2 {
		t.Fatalf("want 2 slugs (deduped), got %v", got)
	}
	seen := map[string]bool{}
	for _, s := range got {
		seen[s] = true
	}
	if !seen["foo"] || !seen["bar"] {
		t.Fatalf("missing slug: %v", got)
	}
}

func TestIndexRemove_DropsSlugAndEmptyRepo(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repo := "/Users/x/repo-a"
	_ = IndexUpsert(repo, "foo", 1, "h", 1)
	_ = IndexUpsert(repo, "bar", 1, "h", 2)
	if err := IndexRemove(repo, "foo", 1, "h", 3); err != nil {
		t.Fatal(err)
	}
	idx, _ := ReadIndex()
	if len(idx[repo]) != 1 || idx[repo][0] != "bar" {
		t.Fatalf("after removing foo want [bar], got %v", idx[repo])
	}
	if err := IndexRemove(repo, "bar", 1, "h", 4); err != nil {
		t.Fatal(err)
	}
	idx, _ = ReadIndex()
	if _, ok := idx[repo]; ok {
		t.Fatalf("repo key should be gone when last slug removed, got %v", idx[repo])
	}
}
