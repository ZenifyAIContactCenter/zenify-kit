package standards

import (
	"os"
	"path/filepath"
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
