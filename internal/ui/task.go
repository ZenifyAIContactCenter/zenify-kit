package ui

// Task runs `run` as one step of the flow. On a styled TTY it shows a braille
// spinner on a gutter line while `run` executes, then clears it and prints the
// resolved status line. When not styled it does not animate: it runs `run` and
// prints the result line directly, so piped/NO_COLOR output stays byte-clean.
func (f *Flow) Task(label string, run func() (Status, string)) {
	if !f.u.styled {
		status, detail := run()
		f.Line(status, label, detail)
		return
	}
	// Styled path: animate a spinner on the gutter, then replace with the result.
	// NewSpinner(w io.Writer, label string) *Spinner — pass the Writer's underlying
	// sink f.u.w (unexported, same package), NOT f.u (a *Writer). Grounded: spinner.go:32.
	sp := NewSpinner(f.u.w, f.gutter()+"  "+label)
	sp.Start()
	status, detail := run()
	sp.Stop()
	f.Line(status, label, detail)
}
