package dbperf

import (
	"regexp"
	"strconv"
	"strings"
)

// AddedLine is one added ('+') content line in a unified diff.
type AddedLine struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

var (
	diffFileRe = regexp.MustCompile(`^\+\+\+ b/(.+)$`)
	hunkRe     = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@`)
)

// AddedLines parses `git diff` unified output and returns only added content
// lines, tagged with the new-file path and new-file 1-indexed line number.
func AddedLines(diff string) []AddedLine {
	var out []AddedLine
	var file string
	var newLine int
	for _, l := range strings.Split(diff, "\n") {
		if m := diffFileRe.FindStringSubmatch(l); m != nil {
			file = m[1]
			continue
		}
		if m := hunkRe.FindStringSubmatch(l); m != nil {
			newLine, _ = strconv.Atoi(m[1])
			continue
		}
		switch {
		case strings.HasPrefix(l, "+++") || strings.HasPrefix(l, "---"):
			continue
		case strings.HasPrefix(l, "+"):
			out = append(out, AddedLine{File: file, Line: newLine, Text: l[1:]})
			newLine++
		case strings.HasPrefix(l, "-"):
			// removed line: does not advance the new-file counter
		default:
			newLine++ // context line
		}
	}
	return out
}
