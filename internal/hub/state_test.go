package hub

import (
	"os"
	"path/filepath"
	"testing"
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
