package ui

import (
	"fmt"
	"io"
	"sync"
	"time"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner animates a single status line on w (stderr) while a long op runs. On a
// non-TTY (piped, NO_COLOR, --no-color) it is inert: Start/Stop write nothing and
// only Success/Fail print one static line, so stdout is never touched.
//
// Lifecycle: Start/Stop (or Start/Success, Start/Fail) may be called any number of
// times in sequence — a second Start while already running is a safe no-op, and
// Stop/Success/Fail are all safe to call with no Start ever having run (e.g. an
// early-return error path before Start was reached).
type Spinner struct {
	w      io.Writer
	label  string
	styled bool

	mu      sync.Mutex
	stop    chan struct{}
	done    chan struct{}
	running bool
}

// NewSpinner builds a spinner targeting w (pass os.Stderr in production).
func NewSpinner(w io.Writer, label string) *Spinner {
	return &Spinner{w: w, label: label, styled: decideStyled(w)}
}

// Start begins animation (no-op when not styled, or when already running).
func (s *Spinner) Start() {
	if !s.styled {
		return
	}
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	s.stop = stopCh
	s.done = doneCh
	s.running = true
	s.mu.Unlock()

	go func() {
		defer close(doneCh)
		t := time.NewTicker(90 * time.Millisecond)
		defer t.Stop()
		i := 0
		for {
			select {
			case <-stopCh:
				return
			case <-t.C:
				fmt.Fprintf(s.w, "\r  %s %s", spinnerFrames[i%len(spinnerFrames)], s.label)
				i++
			}
		}
	}()
}

// clear stops the goroutine (if one is running) and erases the spinner line.
// Safe to call with no prior Start (the erase escape is a harmless no-op with
// nothing to erase) and safe to call repeatedly (idempotent: a second call finds
// running==false and just re-emits the erase, without touching closed channels).
func (s *Spinner) clear() {
	if !s.styled {
		return
	}
	s.mu.Lock()
	running := s.running
	stopCh := s.stop
	doneCh := s.done
	s.running = false
	s.mu.Unlock()

	if running {
		close(stopCh)
		<-doneCh
	}
	fmt.Fprint(s.w, "\r\033[K") // carriage return + clear-to-end-of-line
}

// Stop erases the spinner without a final line.
func (s *Spinner) Stop() { s.clear() }

// Success erases the spinner and prints a ✓ line.
func (s *Spinner) Success(msg string) {
	s.clear()
	fmt.Fprintf(s.w, "  ✓ %s\n", msg)
}

// Fail erases the spinner and prints a ✗ line.
func (s *Spinner) Fail(msg string) {
	s.clear()
	fmt.Fprintf(s.w, "  ✗ %s\n", msg)
}
