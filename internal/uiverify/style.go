package uiverify

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

// styleWaiverMarker waives style violations on its own added line and on the
// added line right after it. The P0 marker (waiverMarker) does not.
const styleWaiverMarker = "znf:ui-style-ok:"

// Violation is one hardcoded-style match on an added line.
type Violation struct {
	File  string
	Line  int
	Rule  string
	Match string
}

// Go's RE2 has no lookahead, so each rule captures what it needs and the
// trailing boundary is checked in code (see classMatches / hexMatches).
var (
	hexRe     = regexp.MustCompile(`#[0-9a-fA-F]+[A-Za-z0-9_-]?`)
	paletteRe = regexp.MustCompile(`(?:^|[^A-Za-z0-9_-])((?:bg|text|border|ring|fill|stroke|from|via|to|outline|divide|decoration|shadow|accent|caret|placeholder)-` +
		`(?:slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)-` +
		`(?:950|900|800|700|600|500|400|300|200|100|50)(?:/[0-9]+)?)`)
	arbitraryRe = regexp.MustCompile(`(?:^|[^A-Za-z0-9_-])(-?[a-z][a-z0-9-]*-\[[0-9]+(?:\.[0-9]+)?(?:px|rem|em)\])`)
)

func isWordByte(b byte) bool {
	return b == '-' || b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

// classMatches returns group 1 of every re match that is not glued to a
// following identifier character ("bg-red-5000" is not "bg-red-500").
func classMatches(re *regexp.Regexp, line string) []string {
	var out []string
	for _, m := range re.FindAllStringSubmatchIndex(line, -1) {
		if end := m[3]; end < len(line) && isWordByte(line[end]) {
			continue
		}
		out = append(out, line[m[2]:m[3]])
	}
	return out
}

func hexMatches(line string) []string {
	var out []string
	for _, m := range hexRe.FindAllString(line, -1) {
		body := m[1:]
		if strings.IndexByte("0123456789abcdefABCDEF", body[len(body)-1]) < 0 {
			continue // an identifier character followed the hex run
		}
		if n := len(body); n == 3 || n == 4 || n == 6 || n == 8 {
			out = append(out, m)
		}
	}
	return out
}

// matchRules applies rules (in the given order) to one line; File and Line
// are the caller's to fill.
func matchRules(line string, rules []string) []Violation {
	var out []Violation
	for _, r := range rules {
		var ms []string
		switch r {
		case StyleHex:
			ms = hexMatches(line)
		case StylePalette:
			ms = classMatches(paletteRe, line)
		case StyleArbitrary:
			ms = classMatches(arbitraryRe, line)
		}
		for _, m := range ms {
			out = append(out, Violation{Rule: r, Match: m})
		}
	}
	return out
}

func isStyleFile(p string) bool {
	for _, ext := range []string{".tsx", ".jsx", ".vue", ".svelte"} {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}

func hasStyleWaiver(line string) bool {
	idx := strings.Index(line, styleWaiverMarker)
	return idx >= 0 && strings.TrimSpace(line[idx+len(styleWaiverMarker):]) != ""
}

// hunkNewStart parses the +c of "@@ -a,b +c,d @@".
func hunkNewStart(header string) int {
	for _, f := range strings.Fields(header) {
		if strings.HasPrefix(f, "+") {
			n, _, _ := strings.Cut(f[1:], ",")
			if v, err := strconv.Atoi(n); err == nil {
				return v
			}
		}
	}
	return 0
}

// scanAdded checks the added lines of a unified diff (as produced by
// branchDiff) in files that render and carry markup. Line numbers are those
// of the new file; checked counts the added lines examined.
func scanAdded(diff string, files, rules []string) (violations []Violation, checked int) {
	inFiles := make(map[string]bool, len(files))
	for _, f := range files {
		if isStyleFile(f) {
			inFiles[f] = true
		}
	}
	cur, inHunk, newLine, waivedNext := "", false, 0, false
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			cur, inHunk = "", false
		case !inHunk && strings.HasPrefix(line, "+++ "):
			cur = strings.TrimPrefix(strings.TrimRight(line[len("+++ "):], "\t"), "b/")
		case strings.HasPrefix(line, "@@"):
			inHunk, newLine, waivedNext = true, hunkNewStart(line), false
		case !inHunk || line == "" || line[0] == '\\':
		case line[0] == '+':
			text := line[1:]
			if inFiles[cur] {
				checked++
				waived := waivedNext || hasStyleWaiver(text)
				if !waived {
					for _, v := range matchRules(text, rules) {
						v.File, v.Line = cur, newLine
						violations = append(violations, v)
					}
				}
				waivedNext = hasStyleWaiver(text)
			}
			newLine++
		case line[0] == ' ':
			waivedNext = false
			newLine++
		default: // '-' line: not in the new file
			waivedNext = false
		}
	}
	return violations, checked
}

// Style lints the lines this branch added in rendering files against the
// rules declared in uiVerify.style. declared=false (no rules) returns before
// any git call, so repos that never opted in see no behaviour change.
func Style(d Deps, repo, base string) (violations []Violation, checked int, declared bool, err error) {
	cfg, ok, err := LoadConfig(repo)
	if err != nil || !ok || len(cfg.Style) == 0 {
		return nil, 0, false, err
	}
	files, err := RenderSet(d, repo, base)
	if err != nil {
		return nil, 0, true, err
	}
	if len(files) == 0 {
		return nil, 0, true, nil
	}
	diff, err := branchDiff(d, repo, base)
	if err != nil {
		return nil, 0, true, exitcode.New(exitcode.Fail, fmt.Errorf("ui-verify style: %w", err))
	}
	extra, err := untrackedDiff(d, repo, files)
	if err != nil {
		return nil, 0, true, exitcode.New(exitcode.Fail, fmt.Errorf("ui-verify style: %w", err))
	}
	violations, checked = scanAdded(diff+extra, files, cfg.Style)
	return violations, checked, true, nil
}

// newFileDiff renders body as the diff of a brand-new file, so scanAdded
// treats every line as added with the same rules and waivers.
func newFileDiff(path, body string) string {
	body = strings.TrimSuffix(body, "\n")
	if body == "" {
		return ""
	}
	lines := strings.Split(body, "\n")
	return fmt.Sprintf("diff --git a/%[1]s b/%[1]s\n+++ b/%[1]s\n@@ -0,0 +1,%d @@\n+%s\n",
		path, len(lines), strings.Join(lines, "\n+"))
}

// untrackedDiff covers what `git diff <merge-base>` never shows: untracked
// files in the render set. An unreadable file is an error, never "clean".
func untrackedDiff(d Deps, repo string, files []string) (string, error) {
	status, err := d.RunGit(repo, "-c", "core.quotePath=false", "status", "--porcelain", "-uall")
	if err != nil {
		return "", fmt.Errorf("git status --porcelain -uall: %w", err)
	}
	inSet := make(map[string]bool, len(files))
	for _, f := range files {
		if isStyleFile(f) {
			inSet[f] = true
		}
	}
	var sb strings.Builder
	for _, line := range strings.Split(status, "\n") {
		line = strings.TrimRight(line, "\r")
		if !strings.HasPrefix(line, "?? ") {
			continue
		}
		path := parsePorcelainPath(line)
		if !inSet[path] {
			continue
		}
		body, err := os.ReadFile(filepath.Join(repo, path)) //nolint:gosec // G304 -- path comes from git status of this repo
		if err != nil {
			return "", fmt.Errorf("read untracked %s: %w", path, err)
		}
		sb.WriteString(newFileDiff(path, string(body)))
	}
	return sb.String(), nil
}

const maxStyleLines = 10

// styleMessage is Check's failure text: the first maxStyleLines violations
// and the total.
func styleMessage(vs []Violation) string {
	parts := make([]string, 0, maxStyleLines)
	for i, v := range vs {
		if i == maxStyleLines {
			break
		}
		parts = append(parts, fmt.Sprintf("%s:%d %s %s", v.File, v.Line, v.Rule, v.Match))
	}
	more := ""
	if len(vs) > maxStyleLines {
		more = fmt.Sprintf("; +%d more", len(vs)-maxStyleLines)
	}
	return fmt.Sprintf("ui-verify style — %d violation(s): %s%s", len(vs), strings.Join(parts, "; "), more)
}
