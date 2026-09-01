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
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"version":1,"worktrees":[
		{"slug":"b2b2-restore","type":"feat","branch":"namph/feat/x","ports":[3251],"portBase":3251,"path":".worktrees/b2b2-restore","createdAt":"2026-09-01T00:00:00Z"}]}`
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(body), 0o644); err != nil {
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

func TestReadIndex_MissingIsEmpty(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	m, err := ReadIndex()
	if err != nil {
		t.Fatalf("missing index must not error: %v", err)
	}
	if len(m) != 0 {
		t.Fatalf("want empty, got %d", len(m))
	}
}
