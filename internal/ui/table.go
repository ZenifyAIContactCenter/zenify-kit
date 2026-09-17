package ui

import (
	"fmt"
	"strings"
)

// Table prints headers (bold) then rows, columns left-aligned to the widest cell.
func (u *Writer) Table(headers []string, rows [][]string) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, c := range row {
			if i < len(widths) && len(c) > widths[i] {
				widths[i] = len(c)
			}
		}
	}
	p := u.styles()
	var b strings.Builder
	for i, h := range headers {
		fmt.Fprintf(&b, "%s  ", p.bold.Render(fmt.Sprintf("%-*s", widths[i], h)))
	}
	fmt.Fprintln(u.w, strings.TrimRight(b.String(), " "))
	for _, row := range rows {
		b.Reset()
		for i, c := range row {
			if i < len(widths) {
				fmt.Fprintf(&b, "%-*s  ", widths[i], c)
			}
		}
		fmt.Fprintln(u.w, strings.TrimRight(b.String(), " "))
	}
}
