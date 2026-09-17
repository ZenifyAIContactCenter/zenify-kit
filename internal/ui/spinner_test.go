package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestSpinner_NonTTY_NoStdoutFrames(t *testing.T) {
	SetNoColor(false)
	var errBuf bytes.Buffer // non-TTY
	sp := NewSpinner(&errBuf, "Cloning")
	sp.Start()
	sp.Success("done")
	out := errBuf.String()
	if strings.Contains(out, "\r") {
		t.Fatalf("non-TTY spinner must not emit \\r frames: %q", out)
	}
	if !strings.Contains(out, "done") {
		t.Fatalf("Success line missing: %q", out)
	}
}

func TestSpinner_NonTTY_NeverPanicsOnStop(t *testing.T) {
	var errBuf bytes.Buffer
	sp := NewSpinner(&errBuf, "x")
	sp.Start()
	sp.Stop() // must be safe with no Success/Fail
}
