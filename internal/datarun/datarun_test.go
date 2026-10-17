package datarun

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func mustKey(t *testing.T, argv []string, cwd string) string {
	t.Helper()
	k, err := Key(argv, cwd, os.ReadFile)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestKey_IgnoresDryRunToken(t *testing.T) {
	dir := t.TempDir()
	x := writeFile(t, dir, "x.js", "a")
	base := mustKey(t, []string{"node", x}, dir)
	if got := mustKey(t, []string{"node", x, "--dry-run"}, dir); got != base {
		t.Errorf("--dry-run changed the key")
	}
	if got := mustKey(t, []string{"node", x, "--dry-run=true"}, dir); got == base {
		t.Errorf("--dry-run=true must produce a different key")
	}
}

func TestKey_FileContentChanges(t *testing.T) {
	dir := t.TempDir()
	x := writeFile(t, dir, "x.js", "a")
	k1 := mustKey(t, []string{"node", x}, dir)
	writeFile(t, dir, "x.js", "b")
	if k2 := mustKey(t, []string{"node", x}, dir); k2 == k1 {
		t.Errorf("content change did not change the key")
	}
	if k3 := mustKey(t, []string{"node", x}, t.TempDir()); k3 == mustKey(t, []string{"node", x}, dir) {
		t.Errorf("cwd change did not change the key")
	}
}

func TestKey_Unambiguous(t *testing.T) {
	dir := t.TempDir()
	if mustKey(t, []string{"a b"}, dir) == mustKey(t, []string{"a", "b"}, dir) {
		t.Errorf("argv boundaries are ambiguous")
	}
}

func TestKey_NonFileArgsNotRead(t *testing.T) {
	dir := t.TempDir()
	called := false
	_, err := Key([]string{"node", dir, "plain"}, dir, func(string) ([]byte, error) { called = true; return nil, nil })
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Errorf("readFile called for a directory or non-file arg")
	}
}

func newStore(t *testing.T) (*Store, *time.Time) {
	now := time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC)
	return &Store{Dir: filepath.Join(t.TempDir(), "data-runs"), Now: func() time.Time { return now }}, &now
}

func TestStore_FindAfterDryRun(t *testing.T) {
	s, _ := newStore(t)
	if err := s.RecordDryRun("k", "staging", "me", 0); err != nil {
		t.Fatal(err)
	}
	id, err := s.FindForWrite("k", "staging")
	if err != nil || id == "" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if _, err := s.FindForWrite("k", "prod"); !errors.Is(err, ErrNoDryRun) {
		t.Errorf("other env: err=%v", err)
	}
	if _, err := s.FindForWrite("other", "staging"); !errors.Is(err, ErrNoDryRun) {
		t.Errorf("other key: err=%v", err)
	}
	fi, err := os.Stat(s.Dir)
	if err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("dir mode: %v %v", fi, err)
	}
}

func TestStore_FailedDryRun(t *testing.T) {
	s, _ := newStore(t)
	if err := s.RecordDryRun("k", "staging", "me", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FindForWrite("k", "staging"); !errors.Is(err, ErrNoDryRun) {
		t.Errorf("err=%v", err)
	}
}

func TestStore_Window(t *testing.T) {
	s, now := newStore(t)
	if err := s.RecordDryRun("k", "staging", "me", 0); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(59 * time.Minute)
	if _, err := s.FindForWrite("k", "staging"); err != nil {
		t.Errorf("at 59m: %v", err)
	}
	*now = now.Add(2 * time.Minute)
	if _, err := s.FindForWrite("k", "staging"); !errors.Is(err, ErrNoDryRun) {
		t.Errorf("at 61m: err=%v", err)
	}
}

func TestStore_SingleUse(t *testing.T) {
	s, _ := newStore(t)
	if err := s.RecordDryRun("k", "staging", "me", 0); err != nil {
		t.Fatal(err)
	}
	id, err := s.FindForWrite("k", "staging")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkUsed(id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FindForWrite("k", "staging"); !errors.Is(err, ErrNoDryRun) {
		t.Errorf("after MarkUsed: err=%v", err)
	}
}

func TestStore_TwoRecordsSameInstant(t *testing.T) {
	s, _ := newStore(t)
	for i := 0; i < 2; i++ {
		if err := s.RecordDryRun("k", "staging", "me", 0); err != nil {
			t.Fatal(err)
		}
	}
	id1, _ := s.FindForWrite("k", "staging")
	if err := s.MarkUsed(id1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FindForWrite("k", "staging"); err != nil {
		t.Errorf("second record should remain usable: %v", err)
	}
}

func TestStore_ToleratesJunk(t *testing.T) {
	s, _ := newStore(t)
	if _, err := s.FindForWrite("k", "e"); !errors.Is(err, ErrNoDryRun) {
		t.Errorf("missing dir: %v", err)
	}
	if err := s.RecordDryRun("k", "e", "me", 0); err != nil {
		t.Fatal(err)
	}
	writeFile(t, s.Dir, "junk.json", "{not json")
	writeFile(t, s.Dir, "notes.txt", "hi")
	if err := os.Mkdir(filepath.Join(s.Dir, "sub.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FindForWrite("k", "e"); err != nil {
		t.Errorf("junk broke lookup: %v", err)
	}
}

func TestStore_MarkUsedRejectsBadID(t *testing.T) {
	s, _ := newStore(t)
	if err := s.MarkUsed("../x"); err == nil {
		t.Errorf("path traversal id accepted")
	}
}
