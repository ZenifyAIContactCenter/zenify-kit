package datarun

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
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

func openFile(p string) (io.ReadCloser, error) { return os.Open(p) } //nolint:gosec // G304 -- test temp files

func mustKey(t *testing.T, argv []string, cwd string) string {
	t.Helper()
	k, err := Key(argv, cwd, openFile)
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
	_, err := Key([]string{"node", dir, "plain"}, dir, func(string) (io.ReadCloser, error) { called = true; return nil, nil })
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Errorf("open called for a directory or non-file arg")
	}
}

func newStore(t *testing.T) (*Store, *time.Time) {
	now := time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC)
	return &Store{Dir: filepath.Join(t.TempDir(), "data-runs"), Now: func() time.Time { return now }}, &now
}

func TestStore_FindAfterDryRun(t *testing.T) {
	s, _ := newStore(t)
	if err := s.RecordDryRun("k", "staging", false, "me", 0); err != nil {
		t.Fatal(err)
	}
	id, err := s.FindForWrite("k", "staging", false)
	if err != nil || id == "" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if _, err := s.FindForWrite("k", "prod", false); !errors.Is(err, ErrNoDryRun) {
		t.Errorf("other env: err=%v", err)
	}
	if _, err := s.FindForWrite("other", "staging", false); !errors.Is(err, ErrNoDryRun) {
		t.Errorf("other key: err=%v", err)
	}
	fi, err := os.Stat(s.Dir)
	if err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("dir mode: %v %v", fi, err)
	}
}

func TestStore_FailedDryRun(t *testing.T) {
	s, _ := newStore(t)
	if err := s.RecordDryRun("k", "staging", false, "me", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FindForWrite("k", "staging", false); !errors.Is(err, ErrNoDryRun) {
		t.Errorf("err=%v", err)
	}
}

func TestStore_Window(t *testing.T) {
	s, now := newStore(t)
	if err := s.RecordDryRun("k", "staging", false, "me", 0); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(59 * time.Minute)
	if _, err := s.FindForWrite("k", "staging", false); err != nil {
		t.Errorf("at 59m: %v", err)
	}
	*now = now.Add(2 * time.Minute)
	if _, err := s.FindForWrite("k", "staging", false); !errors.Is(err, ErrNoDryRun) {
		t.Errorf("at 61m: err=%v", err)
	}
}

func TestStore_SingleUse(t *testing.T) {
	s, _ := newStore(t)
	if err := s.RecordDryRun("k", "staging", false, "me", 0); err != nil {
		t.Fatal(err)
	}
	id, err := s.FindForWrite("k", "staging", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkUsed(id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FindForWrite("k", "staging", false); !errors.Is(err, ErrNoDryRun) {
		t.Errorf("after MarkUsed: err=%v", err)
	}
}

func TestStore_TwoRecordsSameInstant(t *testing.T) {
	s, _ := newStore(t)
	for i := 0; i < 2; i++ {
		if err := s.RecordDryRun("k", "staging", false, "me", 0); err != nil {
			t.Fatal(err)
		}
	}
	id1, _ := s.FindForWrite("k", "staging", false)
	if err := s.MarkUsed(id1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FindForWrite("k", "staging", false); err != nil {
		t.Errorf("second record should remain usable: %v", err)
	}
}

func TestStore_ToleratesJunk(t *testing.T) {
	s, _ := newStore(t)
	if _, err := s.FindForWrite("k", "e", false); !errors.Is(err, ErrNoDryRun) {
		t.Errorf("missing dir: %v", err)
	}
	if err := s.RecordDryRun("k", "e", false, "me", 0); err != nil {
		t.Fatal(err)
	}
	writeFile(t, s.Dir, "junk.json", "{not json")
	writeFile(t, s.Dir, "notes.txt", "hi")
	if err := os.Mkdir(filepath.Join(s.Dir, "sub.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FindForWrite("k", "e", false); err != nil {
		t.Errorf("junk broke lookup: %v", err)
	}
}

func TestStore_MarkUsedRejectsBadID(t *testing.T) {
	s, _ := newStore(t)
	if err := s.MarkUsed("../x"); err == nil {
		t.Errorf("path traversal id accepted")
	}
}

func TestStore_MarkUsedTwiceSecondLoses(t *testing.T) {
	s, _ := newStore(t)
	if err := s.RecordDryRun("k", "staging", false, "me", 0); err != nil {
		t.Fatal(err)
	}
	id, err := s.FindForWrite("k", "staging", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkUsed(id); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkUsed(id); !errors.Is(err, ErrNoDryRun) {
		t.Errorf("second MarkUsed: err=%v, want ErrNoDryRun", err)
	}
	if _, err := s.FindForWrite("k", "staging", false); !errors.Is(err, ErrNoDryRun) {
		t.Errorf("FindForWrite after MarkUsed: err=%v", err)
	}
}

// closeCounter records that Key read a file through the open seam and closed it.
type closeCounter struct {
	io.Reader
	closed *int
}

func (c closeCounter) Close() error { *c.closed++; return nil }

func TestKey_StreamsAndClosesFiles(t *testing.T) {
	dir := t.TempDir()
	x := writeFile(t, dir, "x.js", "a")
	y := writeFile(t, dir, "y.js", "b")
	closed := 0
	open := func(p string) (io.ReadCloser, error) {
		f, err := openFile(p)
		if err != nil {
			return nil, err
		}
		b, _ := io.ReadAll(f)
		_ = f.Close()
		return closeCounter{Reader: strings.NewReader(string(b)), closed: &closed}, nil
	}
	k, err := Key([]string{"node", x, y}, dir, open)
	if err != nil {
		t.Fatal(err)
	}
	if closed != 2 {
		t.Errorf("closed %d files, want 2", closed)
	}
	if k != mustKey(t, []string{"node", x, y}, dir) {
		t.Errorf("key is not stable for the same inputs")
	}
}

func TestStore_LumiMustMatch(t *testing.T) {
	s, _ := newStore(t)
	if err := s.RecordDryRun("k", "stg", false, "me", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FindForWrite("k", "stg", true); !errors.Is(err, ErrNoDryRun) {
		t.Errorf("dry-run without lumi unlocked a lumi write: err=%v", err)
	}
	if err := s.RecordDryRun("k2", "stg", true, "me", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FindForWrite("k2", "stg", false); !errors.Is(err, ErrNoDryRun) {
		t.Errorf("lumi dry-run unlocked a non-lumi write: err=%v", err)
	}
	if _, err := s.FindForWrite("k2", "stg", true); err != nil {
		t.Errorf("matching lumi write refused: %v", err)
	}
}
