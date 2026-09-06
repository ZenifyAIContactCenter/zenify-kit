package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGateParticipantsFiltersSharedStore(t *testing.T) {
	ws := t.TempDir()
	// repo A: sharedStore true
	mkRepo(t, ws, "repoA", `{"abbrev":"a","gate":{"sharedStore":true,"dbAccessor":"db_read","accessPatterns":["models.mongo.*"]}}`)
	// repo B: sharedStore false
	mkRepo(t, ws, "repoB", `{"abbrev":"b","gate":{"sharedStore":false}}`)
	// thư mục rác không phải repo (không có .claude/worktree.json) → bỏ qua
	if err := os.MkdirAll(filepath.Join(ws, "notarepo"), 0o750); err != nil {
		t.Fatal(err)
	}
	ps, err := gateParticipants(ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || ps[0].Name != "repoA" {
		t.Fatalf("participants = %+v, want chỉ repoA", ps)
	}
	if ps[0].DBAccessor != "db_read" || len(ps[0].AccessPatterns) != 1 {
		t.Fatalf("repoA thiếu accessPatterns/dbAccessor: %+v", ps[0])
	}
}

func TestGateParticipantsFindsNested(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repos", "svc-a")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `{"abbrev":"a","baseRef":"origin/staging","gate":{"sharedStore":true,"accessPatterns":["users"]}}`
	if err := os.WriteFile(filepath.Join(repo, ".claude", "worktree.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	ps, err := gateParticipants(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || ps[0].Name != "svc-a" {
		t.Fatalf("phải tìm thấy svc-a ở repos/, được %+v", ps)
	}
}

func mkRepo(t *testing.T, ws, name, cfg string) {
	t.Helper()
	dir := filepath.Join(ws, name, ".claude")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "worktree.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
}
