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
type Spinner struct {
	w      io.Writer
	label  string
	styled bool
	stop   chan struct{}
	done   chan struct{}
	once   sync.Once
}

// NewSpinner builds a spinner targeting w (pass os.Stderr in production).
func NewSpinner(w io.Writer, label string) *Spinner {
	return &Spinner{w: w, label: label, styled: decideStyled(w)}
}

// Start begins animation (no-op when not styled).
func (s *Spinner) Start() {
	if !s.styled {
		return
	}
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		t := time.NewTicker(90 * time.Millisecond)
		defer t.Stop()
		i := 0
		for {
			select {
			case <-s.stop:
				return
			case <-t.C:
				fmt.Fprintf(s.w, "\r  %s %s", spinnerFrames[i%len(spinnerFrames)], s.label)
				i++
			}
		}
	}()
}

// clear stops the goroutine and erases the spinner line.
func (s *Spinner) clear() {
	if !s.styled {
		return
	}
	s.once.Do(func() {
		close(s.stop)
		<-s.done
		fmt.Fprint(s.w, "\r\033[K") // carriage return + clear-to-end-of-line
	})
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
