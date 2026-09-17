package ui

import "github.com/charmbracelet/lipgloss"

// Status selects a Step's leading symbol and color.
type Status int

const (
	StatusOK Status = iota
	StatusFail
	StatusActive
	StatusWarn
	StatusInfo
)

// symbol returns the plain glyph for a status (kept even in plain mode).
func (s Status) symbol() string {
	switch s {
	case StatusOK:
		return "✓"
	case StatusFail:
		return "✗"
	case StatusActive:
		return "›"
	case StatusWarn:
		return "⚠"
	default:
		return "•"
	}
}

// palette holds the lipgloss styles for one Writer's renderer. Vercel-restrained:
// color only on status marks + the header symbol; body text stays default; helper
// text is faint. In Ascii profile these Render as plain (no escapes).
type palette struct {
	header lipgloss.Style
	ok     lipgloss.Style
	fail   lipgloss.Style
	active lipgloss.Style
	warn   lipgloss.Style
	dim    lipgloss.Style
	bold   lipgloss.Style
}

func (u *Writer) styles() palette {
	s := u.r.NewStyle
	return palette{
		header: s().Bold(true),
		ok:     s().Foreground(lipgloss.Color("2")), // green
		fail:   s().Foreground(lipgloss.Color("1")), // red
		active: s().Foreground(lipgloss.Color("6")), // cyan
		warn:   s().Foreground(lipgloss.Color("3")), // yellow
		dim:    s().Faint(true),
		bold:   s().Bold(true),
	}
}

func (s Status) style(p palette) lipgloss.Style {
	switch s {
	case StatusOK:
		return p.ok
	case StatusFail:
		return p.fail
	case StatusActive:
		return p.active
	case StatusWarn:
		return p.warn
	default:
		return p.dim
	}
}
