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
	reTestBlock = regexp.MustCompile(`(?s)\btest\s*\(\s*['"` + "`" + `](.*?)['"` + "`" + `]`)
	reMarker    = regexp.MustCompile(`//\s*@domain-assert:`)
	reExpect    = regexp.MustCompile(`\bexpect\s*\(`)
	reExpectPg  = regexp.MustCompile(`\bexpect\s*\(\s*page\b`)
)

// splitTests cắt file thành các khối test theo vị trí `test(`. Khối i chạy từ đầu match i
// tới đầu match i+1 (đủ để quét theo rule; không cần parse cân ngoặc).
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

// LintSource quét một file spec, trả findings (rỗng = đạt).
func LintSource(name, src string) []Finding {
	var out []Finding
	hasAfterEach := strings.Contains(src, "test.afterEach")
	add := func(off int, rule, msg string) {
		out = append(out, Finding{File: name, Line: lineAt(src, off), Rule: rule, Msg: msg})
	}
	for _, sp := range splitTests(src) {
		block := src[sp[0]:sp[1]]
		base := sp[0]

		// traceability
		if !strings.Contains(block, "FR-") && !strings.Contains(block, "SC-") {
			add(base, "traceability", "scenario thiếu tham chiếu FR-/SC-")
		}
		// anti-pattern
		if i := strings.Index(block, "networkidle"); i >= 0 {
			add(base+i, "no-networkidle", "cấm networkidle (Socket.io/BullMQ gây flaky)")
		}
		if i := strings.Index(block, "waitForTimeout"); i >= 0 {
			add(base+i, "no-hardwait", "cấm waitForTimeout — dùng web-first assertion")
		}
		for _, frag := range []string{"xpath=", "nth-child(", ">> nth="} {
			if i := strings.Index(block, frag); i >= 0 {
				add(base+i, "no-fragile-selector", "selector mong manh: "+frag+" — dùng getByRole/getByTestId/getByPlaceholder")
			}
		}
		// marker
		markers := reMarker.FindAllStringIndex(block, -1)
		if len(markers) == 0 {
			add(base, "marker", "thiếu // @domain-assert:<entity> sau thao tác UI")
			// không có marker thì không xét refetch theo-sau-marker
		} else {
			mOff := markers[0][1]
			after := block[markers[0][0]:]
			hasClient := strings.Contains(after, "apiClient.")
			// một expect nào đó sau marker không phải expect(page
			realExpect := false
			for _, e := range reExpect.FindAllStringIndex(after, -1) {
				if !reExpectPg.MatchString(after[e[0]:min(e[0]+20, len(after))]) {
					realExpect = true
					break
				}
			}
			if !hasClient || !realExpect {
				add(base+mOff, "refetch", "sau @domain-assert phải có apiClient re-fetch + expect trên field thật (không chỉ expect(page))")
			}
		}
		// cleanup (chỉ đòi khi có marker = có tạo/assert entity)
		if len(markers) > 0 && !strings.Contains(block, "cleanupTracker") && !hasAfterEach {
			add(base, "cleanup", "scenario tạo entity phải cleanupTracker.add(...) hoặc test.afterEach xoá")
		}
	}
	return out
}

// Lint quét mọi *.spec.ts trong specDir, in findings, trả (số finding, error-exitcode).
func Lint(specDir string, out io.Writer) (int, error) {
	entries, err := filepath.Glob(filepath.Join(specDir, "*.spec.ts"))
	if err != nil {
		return 0, exitcode.New(exitcode.Fail, err)
	}
	if len(entries) == 0 {
		return 0, exitcode.New(exitcode.BadArgs,
			fmt.Errorf("không thấy *.spec.ts trong %s", specDir))
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
		return total, exitcode.New(exitcode.Fail, fmt.Errorf("%d vi phạm e2e-lint", total))
	}
	return 0, nil
}
