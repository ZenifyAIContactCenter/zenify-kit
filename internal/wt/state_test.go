package wt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadState_MissingIsEmpty(t *testing.T) {
	s, err := ReadState(t.TempDir())
	if err != nil {
		t.Fatalf("missing state must not error: %v", err)
	}
	if len(s.Worktrees) != 0 {
		t.Fatalf("want empty, got %d", len(s.Worktrees))
	}
}

func TestReadState_FindBySlug(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".wt")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	body := `{"version":1,"worktrees":[
		{"slug":"b2b2-restore","type":"feat","branch":"namph/feat/x","ports":[3251],"portBase":3251,"path":".worktrees/b2b2-restore","createdAt":"2026-09-01T00:00:00Z"}]}`
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := ReadState(root)
	if err != nil {
		t.Fatal(err)
	}
	w, ok := s.Find("b2b2-restore")
	if !ok || w.Path != ".worktrees/b2b2-restore" || len(w.Ports) != 1 || w.Ports[0] != 3251 {
		t.Fatalf("find wrong: %+v ok=%v", w, ok)
	}
	if _, ok := s.Find("nope"); ok {
		t.Fatal("unexpected find of missing slug")
	}
}

func TestReadState_OldDevPidFieldIgnored(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".wt"), 0o750); err != nil {
		t.Fatal(err)
	}
	old := `{"version":1,"worktrees":[{"slug":"a","type":"feat","branch":"namph/feat/a","path":"/x/a","createdAt":"","ports":[3201],"portBase":0,"devPid":null}]}`
	if err := os.WriteFile(filepath.Join(root, ".wt", "state.json"), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := ReadState(root)
	if err != nil {
		t.Fatalf("old state with devPid must still parse: %v", err)
	}
	if len(st.Worktrees) != 1 || st.Worktrees[0].Ports[0] != 3201 {
		t.Fatalf("unexpected state: %+v", st)
	}
}
