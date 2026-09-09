// Package review — the doctrine part (doctrine seam of znf:review, M4d).
// SanitizeVerified strips verdict-ONLY lines from a ship-pack's ## Verified block,
// keeping lines that carry facts, so the reviewer isn't anchored to the author's own conclusion.
// Mechanical, fail-open; does NOT use an LLM.
package review

import "strings"

// verdictPhrases: signals that a line is a CLAIM the code is correct (matched lowercase).
// NO bare "correct" — too broad (collides with "correctness"); use unambiguous phrases instead.
var verdictPhrases = []string{
	"✅", "verified", "looks good", "lgtm", "no issues", "no problem",
	"all correct", "works correctly", "passes review", "ready to ship",
	"shippable", "all good",
}

// gapPhrases: a line ABOUT a missing test — the most valuable kind of line, never stripped.
var gapPhrases = []string{
	"no test", "not covered", "untested", "no coverage",
	"chưa có test", "không có test", "không test", //znf:allow-lang
}

// hasFactToken: the line carries concrete evidence (number / path / command) → keep it even with a verdict-phrase.
func hasFactToken(line string) bool {
	for _, r := range line {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	if strings.Contains(line, "/") {
		return true
	}
	for _, ext := range []string{".go", ".js", ".ts", ".py", ".md"} {
		if strings.Contains(line, ext) {
			return true
		}
	}
	trimmed := strings.TrimSpace(line)
	for _, cmd := range []string{"$ ", "pm ", "go ", "git ", "npm ", "yarn ", "bash "} {
		if strings.HasPrefix(trimmed, cmd) {
			return true
		}
	}
	return false
}

func containsAny(lower string, phrases []string) bool {
	for _, p := range phrases {
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// SanitizeVerified strips verdict-only lines. clean keeps the original order + blank lines;
// stripped is the removed lines (trimmed). Fail-open: empty text → "", nil.
// A line is stripped when: it has a verdict-phrase, has NO fact-token, and is NOT a gap line.
func SanitizeVerified(text string) (clean string, stripped []string) {
	if text == "" {
		return "", nil
	}
	lines := strings.Split(text, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		lower := strings.ToLower(line)
		if containsAny(lower, verdictPhrases) && !hasFactToken(line) && !containsAny(lower, gapPhrases) {
			stripped = append(stripped, strings.TrimSpace(line))
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n"), stripped
}
