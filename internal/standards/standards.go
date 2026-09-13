// Package standards checks test-traceability: every FR declared in the spec
// should have a real test on disk. It reuses internal/analyze for the FR set
// and FR->task coverage, then walks the plan for the Test: file paths each task
// declares and checks them on disk (language-aware). It never modifies analyze.
//
// Advisory + fail-open by construction: Check never panics and never returns an
// error; the caller decides nothing blocks on it.
package standards

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/analyze"
)

// Finding is one test-traceability defect.
type Finding struct {
	Severity string `json:"severity"`           // HIGH | MEDIUM | INFO
	Kind     string `json:"kind"`               // untested-fr | missing-test-file | empty-test-file | unchecked-lang
	ID       string `json:"id,omitempty"`       // FR-N (untested-fr)
	Location string `json:"location,omitempty"` // test path (file findings)
	Message  string `json:"message"`
}

// Result is the full mechanical check.
type Result struct {
	TestPaths      []string       `json:"test_paths"`
	Findings       []Finding      `json:"findings"`
	SeverityCounts map[string]int `json:"severity_counts"`
}

var (
	taskRe = regexp.MustCompile(`^###\s+Task\b`)
	// A Files-block bullet whose label (before the colon) contains "Test" —
	// covers "- Test:" and the compound "- Create/Test:". No colon/backtick in the label.
	testBulletRe = regexp.MustCompile("^\\s*[-*+]\\s+[^:`]*Test[^:`]*:")
	backtickRe   = regexp.MustCompile("`([^`]+)`")
	// bulletRe matches any Markdown list item (-, *, +).
	bulletRe = regexp.MustCompile(`^\s*[-*+]\s+`)
	// testFileNameRe matches a path whose basename looks like a test file, in any
	// language testFuncRe covers: *_test.{go,py,rb}, test_*.py, *.{test,spec}.[cm]?[jt]sx?,
	// *_spec.rb. It picks test files out of Files-block bullets whose label does NOT
	// contain "Test" (e.g. "Create:"/"Modify:"), without pulling production paths
	// listed alongside them.
	testFileNameRe = regexp.MustCompile(`(?:_test\.(?:go|py|rb)|(?:\.test|\.spec)\.[cm]?[jt]sx?|_spec\.rb|(?:^|/)test_[^/]+\.py)$`)

	// language-aware "does this file contain a test function?" detectors, by extension.
	jsTestRe   = regexp.MustCompile(`\b(it|test|describe)\s*\(`)
	testFuncRe = map[string]*regexp.Regexp{
		".go":  regexp.MustCompile(`func\s+Test\w`),
		".js":  jsTestRe,
		".ts":  jsTestRe,
		".jsx": jsTestRe,
		".tsx": jsTestRe,
		".mjs": jsTestRe,
		".cjs": jsTestRe,
		".py":  regexp.MustCompile(`(?m)^\s*(def\s+test_|class\s+Test)`),
		".rb":  regexp.MustCompile(`(?m)^\s*(def\s+test_|it\s+['"])`),
	}

	// lineSuffixRe strips a trailing ":<line>" or ":<from>-<to>" that plan
	// authors append to point at a region ("a/b_test.go:20-31").
	lineSuffixRe = regexp.MustCompile(`:\d+(?:-\d+)?$`)
)

func (r *Result) add(f Finding) {
	r.Findings = append(r.Findings, f)
	r.SeverityCounts[f.Severity]++
}

// asTestPath normalises one backtick value from a Files-block bullet into a
// candidate file path. It rejects values that cannot be a path — a command
// ("go test ./..."), a glob ("*.test.js"), a bare identifier ("TestX") — and
// strips a trailing line suffix. A bare file name ("ensure_test.go") is kept;
// Check resolves it by unique basename under root.
func asTestPath(s string) (string, bool) {
	s = strings.TrimSpace(s)
	s = lineSuffixRe.ReplaceAllString(s, "")
	if s == "" || strings.ContainsAny(s, " \t*?[") {
		return "", false
	}
	if !strings.ContainsAny(s, "/.") {
		return "", false
	}
	return s, true
}

// testPathsByTask walks the plan and maps each task title (trimmed "### Task N: …"
// line, identical to analyze's Coverage values) to the test paths its Files block
// declares.
func testPathsByTask(planText string) map[string][]string {
	out := map[string][]string{}
	cur := ""
	for _, ln := range strings.Split(planText, "\n") {
		if taskRe.MatchString(ln) {
			cur = strings.TrimSpace(ln)
			continue
		}
		if cur == "" {
			continue
		}
		// (a) legacy: a bullet whose LABEL contains "Test:" — take the first
		// backtick value that is a path (a command or glob before it is skipped).
		if testBulletRe.MatchString(ln) {
			for _, m := range backtickRe.FindAllStringSubmatch(ln, -1) {
				if p, ok := asTestPath(m[1]); ok {
					out[cur] = append(out[cur], p)
					break
				}
			}
			continue
		}
		// (b) any other bullet (Create:/Modify:/…): collect backtick paths whose
		// basename looks like a test file, so a _test.go listed there still counts
		// as coverage — but never a production path listed alongside it.
		if bulletRe.MatchString(ln) {
			for _, m := range backtickRe.FindAllStringSubmatch(ln, -1) {
				p, ok := asTestPath(m[1])
				if ok && testFileNameRe.MatchString(p) {
					out[cur] = append(out[cur], p)
				}
			}
		}
	}
	return out
}

// skipDirs are never searched when resolving a bare test file name.
var skipDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true, ".worktrees": true}

// resolveBare looks up a declared test path that has no directory component
// ("ensure_test.go") by unique basename under root. It returns the relative
// path found and the number of matches; n == -1 means not applicable (root is
// empty or rel already has a directory). Errors while walking are ignored —
// the caller falls back to the plain missing-test-file finding.
func resolveBare(root, rel string) (string, int) {
	if root == "" || strings.Contains(rel, "/") {
		return rel, -1
	}
	var hits []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == rel {
			if r, e := filepath.Rel(root, p); e == nil {
				hits = append(hits, r)
			}
		}
		return nil
	})
	if len(hits) == 1 {
		return hits[0], 1
	}
	return rel, len(hits)
}

// Check runs the mechanical test-traceability analysis. root is the directory
// test paths resolve against; readFile reads an absolute path (injected for tests).
func Check(specText, planText, root string, readFile func(string) ([]byte, error)) Result {
	r := Result{SeverityCounts: map[string]int{}}
	res := analyze.Analyze(specText, planText)
	byTask := testPathsByTask(planText)

	// Collect all declared test paths (dedup) and check each on disk.
	seen := map[string]bool{}
	checkFile := func(rel string) {
		if seen[rel] {
			return
		}
		seen[rel] = true
		r.TestPaths = append(r.TestPaths, rel)
		b, err := readFile(filepath.Join(root, rel))
		if err != nil {
			if found, n := resolveBare(root, rel); n == 1 {
				rel = found
				b, err = readFile(filepath.Join(root, rel))
			} else if n > 1 {
				r.add(Finding{Severity: "HIGH", Kind: "missing-test-file", Location: rel,
					Message: fmt.Sprintf("bare test file name matches %d files under root — declare the directory", n)})
				return
			}
		}
		if err != nil {
			r.add(Finding{Severity: "HIGH", Kind: "missing-test-file", Location: rel,
				Message: "declared test file not found or unreadable on disk"})
			return
		}
		ext := strings.ToLower(filepath.Ext(rel))
		re, known := testFuncRe[ext]
		if !known {
			r.add(Finding{Severity: "INFO", Kind: "unchecked-lang", Location: rel,
				Message: "unknown test file extension — existence checked, content not"})
			return
		}
		if !re.Match(b) {
			r.add(Finding{Severity: "MEDIUM", Kind: "empty-test-file", Location: rel,
				Message: "test file exists but no test function detected"})
		}
	}
	for _, paths := range byTask {
		for _, p := range paths {
			checkFile(p)
		}
	}

	// untested-fr: an FR with ≥1 covering task but NONE of those tasks declares a test.
	// (An FR with no covering task at all is analyze's orphan-fr — not double-reported.)
	for _, fr := range res.SpecFRs {
		titles := res.Coverage[fr]
		if len(titles) == 0 {
			continue
		}
		hasTest := false
		for _, t := range titles {
			if len(byTask[t]) > 0 {
				hasTest = true
				break
			}
		}
		if !hasTest {
			r.add(Finding{Severity: "HIGH", Kind: "untested-fr", ID: fr,
				Message: "FR is implemented by a task that declares no test"})
		}
	}
	return r
}
