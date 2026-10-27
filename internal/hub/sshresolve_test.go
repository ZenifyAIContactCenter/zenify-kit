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
