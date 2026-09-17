package ui

import "fmt"

// Marker selects a group's leading glyph in a Flow.
type Marker int

const (
	MarkerActive Marker = iota // ◇ — a step in progress
	MarkerDone                 // ◆ — a completed step
)

func (m Marker) glyph() string {
	if m == MarkerDone {
		return "◆"
	}
	return "◇"
}

// Flow renders a clack-style bordered sequence bound to one Writer: an opening
// cap "┌ <title>", a vertical gutter "│" down the left of every body line, group
// markers ◇/◆, and a closing cap "└ <summary>". In plain mode the glyphs are kept
// (they are content, like ✓) but no ANSI is emitted. Construct with Writer.Flow.
type Flow struct {
	u *Writer
}

// Flow opens a clack flow: prints the "┌ <title>" cap and returns the handle.
func (u *Writer) Flow(title string) *Flow {
	p := u.styles()
	fmt.Fprintln(u.w, p.dim.Render("┌")+"  "+p.bold.Render(title))
	fmt.Fprintln(u.w, p.dim.Render("│"))
	return &Flow{u: u}
}

// gutter renders the dim "│" rail prefix.
func (f *Flow) gutter() string { return f.u.styles().dim.Render("│") }

// Group prints "<marker>  <title>" at the gutter position (marker replaces the rail).
func (f *Flow) Group(m Marker, title string) {
	p := f.u.styles()
	fmt.Fprintln(f.u.w, p.bold.Render(m.glyph())+"  "+p.bold.Render(title))
}

// Line prints a body line "│  <symbol> <label>   <detail-dim>".
func (f *Flow) Line(s Status, label, detail string) {
	p := f.u.styles()
	mark := s.style(p).Render(s.symbol())
	line := f.gutter() + "  " + mark + " " + label
	if detail != "" {
		line += "   " + p.dim.Render(detail)
	}
	fmt.Fprintln(f.u.w, line)
}

// Note prints a dim body line under the gutter.
func (f *Flow) Note(text string) {
	fmt.Fprintln(f.u.w, f.gutter()+"  "+f.u.styles().dim.Render(text))
}

// Blank prints a bare gutter line (group spacing).
func (f *Flow) Blank() { fmt.Fprintln(f.u.w, f.gutter()) }

// Close prints the "└ <summary>" cap that ends the flow.
func (f *Flow) Close(summary string) {
	p := f.u.styles()
	fmt.Fprintln(f.u.w, p.dim.Render("│"))
	fmt.Fprintln(f.u.w, p.dim.Render("└")+"  "+p.bold.Render(summary))
}
