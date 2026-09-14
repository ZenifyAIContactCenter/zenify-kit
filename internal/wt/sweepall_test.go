package wt

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// mtStub is goroutine-safe (parallel fetch) and can delay a repo's fetch.
type mtStub struct {
	mu    sync.Mutex
	out   map[string]string
	err   map[string]error
	delay map[string]time.Duration // key "dir|args" → sleep before answering
	seen  []string
}

func (s *mtStub) Run(dir string, args ...string) ([]byte, error) {
	k := dir + "|" + strings.Join(args, " ")
	s.mu.Lock()
	d := s.delay[k]
	s.seen = append(s.seen, k)
	s.mu.Unlock()
	if d > 0 {
		time.Sleep(d)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.err[k]; ok {
		return nil, e
	}
	return []byte(s.out[k]), nil
}

func (s *mtStub) ran(k string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range s.seen {
		if x == k {
			return true
		}
	}
	return false
}

func TestRunSweepAll_FetchFailureIsPerRepoAndSweepContinues(t *testing.T) {
	ws := t.TempDir()
	a := filepath.Join(ws, "repos", "a")
	b := filepath.Join(ws, "repos", "b")
	for _, r := range []string{a, b} {
		if err := os.MkdirAll(r, 0o750); err != nil {
			t.Fatal(err)
		}
		seedSweepRepo(t, r, nil)
	}
	fetch := &mtStub{out: map[string]string{}, err: map[string]error{a + "|fetch origin --quiet": errors.New("timeout")}}
	sweep := &mtStub{out: map[string]string{
		a + "|worktree list --porcelain": "worktree " + a + "\n\n",
		b + "|worktree list --porcelain": "worktree " + b + "\n\n",
	}}
	var out, errb bytes.Buffer
	err := RunSweepAll(SweepAllOptions{WorkspaceRoot: ws, Repos: []string{a, b}, Host: "h", Pid: 1, Now: 1,
		Runner: sweep, FetchRunner: fetch, Stdout: &out, Stderr: &errb})
	if err != nil {
		t.Fatal(err)
	}
	if !fetch.ran(a+"|fetch origin --quiet") || !fetch.ran(b+"|fetch origin --quiet") {
		t.Fatalf("both repos must be fetched, seen %v", fetch.seen)
	}
	if !strings.Contains(errb.String(), "wt: a: fetch failed") || !strings.Contains(errb.String(), "may be stale") {
		t.Fatalf("expected per-repo fetch warning, got %q", errb.String())
	}
	s := out.String()
	if !strings.Contains(s, "== a ==") || !strings.Contains(s, "== b ==") || !strings.Contains(s, "wt: swept 0, left 0 across 2 repos") {
		t.Fatalf("unexpected output:\n%s", s)
	}
}

func TestRunSweepAll_RepoErrorSkippedNotFatal(t *testing.T) {
	ws := t.TempDir()
	good := filepath.Join(ws, "repos", "good")
	bad := filepath.Join(ws, "repos", "bad") // no worktree.json → Load fails
	for _, r := range []string{good, bad} {
		if err := os.MkdirAll(r, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	seedSweepRepo(t, good, nil)
	st := &mtStub{out: map[string]string{good + "|worktree list --porcelain": "worktree " + good + "\n\n"}}
	var out, errb bytes.Buffer
	if err := RunSweepAll(SweepAllOptions{WorkspaceRoot: ws, Repos: []string{bad, good}, Host: "h", Pid: 1, Now: 1,
		Runner: st, FetchRunner: st, Stdout: &out, Stderr: &errb}); err != nil {
		t.Fatalf("a failing repo must not fail the run: %v", err)
	}
	if !strings.Contains(errb.String(), "wt: bad: skipped") {
		t.Fatalf("expected skipped line, got %q", errb.String())
	}
	if !strings.Contains(out.String(), "across 1 repos") {
		t.Fatalf("K must count swept repos only: %q", out.String())
	}
}

func TestRunSweepAll_RemovesLegacyIndex(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_STATE_HOME", xdg)
	legacy := filepath.Join(xdg, "zenify", "wt-index.json")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := RunSweepAll(SweepAllOptions{WorkspaceRoot: t.TempDir(), Repos: []string{}, Runner: &mtStub{}, FetchRunner: &mtStub{}, Stdout: &out, Stderr: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatal("legacy index must be removed")
	}
	if !strings.Contains(out.String(), "wt: removed legacy wt-index.json") {
		t.Fatalf("expected removal note, got %q", out.String())
	}
}
