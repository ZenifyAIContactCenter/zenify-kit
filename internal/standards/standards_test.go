package standards

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// helper: write a file under a temp root, return the root.
func withFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func kinds(r Result) map[string]int {
	m := map[string]int{}
	for _, f := range r.Findings {
		m[f.Kind]++
	}
	return m
}

func TestCheck_FRWithRealTest_NoFinding(t *testing.T) {
	spec := "**FR-1.** do a thing.\n"
	plan := "### Task 1: X\n**Files:**\n- Test: `a/b_test.go`\n- `_Requirements: FR-1_`\n"
	root := withFiles(t, map[string]string{"a/b_test.go": "package a\nfunc TestX(t *testing.T){}\n"})
	r := Check(spec, plan, root, os.ReadFile)
	if len(r.Findings) != 0 {
		t.Fatalf("want 0 findings, got %+v", r.Findings)
	}
}

func TestCheck_UntestedFR(t *testing.T) {
	spec := "**FR-2.** another thing.\n"
	plan := "### Task 1: X\n**Files:**\n- Create: `a/b.go`\n- `_Requirements: FR-2_`\n" // no Test bullet
	root := withFiles(t, map[string]string{"a/b.go": "package a\n"})
	r := Check(spec, plan, root, os.ReadFile)
	if kinds(r)["untested-fr"] != 1 {
		t.Fatalf("want 1 untested-fr, got %+v", r.Findings)
	}
}

func TestCheck_MissingTestFile(t *testing.T) {
	spec := "**FR-1.** x.\n"
	plan := "### Task 1: X\n**Files:**\n- Test: `missing/x_test.go`\n- `_Requirements: FR-1_`\n"
	root := t.TempDir() // file not created
	r := Check(spec, plan, root, os.ReadFile)
	if kinds(r)["missing-test-file"] != 1 {
		t.Fatalf("want 1 missing-test-file, got %+v", r.Findings)
	}
}

func TestCheck_EmptyTestFile(t *testing.T) {
	spec := "**FR-1.** x.\n"
	plan := "### Task 1: X\n**Files:**\n- Test: `a/b_test.go`\n- `_Requirements: FR-1_`\n"
	root := withFiles(t, map[string]string{"a/b_test.go": "package a\n// no test func here\n"})
	r := Check(spec, plan, root, os.ReadFile)
	if kinds(r)["empty-test-file"] != 1 {
		t.Fatalf("want 1 empty-test-file, got %+v", r.Findings)
	}
}

func TestCheck_CompoundCreateTestLabel(t *testing.T) {
	spec := "**FR-1.** x.\n"
	plan := "### Task 1: X\n**Files:**\n- Create/Test: `a/b_test.go`\n- `_Requirements: FR-1_`\n"
	root := withFiles(t, map[string]string{"a/b_test.go": "package a\nfunc TestX(t *testing.T){}\n"})
	r := Check(spec, plan, root, os.ReadFile)
	if len(r.Findings) != 0 {
		t.Fatalf("compound Create/Test label must be recognised; got %+v", r.Findings)
	}
}

func TestCheck_FailOpen_NilReadFileNeverPanics(t *testing.T) {
	spec := "**FR-1.** x.\n"
	plan := "### Task 1: X\n**Files:**\n- Test: `a/b_test.go`\n- `_Requirements: FR-1_`\n"
	// readFile always errors — must degrade to missing-test-file, never panic.
	r := Check(spec, plan, "/nonexistent-root", func(string) ([]byte, error) { return nil, os.ErrNotExist })
	if kinds(r)["missing-test-file"] != 1 {
		t.Fatalf("want 1 missing-test-file on read error, got %+v", r.Findings)
	}
}

func TestCheck_LangDetect_JSAndPy(t *testing.T) {
	// JS with it() → tested; Py without def test_ → empty.
	spec := "**FR-1.** a.\n**FR-2.** b.\n"
	plan := "### Task 1: A\n**Files:**\n- Test: `a.test.js`\n- `_Requirements: FR-1_`\n" +
		"### Task 2: B\n**Files:**\n- Test: `b_test.py`\n- `_Requirements: FR-2_`\n"
	root := withFiles(t, map[string]string{
		"a.test.js": "it('works', () => { expect(1).toBe(1) })\n",
		"b_test.py": "x = 1  # no test def\n",
	})
	r := Check(spec, plan, root, os.ReadFile)
	k := kinds(r)
	if k["empty-test-file"] != 1 || k["untested-fr"] != 0 || k["missing-test-file"] != 0 {
		t.Fatalf("want exactly 1 empty-test-file (the .py), got %+v", r.Findings)
	}
}

// D1: a _test.go listed under a Create:/Modify: bullet (label not "Test") must
// still be collected as a test path, while a production file alongside it must not.
func TestTestPathsByTask_CollectsTestFileUnderCreateBullet(t *testing.T) {
	plan := "### Task 1: thing\n" +
		"**Files:**\n" +
		"- Create: `internal/x/x.go`\n" +
		"- Create: `internal/x/x_test.go`\n"
	var all []string
	for _, ps := range testPathsByTask(plan) {
		all = append(all, ps...)
	}
	if !stdContains(all, "internal/x/x_test.go") {
		t.Fatalf("expected x_test.go collected, got %v", all)
	}
	if stdContains(all, "internal/x/x.go") {
		t.Fatalf("production file must NOT be collected as a test path: %v", all)
	}
}

// D1 regression: testFileNameRe mirrors testFuncRe's languages — a *_test.py
// (pytest suffix), a .mjs test, and a Minitest *_test.rb under a Create: bullet
// must all be collected, or the false untested-fr this fix kills returns for non-Go.
func TestTestPathsByTask_CollectsNonGoTestConventions(t *testing.T) {
	plan := "### Task 1: polyglot\n" +
		"**Files:**\n" +
		"- Create: `svc/foo_test.py`\n" +
		"- Create: `web/foo.test.mjs`\n" +
		"- Create: `lib/foo_test.rb`\n" +
		"- Create: `svc/foo.py`\n"
	var all []string
	for _, ps := range testPathsByTask(plan) {
		all = append(all, ps...)
	}
	for _, want := range []string{"svc/foo_test.py", "web/foo.test.mjs", "lib/foo_test.rb"} {
		if !stdContains(all, want) {
			t.Fatalf("expected %s collected, got %v", want, all)
		}
	}
	if stdContains(all, "svc/foo.py") {
		t.Fatalf("production file must NOT be collected as a test path: %v", all)
	}
}

// D1 regression: the legacy "Test:" label path still works.
func TestTestPathsByTask_LegacyTestLabelStillWorks(t *testing.T) {
	plan := "### Task 2: legacy\n" +
		"- Test: `pkg/foo_test.go`\n"
	var all []string
	for _, ps := range testPathsByTask(plan) {
		all = append(all, ps...)
	}
	if !stdContains(all, "pkg/foo_test.go") {
		t.Fatalf("legacy Test: label path lost, got %v", all)
	}
}

func stdContains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func TestAsTestPath_FiltersCommandsGlobsAndLineSuffix(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"go test ./...", "", false},                     // command: whitespace
		{"*.test.js", "", false},                         // glob
		{"*/**/*.spec.ts", "", false},                    // glob
		{"a/b_test.go:20-31", "a/b_test.go", true},       // line range suffix stripped
		{"a/b_test.go:7", "a/b_test.go", true},           // single line suffix stripped
		{"TestX", "", false},                             // bare identifier: no / and no .
		{"a/b_test.go", "a/b_test.go", true},             // plain path unchanged
		{" ensure_test.go ", "ensure_test.go", true},     // bare file name kept (resolved later)
		{"../../etc/passwd", "", false},                  // escapes root
		{"../x_test.go", "", false},                      // escapes root
		{"_test.go", "", false},                          // bare suffix mention, not a file
		{".test.js", "", false},                          // bare suffix mention, not a file
		{".claude/x_test.go", ".claude/x_test.go", true}, // dot-dir with a slash is a real path
	}
	for _, c := range cases {
		got, ok := asTestPath(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("asTestPath(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestTestPathsByTask_DeleteBulletIsNotADeclaration(t *testing.T) {
	plan := "### Task 9: remove\n" +
		"- Delete: `internal/cli/selfheal.go`, `internal/cli/selfheal_test.go`\n" +
		"- Remove: `internal/plugin/hooks_json_test.go`\n" +
		"- Create: `internal/cli/ensure_test.go`\n"
	got := testPathsByTask(plan)["### Task 9: remove"]
	if len(got) != 1 || got[0] != "internal/cli/ensure_test.go" {
		t.Fatalf("Delete:/Remove: bullets must contribute nothing, got %v", got)
	}
}

func TestTestPathsByTask_LegacyLabelSkipsCommandTakesPath(t *testing.T) {
	plan := "### Task 3: cmd-first\n" +
		"- Test: `go test ./internal/x` then `internal/x/x_test.go`\n"
	var all []string
	for _, ps := range testPathsByTask(plan) {
		all = append(all, ps...)
	}
	if len(all) != 1 || all[0] != "internal/x/x_test.go" {
		t.Fatalf("expected only the path after the command, got %v", all)
	}
}

func TestTestPathsByTask_LegacyLabelCommandOnlyContributesNothing(t *testing.T) {
	plan := "### Task 4: cmd-only\n" +
		"- Test: `go test ./...`\n"
	if ps := testPathsByTask(plan)["### Task 4: cmd-only"]; len(ps) != 0 {
		t.Fatalf("command-only Test: bullet must contribute no path, got %v", ps)
	}
}

func TestTestPathsByTask_CreateBulletStripsLineSuffix(t *testing.T) {
	plan := "### Task 5: modify\n" +
		"- Modify: `internal/x/x_test.go:20-31`\n"
	var all []string
	for _, ps := range testPathsByTask(plan) {
		all = append(all, ps...)
	}
	if !stdContains(all, "internal/x/x_test.go") {
		t.Fatalf("expected line-suffix stripped path collected, got %v", all)
	}
}

func TestCheck_BareNameResolvedByUniqueBasename(t *testing.T) {
	spec := "- **FR-1** thing\n"
	plan := "### Task 1: t\n- Test: `b_test.go`\n- `_Requirements: FR-1_`\n"
	root := withFiles(t, map[string]string{"a/b_test.go": "package a\nfunc TestX(t *testing.T){}\n"})
	r := Check(spec, plan, root, os.ReadFile)
	if k := kinds(r); k["missing-test-file"] != 0 || k["untested-fr"] != 0 {
		t.Fatalf("bare name with one match must resolve cleanly, got %v", k)
	}
}

func TestCheck_BareNameAmbiguousIsMissing(t *testing.T) {
	spec := "- **FR-1** thing\n"
	plan := "### Task 1: t\n- Test: `b_test.go`\n- `_Requirements: FR-1_`\n"
	root := withFiles(t, map[string]string{
		"a/b_test.go": "package a\nfunc TestX(t *testing.T){}\n",
		"c/b_test.go": "package c\nfunc TestY(t *testing.T){}\n",
	})
	r := Check(spec, plan, root, os.ReadFile)
	if k := kinds(r); k["missing-test-file"] != 1 {
		t.Fatalf("ambiguous bare name must be reported once, got %v", k)
	}
	if !strings.Contains(r.Findings[0].Message, "2 files") {
		t.Fatalf("message must state the match count, got %q", r.Findings[0].Message)
	}
}

func TestCheck_BareNameSkipsGitAndNodeModules(t *testing.T) {
	spec := "- **FR-1** thing\n"
	plan := "### Task 1: t\n- Test: `b_test.go`\n- `_Requirements: FR-1_`\n"
	root := withFiles(t, map[string]string{
		"a/b_test.go":              "package a\nfunc TestX(t *testing.T){}\n",
		"node_modules/x/b_test.go": "junk",
		".git/b_test.go":           "junk",
	})
	r := Check(spec, plan, root, os.ReadFile)
	if k := kinds(r); k["missing-test-file"] != 0 {
		t.Fatalf("matches under .git/node_modules must not count, got %v", k)
	}
}
