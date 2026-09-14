package docsgen

import "strings"

// escapeAngleReplacer backslash-escapes literal "<"/">" so markdown-it does
// not parse them as a bare HTML tag (see escapeAngle for why this matters).
var escapeAngleReplacer = strings.NewReplacer("<", "\\<", ">", "\\>")

// escapeCellReplacer additionally escapes "|", which would otherwise be read
// as a markdown table column separator.
var escapeCellReplacer = strings.NewReplacer("<", "\\<", ">", "\\>", "|", "\\|")

// escapeAngle backslash-escapes literal "<"/">" in page-body text (e.g.
// "R<N>.md", "--port <key>") so markdown-it does not parse them as a bare
// HTML tag — Vue's compiler then fails the whole build on an unclosed
// element. CommonMark treats "\<" as a literal "<", so this only changes how
// the character round-trips through markdown, not what the reader sees.
func escapeAngle(s string) string {
	return escapeAngleReplacer.Replace(s)
}

// escapeCell is escapeAngle plus "|" escaping, for text that lands inside a
// markdown table cell (index rows, hook rows) where an unescaped "|" would
// split the column.
func escapeCell(s string) string {
	return escapeCellReplacer.Replace(s)
}
