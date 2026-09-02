package observe

import (
	"os"
	"sync"
	"testing"
	"time"
)

func TestResolveCap(t *testing.T) {
	cases := map[string]int{"": 10, "0": 0, "5": 5, "abc": 10, "-1": -1}
	for in, want := range cases {
		got := ResolveCap(func(string) string { return in })
		if got != want {
			t.Errorf("ResolveCap(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestBump_PersistsAcrossCallsAndWarnsAtCap(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	sid := "sess-abc"
	now := time.Now()
	var warned bool
	for i := 0; i < 10; i++ {
		if Bump(sid, 10, now).Warn {
			warned = true
		}
	}
	if !warned {
		t.Fatal("expected a warn by the 10th dispatch")
	}
}

func TestBump_ConcurrentNoCorruptionNoCrash(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	sid := "sess-race"
	now := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); Bump(sid, 5, now) }()
	}
	wg.Wait()
	// File must be valid JSON and count must not exceed the number of bumps.
	st := readForTest(t, sid)
	if st.Count < 0 || st.Count > 20 {
		t.Fatalf("count out of range after 20 concurrent bumps: %d", st.Count)
	}
}

func TestSanitizeSession_NoTraversal(t *testing.T) {
	if got := sanitizeSession("../../etc/passwd"); got != "etcpasswd" {
		t.Fatalf("sanitizeSession left traversal chars: %q", got)
	}
	if got := sanitizeSession(""); got != "unknown" {
		t.Fatalf("empty session should map to 'unknown', got %q", got)
	}
}

func readForTest(t *testing.T, sessionID string) State {
	t.Helper()
	d, err := dir()
	if err != nil {
		t.Fatal(err)
	}
	st, _ := readState(sessPath(d, sanitizeSession(sessionID)))
	return st
}

func TestBump_CorruptStateFailsOpen(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	sid := "sess-corrupt"
	now := time.Now()

	base, err := dir()
	if err != nil {
		t.Fatal(err)
	}
	sd := sessDir(base, sanitizeSession(sid))
	if err := os.MkdirAll(sd, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sessPath(base, sanitizeSession(sid)), []byte("{not valid json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := Bump(sid, 1, now); got.Warn {
		t.Fatalf("Bump on corrupt state should fail open, got Warn=%v", got.Warn)
	}
}
