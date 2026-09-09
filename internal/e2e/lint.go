package e2e

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
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
	reTestBlock  = regexp.MustCompile(`(?s)\btest(?:\.(?:only|skip|fixme))?\s*\(\s*['"` + "`" + `](.*?)['"` + "`" + `]`)
	reMarker     = regexp.MustCompile(`//\s*@domain-assert:`)
	reExpect     = regexp.MustCompile(`\bexpect\s*\(`)
	reExpectPgAt = regexp.MustCompile(`^\bexpect\s*\(\s*page\b`)
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

func lineAt(src string, off int) int { return 1 + strings.Count(src[:off], "\n") }

// LintSource scans one spec file, returning findings (empty = pass).
func LintSource(name, src string) []Finding {
	var out []Finding
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
	for _, sp := range spans {
		block := src[sp[0]:sp[1]]
		base := sp[0]

		// traceability
		if !strings.Contains(block, "FR-") && !strings.Contains(block, "SC-") {
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
	for _, f := range entries {
		b, err := os.ReadFile(f)
		if err != nil {
			return total, exitcode.New(exitcode.Fail, err)
		}
		for _, fd := range LintSource(filepath.Base(f), string(b)) {
			fmt.Fprintf(out, "%s:%d [%s] %s\n", fd.File, fd.Line, fd.Rule, fd.Msg)
			total++
		}
	}
	if total > 0 {
		return total, exitcode.New(exitcode.Fail, fmt.Errorf("%d e2e-lint violations", total))
	}
	return 0, nil
}
