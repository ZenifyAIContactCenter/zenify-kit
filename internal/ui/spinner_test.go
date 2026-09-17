package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"
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

// TestSpinner_StyledPath_FailWithoutStart_NoPanic exercises styled==true without a
// real TTY by constructing the struct directly (package-internal test). It covers
// the CRITICAL bug: calling Fail/Stop before Start must not panic on a nil channel,
// and the erase sequence must still precede the final line.
func TestSpinner_StyledPath_FailWithoutStart_NoPanic(t *testing.T) {
	var buf bytes.Buffer
	s := &Spinner{w: &buf, label: "x", styled: true}

	s.Fail("boom") // no prior Start() — must not panic

	out := buf.String()
	eraseIdx := strings.Index(out, "\r\033[K")
	if eraseIdx == -1 {
		t.Fatalf("expected erase sequence in output: %q", out)
	}
	finalIdx := strings.Index(out, "✗ boom")
	if finalIdx == -1 {
		t.Fatalf("expected final Fail line in output: %q", out)
	}
	if finalIdx < eraseIdx {
		t.Fatalf("expected erase sequence before final line: %q", out)
	}

	// A second Stop() with nothing running must also be a safe no-op.
	s2 := &Spinner{w: &buf, label: "y", styled: true}
	s2.Stop()
}

// TestSpinner_DoubleStartStop_NoPanicFramesStop covers the MEDIUM leak: a second
// Start() while already running must not spawn a second goroutine or replace the
// live channels, and a single Stop() must cleanly end the run with no further
// frames written afterward.
func TestSpinner_DoubleStartStop_NoPanicFramesStop(t *testing.T) {
	var buf bytes.Buffer
	s := &Spinner{w: &buf, label: "y", styled: true}

	s.Start()
	s.Start() // must no-op: already running, must not leak a second goroutine

	time.Sleep(150 * time.Millisecond) // allow at least one 90ms tick

	s.Stop() // blocks until the goroutine has actually exited
	n := buf.Len()
	if n == 0 {
		t.Fatalf("expected at least one frame to have been written before Stop")
	}

	// Safe to read buf now: Stop() only returns after <-done, which happens after
	// the goroutine's last write, so there is no concurrent access here.
	time.Sleep(150 * time.Millisecond)
	if buf.Len() != n {
		t.Fatalf("frames continued writing after Stop: before=%d after=%d", n, buf.Len())
	}

	s.Stop() // idempotent: must not panic when called again after already stopped
}
