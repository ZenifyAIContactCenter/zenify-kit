// Package ui is the central human-facing renderer for the zenify CLI. It emits
// styled output (color, symbols, spacing) only to a real TTY; when the writer is
// not a terminal, NO_COLOR is set, or --no-color was passed, it degrades to plain
// text with zero ANSI escapes. Machine-readable output (JSON envelopes, --json
// branches, hook contracts) must NOT go through this package.
package ui

import (
	"io"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"golang.org/x/term"
)

// forceNoColor is set by the root --no-color flag (via SetNoColor) and forces
// every subsequently-created Writer to plain mode.
var forceNoColor bool

// SetNoColor toggles the global no-color override (root persistent --no-color).
func SetNoColor(v bool) { forceNoColor = v }

// Writer renders human-facing output to w. Construct with New.
type Writer struct {
	w      io.Writer
	styled bool
	r      *lipgloss.Renderer
}

// New builds a Writer bound to w, deciding styled-vs-plain from the environment.
func New(w io.Writer) *Writer {
	styled := decideStyled(w)
	var r *lipgloss.Renderer
	if styled {
		r = lipgloss.NewRenderer(w)
	} else {
		// Ascii profile makes every Style.Render a no-op: no color, no bold, no
		// escapes — exactly what a pipe or NO_COLOR consumer needs.
		r = lipgloss.NewRenderer(w, termenv.WithProfile(termenv.Ascii))
	}
	return &Writer{w: w, styled: styled, r: r}
}

// decideStyled is true only for a real terminal with color permitted.
func decideStyled(w io.Writer) bool {
	if forceNoColor || os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// Out returns the underlying writer (for callers that still print raw).
func (u *Writer) Out() io.Writer { return u.w }

// Styled reports whether color/style is active.
func (u *Writer) Styled() bool { return u.styled }
