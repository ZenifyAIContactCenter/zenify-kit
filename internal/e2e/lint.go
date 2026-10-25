package e2e

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

type Finding struct {
	File string
	Line int
	Rule string
	Msg  string
}

var (
	// Match test.only/.skip/.fixme too — otherwise a `test.only(...)` scenario would
	// NOT match, splitTests returns empty and the whole gate skips it (bypasses the rule entirely).
	// One alternative per quote kind, so an apostrophe inside a "..." title does not end it.
	// Escape-aware, so `'it\'s @ac:1'` keeps its tag; jsUnescape decodes the title.
	reTestBlock  = regexp.MustCompile(`\btest(?:\.(?:only|skip|fixme))?\s*\(\s*(?:'((?:[^'\\]|\\.)*)'|"((?:[^"\\]|\\.)*)"|` + "`((?:[^`\\\\]|\\\\.)*)`" + `)`)
	reMarker     = regexp.MustCompile(`//\s*@domain-assert:`)
	reExpect     = regexp.MustCompile(`\bexpect\s*\(`)
	reExpectPgAt = regexp.MustCompile(`^\bexpect\s*\(\s*page\b`)

	reScenarioHdr = regexp.MustCompile(`(?m)^\s*//\s*@scenario\s+(ZEN-\d+)\b`)
	reACTag       = regexp.MustCompile(`@ac:(\d+)`)
	// The trailing alternative catches test.step.skip(...) and testInfo/step.skip(...): a skipped step's expects never run.
	reSkipOnly = regexp.MustCompile(`\btest\.(?:describe(?:\.(?:serial|parallel))?\.(?:skip|fixme|only)|skip|fixme|only|fail)\s*\(|\.(?:skip|fixme)\s*\(`)
	// expect-literal scans expect(, expect.soft( and expect.poll( statements.
	reExpectLine = regexp.MustCompile(`\bexpect(?:\s*\.(?:soft|poll))?\s*\(`)
	// A regex in a copy-bearing position escapes the literal check, so it is flagged.
	reCopyRegex = regexp.MustCompile(`(?:\.(?:toHaveText|toContainText|toHaveValue|toHaveValues|toHaveAccessibleName|toHaveAccessibleDescription|getByText|getByLabel|getByPlaceholder|getByAltText|getByTitle)\(\s*\[?\s*|\b(?:name|hasText|hasNotText)\s*:\s*)/[^/\n]`)
	// Text engines inside a locator(...) selector carry user-visible copy.
	reTextEngine = regexp.MustCompile(`(?:^|>>)\s*(?:internal:)?(?:has-)?text=|:has-text\(|:text(?:-is|-matches)?\(`)
	reEngineText = regexp.MustCompile(`(?:^|>>)\s*(?:internal:)?(?:has-)?text=\s*(?:"([^"]*)"[is]?|'([^']*)'[is]?|([^>]*?))\s*(?:>>|$)|:(?:has-text|text|text-is)\(\s*(?:"([^"]*)"|'([^']*)')\s*\)`)
	reTplExpr    = regexp.MustCompile(`\$\{[^}]*\}`)
	// Comments that are lint markers survive blankComments; `// // @domain-assert` does not.
	reKeptComment = regexp.MustCompile(`^//\s*@(?:scenario\s|domain-assert:)`)
	// Only selector-ish arguments are exempt from expect-literal: getByRole's role (first arg),
	// getByTestId's id and locator(...) CSS without a text engine. getByText/getByLabel/
	// getByPlaceholder, getByRole name and hasText carry user-visible copy and stay checked.
	reSelectorArg = regexp.MustCompile(`\.(?:getByRole|getByTestId|locator)\(\s*(?:'((?:[^'\\\n]|\\.)*)'|"((?:[^"\\\n]|\\.)*)"|` + "`((?:[^`\\\\\n]|\\\\.)*)`" + `)`)
	// An apiClient call's path is not copy (expect.poll re-fetches through it).
	reAPIArg  = regexp.MustCompile(`\bapiClient\.\w+\(\s*(?:'(?:[^'\\\n]|\\.)*'|"(?:[^"\\\n]|\\.)*"|` + "`(?:[^`\\\\\n]|\\\\.)*`" + `)`)
	reLiteral = regexp.MustCompile(`'((?:[^'\\\n]|\\.)*)'|"((?:[^"\\\n]|\\.)*)"|` + "`((?:[^`\\\\\n]|\\\\.)*)`")
)

// splitTests cuts the file into test blocks by `test(` position. Block i runs from the start
// of match i to the start of match i+1 (enough to scan by rule; no need to parse brace balance).
func splitTests(src string) [][2]int {
	locs := reTestBlock.FindAllStringIndex(src, -1)
	var spans [][2]int
	for i, l := range locs {
		end := len(src)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		spans = append(spans, [2]int{l[0], end})
	}
	return spans
}

func hasAC(sc Scenario, n int) bool {
	for _, ac := range sc.ACs {
		if ac.N == n {
			return true
		}
	}
	return false
}

func lineAt(src string, off int) int { return 1 + strings.Count(src[:off], "\n") }

// blankComments replaces // and /* */ comments with spaces (newlines kept, so offsets and
// line numbers hold), except the `// @scenario` and `// @domain-assert:` markers. String
// literals are skipped, so `'https://…'` is not read as a comment.
func blankComments(src string) string {
	b := []byte(src)
	for i := 0; i < len(b); i++ {
		switch c := b[i]; {
		case c == '\'' || c == '"' || c == '`':
			for i++; i < len(b) && b[i] != c && (c == '`' || b[i] != '\n'); i++ {
				if b[i] == '\\' {
					i++
				}
			}
		case c == '/' && i+1 < len(b) && b[i+1] == '/':
			end := strings.IndexByte(src[i:], '\n')
			if end < 0 {
				end = len(b) - i
			}
			if reKeptComment.MatchString(src[i : i+end]) {
				i += end - 1
				continue
			}
			for j := i; j < i+end; j++ {
				b[j] = ' '
			}
			i += end - 1
		case c == '/' && i+1 < len(b) && b[i+1] == '*':
			end := strings.Index(src[i+2:], "*/")
			stop := len(b)
			if end >= 0 {
				stop = i + 2 + end + 2
			}
			for j := i; j < stop; j++ {
				if b[j] != '\n' {
					b[j] = ' '
				}
			}
			i = stop - 1
		}
	}
	return string(b)
}

// jsUnescape decodes a JS string literal body: `\'` → `'`, `\n` → newline, `\\` → `\`.
func jsUnescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// statementEnd returns the offset in src where the statement starting at start ends: a `;`
// outside brackets and strings, or a newline outside them not followed by a `.` chain
// (`expect(x)\n  .toHaveText('y')` is one statement), or the end of src.
func statementEnd(src string, start int) int {
	depth := 0
	for i := start; i < len(src); i++ {
		switch c := src[i]; c {
		case '\'', '"', '`':
			for i++; i < len(src) && src[i] != c; i++ {
				if src[i] == '\\' {
					i++
				}
			}
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
			if depth < 0 {
				return i
			}
		case ';':
			if depth == 0 {
				return i
			}
		case '\n':
			if depth == 0 && !strings.HasPrefix(strings.TrimLeft(src[i:], " \t\r\n"), ".") {
				return i
			}
		}
	}
	return len(src)
}

// selectorCopy returns the user-visible copy carried by a locator(...) selector's text
// engines; ok=false when the selector has a text engine whose text cannot be extracted.
func selectorCopy(sel string) (texts []string, ok bool) {
	if !reTextEngine.MatchString(sel) {
		return nil, true
	}
	for _, m := range reEngineText.FindAllStringSubmatch(sel, -1) {
		t := strings.TrimSpace(strings.Join(m[1:], ""))
		if t == "" || strings.HasPrefix(t, "/") {
			return nil, false
		}
		texts = append(texts, t)
	}
	return texts, len(texts) > 0
}

// expectLiterals returns the literal copy one expect statement asserts: string literals outside
// selector and apiClient arguments, text-engine copy inside locator selectors, and the
// static parts of template literals.
func expectLiterals(line string) []string {
	var lits []string
	for _, m := range reSelectorArg.FindAllStringSubmatch(line, -1) {
		if !strings.Contains(m[0], ".locator(") {
			continue
		}
		sel := jsUnescape(m[1] + m[2] + m[3])
		texts, ok := selectorCopy(sel)
		if !ok {
			texts = []string{sel}
		}
		lits = append(lits, texts...)
	}
	stripped := reAPIArg.ReplaceAllString(reSelectorArg.ReplaceAllString(line, ""), "")
	for _, lm := range reLiteral.FindAllStringSubmatch(stripped, -1) {
		if lm[3] != "" {
			for _, part := range reTplExpr.Split(lm[3], -1) {
				if p := strings.TrimSpace(jsUnescape(part)); p != "" {
					lits = append(lits, p)
				}
			}
			continue
		}
		if lit := jsUnescape(lm[1] + lm[2]); lit != "" {
			lits = append(lits, lit)
		}
	}
	return lits
}

// LintSource scans one spec file, returning findings (empty = pass).
func LintSource(name, src string, scenarios map[string]Scenario) []Finding {
	var out []Finding
	// Commented-out code must not count: a `// test('… @ac:2')` would otherwise cover @ac:2.
	src = blankComments(src)
	hasAfterEach := strings.Contains(src, "test.afterEach")
	add := func(off int, rule, msg string) {
		out = append(out, Finding{File: name, Line: lineAt(src, off), Rule: rule, Msg: msg})
	}
	spans := splitTests(src)
	// A *.spec.ts with no test(...) scenario at all (e.g. fully commented out, or only an
	// empty test.describe) must NOT be read as "clean" — there is nothing to check
	// marker/refetch against, so it must be flagged instead of silently returning 0 findings.
	if len(spans) == 0 {
		add(0, "no-test", "*.spec.ts không có scenario test(...) nào để soi") //znf:allow-lang
		return out
	}
	// Scenario-bound rules apply only to journeys that declare `// @scenario ZEN-<n>`.
	var sc Scenario
	bound, hasScenario := false, false
	if m := reScenarioHdr.FindStringSubmatchIndex(src); m != nil {
		key := src[m[2]:m[3]]
		if s, ok := scenarios[key]; ok {
			sc, bound = s, true
		} else {
			add(m[0], "scenario-link", "journey references "+key+" but no scenarios/"+key+".md was found")
		}
		hasScenario = true
		for _, l := range reSkipOnly.FindAllStringIndex(src, -1) {
			add(l[0], "no-skip", "test.skip/fixme/only/fail and test.describe.skip/fixme/only are not allowed in a scenario-bound journey")
		}
	}
	covered := map[int]bool{}
	for _, sp := range spans {
		block := src[sp[0]:sp[1]]
		base := sp[0]
		title := ""
		if tm := reTestBlock.FindStringSubmatch(block); tm != nil {
			title = jsUnescape(tm[1] + tm[2] + tm[3])
		}
		acs := reACTag.FindAllStringSubmatch(title, -1)
		for _, a := range acs {
			n, err := strconv.Atoi(a[1])
			if err != nil {
				continue
			}
			covered[n] = true
			if bound && !hasAC(sc, n) {
				add(base, "ac-coverage", "test is tagged @ac:"+a[1]+" but the scenario has no such @ac")
			}
		}

		if bound && len(acs) > 0 {
			// Whole statements, not lines: a matcher or its argument on a continuation line
			// (`expect(loc)\n  .toHaveText('x')`) is still checked.
			done := 0
			for _, m := range reExpectLine.FindAllStringIndex(block, -1) {
				if m[0] < done {
					continue // inside the previous statement (expect within expect.poll)
				}
				done = statementEnd(block, m[0])
				stmt := strings.ReplaceAll(block[m[0]:done], "\n", " ")
				if reCopyRegex.MatchString(stmt) {
					add(base+m[0], "expect-literal", "a regex in place of on-screen text cannot be checked against the scenario; use the exact text")
				}
				for _, lit := range expectLiterals(stmt) {
					if !strings.Contains(sc.Raw, lit) {
						add(base+m[0], "expect-literal", "expected text "+strconv.Quote(lit)+" is not in the scenario file")
					}
				}
			}
		}

		// traceability (a scenario-bound journey may use @ac:<n> instead of FR-/SC-)
		acOK := hasScenario && len(acs) > 0
		if !acOK && !strings.Contains(block, "FR-") && !strings.Contains(block, "SC-") {
			add(base, "traceability", "scenario thiếu tham chiếu FR-/SC-") //znf:allow-lang
		}
		// anti-pattern
		if i := strings.Index(block, "networkidle"); i >= 0 {
			add(base+i, "no-networkidle", "cấm networkidle (Socket.io/BullMQ gây flaky)") //znf:allow-lang
		}
		if i := strings.Index(block, "waitForTimeout"); i >= 0 {
			add(base+i, "no-hardwait", "cấm waitForTimeout — dùng web-first assertion") //znf:allow-lang
		}
		for _, frag := range []string{"xpath=", "nth-child(", ">> nth="} {
			if i := strings.Index(block, frag); i >= 0 {
				add(base+i, "no-fragile-selector", "selector mong manh: "+frag+" — dùng getByRole/getByTestId/getByPlaceholder") //znf:allow-lang
			}
		}
		// marker
		markers := reMarker.FindAllStringIndex(block, -1)
		switch {
		case len(markers) == 0:
			add(base, "marker", "thiếu // @domain-assert:<entity> sau thao tác UI") //znf:allow-lang
			// no marker means the after-marker refetch check does not apply
		case len(markers) > 1:
			add(base, "marker", "chỉ được đúng một // @domain-assert:<entity> mỗi scenario") //znf:allow-lang
			fallthrough
		default:
			mOff := markers[0][1]
			after := block[markers[0][0]:]
			// Exclude the cleanupTracker.add(...) body from the scan window: `apiClient.put`
			// inside the cleanup closure is NOT a re-fetch — if counted, a shallow test with
			// only cleanup (no actual re-fetch/domain assertion) would still pass the rule.
			// The real re-fetch + assertion must come BEFORE registering cleanup (per the exemplar shape).
			scan := after
			if i := strings.Index(after, "cleanupTracker.add("); i >= 0 {
				scan = after[:i]
			}
			hasClient := strings.Contains(scan, "apiClient.")
			// any expect after the marker (before cleanup) that is not expect(page
			realExpect := false
			for _, e := range reExpect.FindAllStringIndex(scan, -1) {
				if !reExpectPgAt.MatchString(scan[e[0]:]) {
					realExpect = true
					break
				}
			}
			if !hasClient || !realExpect {
				add(base+mOff, "refetch", "sau @domain-assert phải có apiClient re-fetch + expect trên field thật (không chỉ expect(page))") //znf:allow-lang
			}
		}
		// cleanup (only required when a marker exists = an entity was created/asserted)
		if len(markers) > 0 && !strings.Contains(block, "cleanupTracker") && !hasAfterEach {
			add(base, "cleanup", "scenario tạo entity phải cleanupTracker.add(...) hoặc test.afterEach xoá") //znf:allow-lang
		}
	}
	if bound {
		var missing []string
		for _, ac := range sc.ACs {
			if ac.Kind == ACKindE2E && !covered[ac.N] {
				missing = append(missing, "@ac:"+strconv.Itoa(ac.N))
			}
		}
		if len(missing) > 0 {
			add(0, "ac-coverage", "no test covers "+strings.Join(missing, ", "))
		}
	}
	return out
}

// Lint scans every *.spec.ts in specDir, prints findings, and returns (finding count, error-exitcode).
func Lint(specDir string, out io.Writer) (int, error) {
	entries, err := filepath.Glob(filepath.Join(specDir, "*.spec.ts"))
	if err != nil {
		return 0, exitcode.New(exitcode.Fail, err)
	}
	if len(entries) == 0 {
		return 0, exitcode.New(exitcode.BadArgs,
			fmt.Errorf("no *.spec.ts found in %s", specDir))
	}
	total := 0
	scenarios := map[string]Scenario{}
	scenFiles, _ := filepath.Glob(filepath.Join(specDir, "scenarios", "*.md"))
	for _, f := range scenFiles {
		b, err := os.ReadFile(f) //nolint:gosec // G304 -- f enumerated from the scan root
		if err != nil {
			return total, exitcode.New(exitcode.Fail, err)
		}
		sc, perr := ParseScenario(string(b))
		if perr != nil {
			fmt.Fprintf(out, "%s:1 [scenario-parse] %v\n", filepath.Join("scenarios", filepath.Base(f)), perr)
			total++
			continue
		}
		key := strings.TrimSuffix(filepath.Base(f), ".md")
		if sc.Key != key {
			fmt.Fprintf(out, "%s:1 [scenario-parse] heading key %s does not match file name %s\n", filepath.Join("scenarios", filepath.Base(f)), sc.Key, key)
			total++
			continue
		}
		scenarios[key] = sc
	}
	for _, f := range entries {
		b, err := os.ReadFile(f) //nolint:gosec // G304 -- f is a lint-target path enumerated internally from the scan root, not externally-tainted
		if err != nil {
			return total, exitcode.New(exitcode.Fail, err)
		}
		for _, fd := range LintSource(filepath.Base(f), string(b), scenarios) {
			fmt.Fprintf(out, "%s:%d [%s] %s\n", fd.File, fd.Line, fd.Rule, fd.Msg)
			total++
		}
	}
	if total > 0 {
		return total, exitcode.New(exitcode.Fail, fmt.Errorf("%d e2e-lint violations", total))
	}
	return 0, nil
}
