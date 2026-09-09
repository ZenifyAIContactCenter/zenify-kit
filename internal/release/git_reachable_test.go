package release

import (
	"strings"
	"testing"
)

type fakeReachRunner struct {
	out  string
	args []string
}

func (f *fakeReachRunner) Run(dir string, args ...string) ([]byte, error) {
	f.args = args
	return []byte(f.out), nil
}

func TestReachableCommits(t *testing.T) {
	// One feat commit with a Spec: trailer in its body. Fields: %h<us>%s<us>%an<us>%b then recSep.
	rec := "abc123" + sep + "feat(fields): add x" + sep + "Nam" + sep +
		"body line\nSpec: specs/zenify-kit/2026-09-09-fields-design.md" + recSep
	f := &fakeReachRunner{out: rec}
	cs, err := ReachableCommits(f, "/repo", "origin/main")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(cs) != 1 {
		t.Fatalf("len = %d, want 1", len(cs))
	}
	if cs[0].SHA != "abc123" || cs[0].Type != "feat" {
		t.Fatalf("commit = %+v", cs[0])
	}
	if !strings.Contains(cs[0].Body, "Spec:") {
		t.Fatalf("body lost trailer: %q", cs[0].Body)
	}
	if f.args[0] != "log" || f.args[len(f.args)-1] != "origin/main" {
		t.Fatalf("git args = %v, want log ... origin/main", f.args)
	}
}
