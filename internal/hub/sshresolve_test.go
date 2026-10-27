package hub

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestWrapSSHErrIncludesStderr(t *testing.T) {
	ee := &exec.ExitError{Stderr: []byte("  ssh: Could not resolve hostname x\n")}
	err := wrapSSHErr(ee)
	if !strings.Contains(err.Error(), "ssh: Could not resolve hostname x") {
		t.Fatalf("stderr missing: %v", err)
	}
	if !errors.Is(err, ee) {
		t.Fatalf("original error not wrapped: %v", err)
	}
	plain := errors.New("boom")
	if wrapSSHErr(plain) != plain {
		t.Fatalf("non-exit error must pass through")
	}
	if wrapSSHErr(nil) != nil {
		t.Fatalf("nil must stay nil")
	}
}

type argSSH struct{ calls [][]string }

func (f *argSSH) Run(args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	return []byte("hostname github.com\n"), nil
}

func TestResolveUsesURLUser(t *testing.T) {
	f := &argSSH{}
	res := NewHostResolver(f)
	if _, err := normalizeOrigin("git@github-work:acme/tools.git", res); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.calls[0], " "); got != "-G -l git github-work" {
		t.Errorf("args = %q", got)
	}
	if _, err := normalizeOrigin("ssh://bob@gh2/acme/tools.git", res); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.calls[1], " "); got != "-G -l bob gh2" {
		t.Errorf("args = %q", got)
	}
	if _, err := normalizeOrigin("gh:acme/tools", res); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.calls[2], " "); got != "-G gh" {
		t.Errorf("args = %q", got)
	}
}

func TestResolveCachesPerUserHost(t *testing.T) {
	f := &argSSH{}
	res := NewHostResolver(f)
	for _, u := range []string{"git@h:a/b", "git@h:a/b", "bob@h:a/b", "bob@h:a/b"} {
		if _, err := normalizeOrigin(u, res); err != nil {
			t.Fatal(err)
		}
	}
	if len(f.calls) != 2 {
		t.Errorf("calls = %v, want 2 (git@h, bob@h)", f.calls)
	}
}
