//go:build unix

package cli

import (
	"path/filepath"
	"syscall"
	"testing"
)

func TestReadPlanTitleFIFODoesNotBlock(t *testing.T) {
	p := filepath.Join(t.TempDir(), "p.md")
	if err := syscall.Mkfifo(p, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := readPlanTitle("", p); err != nil || got != "" {
		t.Fatalf("got %q err %v", got, err)
	}
}
