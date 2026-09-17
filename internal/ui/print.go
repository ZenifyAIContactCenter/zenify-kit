package ui

import (
	"fmt"
)

// Header prints the command banner: "▲ <title>" in bold, then a blank line.
func (u *Writer) Header(title string) {
	p := u.styles()
	fmt.Fprintln(u.w, p.header.Render("▲ "+title))
	fmt.Fprintln(u.w)
}

// Step prints one status line: "  <symbol> <label>   <detail-dim>".
func (u *Writer) Step(s Status, label, detail string) {
	p := u.styles()
	mark := s.style(p).Render(s.symbol())
	line := "  " + mark + " " + label
	if detail != "" {
		line += "   " + p.dim.Render(detail)
	}
	fmt.Fprintln(u.w, line)
}

// Note prints faint helper text, indented under the preceding action.
func (u *Writer) Note(text string) {
	fmt.Fprintln(u.w, "    "+u.styles().dim.Render(text))
}

// Section prints a blank line then a bold section title.
func (u *Writer) Section(title string) {
	fmt.Fprintln(u.w)
	fmt.Fprintln(u.w, u.styles().bold.Render(title))
}

// Blank prints one empty line (group spacing).
func (u *Writer) Blank() { fmt.Fprintln(u.w) }

// KV prints aligned key/value rows, keys dim.
func (u *Writer) KV(rows [][2]string) {
	p := u.styles()
	w := 0
	for _, r := range rows {
		if len(r[0]) > w {
			w = len(r[0])
		}
	}
	for _, r := range rows {
		fmt.Fprintf(u.w, "  %s   %s\n", p.dim.Render(fmt.Sprintf("%-*s", w, r[0])), r[1])
	}
}
