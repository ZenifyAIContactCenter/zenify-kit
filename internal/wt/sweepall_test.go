package wt

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
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

// boundedFetch wraps a Runner with a hard per-call timeout, the same shape
// gitx.TimeoutRunner gives the real `wt sweep --all` fetch fan-out — but built
// on mtStub's `delay` instead of gitx's unexported process-swap seam, so this
// package can exercise the fail-open path end to end.
type boundedFetch struct {
	inner   gitx.Runner
	timeout time.Duration
}

func (b boundedFetch) Run(dir string, args ...string) ([]byte, error) {
	type res struct {
		out []byte
		err error
	}
	ch := make(chan res, 1)
	go func() {
		out, err := b.inner.Run(dir, args...)
		ch <- res{out, err}
	}()
	select {
	case r := <-ch:
		return r.out, r.err
	case <-time.After(b.timeout):
		return nil, context.DeadlineExceeded
	}
}

// SC-7 end to end: a fetch that hangs past the timeout must not hang
// RunSweepAll — the slow repo is reported fetch-failed and the sweep still
// runs for every repo (fail-open), all within a bounded wall-clock time.
func TestRunSweepAll_SlowFetchTimesOutAndFailsOpen(t *testing.T) {
	ws := t.TempDir()
	a := filepath.Join(ws, "repos", "a")
	b := filepath.Join(ws, "repos", "b")
	for _, r := range []string{a, b} {
		if err := os.MkdirAll(r, 0o750); err != nil {
			t.Fatal(err)
		}
		seedSweepRepo(t, r, nil)
	}
	fetch := &mtStub{
		out:   map[string]string{},
		delay: map[string]time.Duration{b + "|fetch origin --quiet": 300 * time.Millisecond},
	}
	sweep := &mtStub{out: map[string]string{
		a + "|worktree list --porcelain": "worktree " + a + "\n\n",
		b + "|worktree list --porcelain": "worktree " + b + "\n\n",
	}}
	bounded := boundedFetch{inner: fetch, timeout: 50 * time.Millisecond}
	var out, errb bytes.Buffer
	start := time.Now()
	err := RunSweepAll(SweepAllOptions{WorkspaceRoot: ws, Repos: []string{a, b}, Host: "h", Pid: 1, Now: 1,
		Runner: sweep, FetchRunner: bounded, Stdout: &out, Stderr: &errb})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("RunSweepAll must fail open on a slow fetch, took %v", elapsed)
	}
	if !strings.Contains(errb.String(), "wt: b: fetch failed") {
		t.Fatalf("expected fetch-timeout warning for b, got %q", errb.String())
	}
	if !strings.Contains(out.String(), "== a ==") || !strings.Contains(out.String(), "== b ==") {
		t.Fatalf("both repos must still be swept despite b's slow fetch: %q", out.String())
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

func TestRunSweepAll_DryRunKeepsLegacyIndex(t *testing.T) {
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
	if err := RunSweepAll(SweepAllOptions{WorkspaceRoot: t.TempDir(), Repos: []string{}, DryRun: true,
		Runner: &mtStub{}, FetchRunner: &mtStub{}, Stdout: &out, Stderr: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("dry run must not remove legacy index: %v", err)
	}
	if !strings.Contains(out.String(), "would remove legacy wt-index.json") {
		t.Fatalf("expected would-remove note, got %q", out.String())
	}
}
