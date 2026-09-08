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
	// Khớp cả test.only/.skip/.fixme — nếu không, một scenario `test.only(...)` sẽ
	// KHÔNG match, splitTests trả rỗng và cả gate bỏ qua nó (bypass toàn bộ rule).
	reTestBlock  = regexp.MustCompile(`(?s)\btest(?:\.(?:only|skip|fixme))?\s*\(\s*['"` + "`" + `](.*?)['"` + "`" + `]`)
	reMarker     = regexp.MustCompile(`//\s*@domain-assert:`)
	reExpect     = regexp.MustCompile(`\bexpect\s*\(`)
	reExpectPgAt = regexp.MustCompile(`^\bexpect\s*\(\s*page\b`)
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
	spans := splitTests(src)
	// Một *.spec.ts không có scenario test(...) nào (vd bị comment hết, hoặc chỉ còn
	// test.describe rỗng) KHÔNG được đọc là "sạch" — không có gì để soi marker/refetch,
	// nên phải báo thay vì trả 0 finding im lặng.
	if len(spans) == 0 {
		add(0, "no-test", "*.spec.ts không có scenario test(...) nào để soi")
		return out
	}
	for _, sp := range spans {
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
		switch {
		case len(markers) == 0:
			add(base, "marker", "thiếu // @domain-assert:<entity> sau thao tác UI")
			// không có marker thì không xét refetch theo-sau-marker
		case len(markers) > 1:
			add(base, "marker", "chỉ được đúng một // @domain-assert:<entity> mỗi scenario")
			fallthrough
		default:
			mOff := markers[0][1]
			after := block[markers[0][0]:]
			// Loại phần thân cleanupTracker.add(...) khỏi cửa sổ soi: `apiClient.put`
			// trong closure cleanup KHÔNG phải re-fetch — nếu tính, một test shallow chỉ
			// có cleanup (không hề re-fetch/assert domain) vẫn qua rule. Re-fetch + assert
			// field thật phải nằm TRƯỚC khi đăng ký cleanup (đúng khuôn exemplar).
			scan := after
			if i := strings.Index(after, "cleanupTracker.add("); i >= 0 {
				scan = after[:i]
			}
			hasClient := strings.Contains(scan, "apiClient.")
			// một expect nào đó sau marker (trước cleanup) không phải expect(page
			realExpect := false
			for _, e := range reExpect.FindAllStringIndex(scan, -1) {
				if !reExpectPgAt.MatchString(scan[e[0]:]) {
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
