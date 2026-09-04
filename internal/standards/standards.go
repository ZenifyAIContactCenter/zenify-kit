// Package standards checks test-traceability: every FR declared in the spec
// should have a real test on disk. It reuses internal/analyze for the FR set
// and FR->task coverage, then walks the plan for the Test: file paths each task
// declares and checks them on disk (language-aware). It never modifies analyze.
//
// Advisory + fail-open by construction: Check never panics and never returns an
// error; the caller decides nothing blocks on it.
package standards

import (
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
)

func (r *Result) add(f Finding) {
	r.Findings = append(r.Findings, f)
	r.SeverityCounts[f.Severity]++
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
		if cur != "" && testBulletRe.MatchString(ln) {
			if m := backtickRe.FindStringSubmatch(ln); m != nil {
				out[cur] = append(out[cur], strings.TrimSpace(m[1]))
			}
		}
	}
	return out
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
