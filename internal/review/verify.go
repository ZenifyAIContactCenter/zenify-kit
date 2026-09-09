// Package review provides a mechanical finding-verifier for the znf:review engine (VERIFY seam).
// It does NOT judge whether a finding is a real bug (that's the LLM reviewer's job);
// it only verifies the CITATION: whether the finding's line+quote match the real file.
package review

import (
	"strconv"
	"strings"
)

// window is the number of lines of drift allowed on either side of `line` when looking for
// evidence (lines shift after edits).
const window = 3

// Finding follows _shared/finding-schema.md. Every field is omitempty so round-tripping doesn't bloat it.
type Finding struct {
	Dimension string `json:"dimension,omitempty"`
	Severity  string `json:"severity,omitempty"`
	Title     string `json:"title,omitempty"`
	File      string `json:"file,omitempty"`
	Line      string `json:"line,omitempty"`
	Issue     string `json:"issue,omitempty"`
	Fix       string `json:"fix,omitempty"`
	Evidence  string `json:"evidence,omitempty"`
	Refuted   bool   `json:"refuted,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// Result: findings contains ONLY kept findings; refuted ones are removed and counted separately.
type Result struct {
	Findings []Finding `json:"findings"`
	Kept     int       `json:"kept"`
	Refuted  int       `json:"refuted"`
}

// Verify checks each finding. readFile is injected for testing (the CLI passes os.ReadFile).
func Verify(findings []Finding, readFile func(string) ([]byte, error)) Result {
	res := Result{Findings: []Finding{}}
	for _, f := range findings {
		if keep, out := verifyOne(f, readFile); keep {
			res.Findings = append(res.Findings, out)
		} else {
			res.Refuted++
		}
	}
	res.Kept = len(res.Findings)
	return res
}

func verifyOne(f Finding, readFile func(string) ([]byte, error)) (bool, Finding) {
	// Cannot be located → cannot verify mechanically, leave as-is.
	if f.File == "" || f.Line == "" {
		return true, f
	}
	n, ok := parseLine(f.Line)
	if !ok {
		f.Reason = "refuted: line did not parse: " + f.Line
		return false, f
	}
	if f.Evidence == "" {
		f.Reason = "unverified: no evidence"
		return true, f
	}
	data, err := readFile(f.File)
	if err != nil {
		f.Reason = "refuted: could not read file: " + f.File
		return false, f
	}
	lines := strings.Split(string(data), "\n")
	target := normalize(stripDiffMarker(f.Evidence))
	lo := n - window
	if lo < 1 {
		lo = 1
	}
	hi := n + window
	if hi > len(lines) {
		hi = len(lines)
	}
	for i := lo; i <= hi; i++ {
		if strings.Contains(normalize(lines[i-1]), target) {
			return true, f
		}
	}
	f.Reason = "refuted: evidence not found around line " + f.Line
	return false, f
}

// normalize collapses every run of whitespace into one space and trims, so matching doesn't depend on indent.
func normalize(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// stripDiffMarker strips ONE diff-mark character (+ or -) from the start of evidence, in case
// the reviewer quoted a raw line from a unified diff (column 0 of a diff line is +/-/space). The
// real file doesn't carry this mark, so it's stripped before matching; a broader strip only makes
// Contains more lenient, it never causes a false refute.
func stripDiffMarker(s string) string {
	if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		return s[1:]
	}
	return s
}

// parseLine takes the first run of digits from "N" or "N-M". false if there are no digits.
func parseLine(s string) (int, bool) {
	start := -1
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			if start < 0 {
				start = i
			}
		} else if start >= 0 {
			n, _ := strconv.Atoi(s[start:i])
			return n, true
		}
	}
	if start >= 0 {
		n, _ := strconv.Atoi(s[start:])
		return n, true
	}
	return 0, false
}
