package hub

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRollup(t *testing.T) {
	cases := []struct {
		name string
		in   []AgentState
		want AgentState
	}{
		{"blocked beats working", []AgentState{Blocked, Working}, Blocked},
		{"done beats idle", []AgentState{Done, Idle}, Done},
		{"empty is idle", nil, Idle},
	}
	for _, c := range cases {
		if got := Rollup(c.in...); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestMarkSeen(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(AgentsDir(home), 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(id, body string) {
		if err := os.WriteFile(filepath.Join(AgentsDir(home), id+".json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("a", `{"session_id":"a","state":"done"}`)
	write("b", `{"session_id":"b","state":"working"}`)
	for _, id := range []string{"a", "b"} {
		if err := MarkSeen(home, id); err != nil {
			t.Fatalf("MarkSeen %s: %v", id, err)
		}
	}
	got := func(id string) AgentState {
		a, err := readAgent(home, id)
		if err != nil || a == nil {
			t.Fatalf("readAgent %s: %v %v", id, a, err)
		}
		return a.State
	}
	if got("a") != Idle {
		t.Errorf("a: want idle, got %q", got("a"))
	}
	if got("b") != Working {
		t.Errorf("b: want working, got %q", got("b"))
	}
}

// writeAgentAt writes an agent file with an explicit mtime so a change is
// visible to the compare-and-set regardless of filesystem timestamp resolution.
func writeAgentAt(t *testing.T, home, id string, state AgentState, at time.Time) {
	t.Helper()
	p := agentPath(home, id)
	body := `{"session_id":"` + id + `","state":"` + string(state) + `"}`
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, at, at); err != nil {
		t.Fatal(err)
	}
}

func TestMarkSeen_ConcurrentReportWins(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(AgentsDir(home), 0o700); err != nil {
		t.Fatal(err)
	}
	writeAgentAt(t, home, "a", Done, time.Unix(1000, 0))
	calls := 0
	markSeenHook = func() {
		calls++
		writeAgentAt(t, home, "a", Working, time.Unix(2000, 0)) // a hook report lands mid-flight
	}
	t.Cleanup(func() { markSeenHook = func() {} })
	if err := MarkSeen(home, "a"); err != nil {
		t.Fatalf("MarkSeen: %v", err)
	}
	a, err := readAgent(home, "a")
	if err != nil || a == nil || a.State != Working {
		t.Fatalf("state %+v %v, want working kept", a, err)
	}
	if calls != 1 {
		t.Errorf("hook calls %d, want 1 (reread finds working and stops)", calls)
	}
}

func TestMarkSeen_GivesUpAfterSecondChange(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(AgentsDir(home), 0o700); err != nil {
		t.Fatal(err)
	}
	writeAgentAt(t, home, "a", Done, time.Unix(1000, 0))
	calls := 0
	markSeenHook = func() {
		calls++
		writeAgentAt(t, home, "a", Done, time.Unix(int64(2000+calls), 0))
	}
	t.Cleanup(func() { markSeenHook = func() {} })
	if err := MarkSeen(home, "a"); err != nil {
		t.Fatalf("MarkSeen: %v", err)
	}
	a, err := readAgent(home, "a")
	if err != nil || a == nil || a.State != Done {
		t.Fatalf("state %+v %v, want the newer done kept", a, err)
	}
	if calls != 2 {
		t.Errorf("hook calls %d, want 2 (one retry)", calls)
	}
}

func TestMarkSeen_SurfacesTempRemoveError(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(AgentsDir(home), 0o700); err != nil {
		t.Fatal(err)
	}
	writeAgentAt(t, home, "a", Done, time.Unix(1000, 0))
	markSeenHook = func() { writeAgentAt(t, home, "a", Done, time.Unix(2000, 0)) }
	removeTemp = func(string) error { return errors.New("boom") }
	t.Cleanup(func() { markSeenHook = func() {}; removeTemp = os.Remove })
	err := MarkSeen(home, "a")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("MarkSeen = %v, want the remove error surfaced", err)
	}
}
