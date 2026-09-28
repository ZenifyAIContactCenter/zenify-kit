package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitInit builds a temp repo with 1 base commit, returns the repo path and BASE sha.
func gitInit(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	run("init", "-q")
	run("config", "user.email", "t@t.t")
	run("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "seed.txt"), []byte("seed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "seed")
	base := run("rev-parse", "HEAD")
	return dir, base[:len(base)-1] // strip newline
}

// gitCommitAll add + commits every current change in the temp repo — needed because the gate uses
// `git diff "$BASE"` (which only sees COMMITTED/tracked changes, not untracked files).
func gitCommitAll(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "change")
}

func runGate(t *testing.T, dir, base string, env ...string) struct {
	Verdict  string           `json:"verdict"`
	Findings []map[string]any `json:"findings"`
} {
	t.Helper()
	script, err := filepath.Abs(filepath.Join("assets", "znf", "skills", "review", "scripts", "mechanical-gate"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", script, base)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("mechanical-gate: %v\n%s", err, out)
	}
	var res struct {
		Verdict  string           `json:"verdict"`
		Findings []map[string]any `json:"findings"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("gate stdout is not JSON: %v (%q)", err, string(out))
	}
	return res
}

func TestGate_CleanPasses(t *testing.T) {
	dir, base := gitInit(t)
	if err := os.WriteFile(filepath.Join(dir, "ok.txt"), []byte("clean line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, dir)
	res := runGate(t, dir, base)
	if res.Verdict != "pass" {
		t.Errorf("verdict=%q, want pass", res.Verdict)
	}
}

func TestGate_ConflictMarkerBlocks(t *testing.T) {
	dir, base := gitInit(t)
	body := "a\n<<<<<<< HEAD\nb\n=======\nc\n>>>>>>> other\n"
	if err := os.WriteFile(filepath.Join(dir, "merge.txt"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, dir)
	res := runGate(t, dir, base)
	if res.Verdict != "block" {
		t.Errorf("verdict=%q, want block (conflict marker)", res.Verdict)
	}
}

// SC-02: STATIC_OK=1 skips the build → a broken go.mod still passes (build never runs), no "build fail".
func TestGate_StaticOKSkipsBuild(t *testing.T) {
	dir, base := gitInit(t)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module tmptest\n\ngo 1.21\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.go"), []byte("package main\nfunc main() { this is not go }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, dir)
	res := runGate(t, dir, base, "STATIC_OK=1")
	if res.Verdict != "pass" {
		t.Errorf("verdict=%q, want pass (STATIC_OK must skip the build)", res.Verdict)
	}
	for _, f := range res.Findings {
		if f["title"] == "build fail" {
			t.Errorf("got a build fail finding despite STATIC_OK=1 (build should have been skipped)")
		}
	}
}

// SC-01: build fail → block. Needs go on PATH; broken.go makes `go build ./...` fail.
func TestGate_BuildFailBlocks(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}
	dir, base := gitInit(t)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module tmptest\n\ngo 1.21\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.go"), []byte("package main\nfunc main() { this is not go }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, dir)
	res := runGate(t, dir, base) // STATIC_OK defaults to 0 → build runs
	if res.Verdict != "block" {
		t.Errorf("verdict=%q, want block (build fail)", res.Verdict)
	}
}

// FR-03: focused test (.only) + debugger in the diff → produces a finding (HIGH, non-blocking).
func TestGate_FocusedTestAndDebugger(t *testing.T) {
	dir, base := gitInit(t)
	body := "test.only('x', () => { debugger; });\n"
	if err := os.WriteFile(filepath.Join(dir, "a.test.js"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, dir)
	res := runGate(t, dir, base, "STATIC_OK=1") // skip build; only scan anti-patterns
	var focused, dbg bool
	for _, f := range res.Findings {
		switch f["title"] {
		case "focused test":
			focused = true
		case "debugger":
			dbg = true
		}
	}
	if !focused || !dbg {
		t.Errorf("focused=%v debugger=%v, want both true", focused, dbg)
	}
}

func pyAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}
}

func findingByTitle(findings []map[string]any, title string) map[string]any {
	for _, f := range findings {
		if f["title"] == title {
			return f
		}
	}
	return nil
}

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// SC-1: a Python repo whose diff adds a file with a syntax error → CRITICAL syntax error, block.
func TestGate_PythonSyntaxErrorBlocks(t *testing.T) {
	pyAvailable(t)
	dir, base := gitInit(t)
	writeFile(t, dir, "requirements.txt", "fastapi\n")
	writeFile(t, dir, "bad.py", "def f(:\n    return 1\n")
	gitCommitAll(t, dir)
	res := runGate(t, dir, base)
	if res.Verdict != "block" {
		t.Errorf("verdict=%q, want block", res.Verdict)
	}
	f := findingByTitle(res.Findings, "syntax error")
	if f == nil || f["severity"] != "CRITICAL" {
		t.Errorf("want CRITICAL syntax error finding, got %v", res.Findings)
	}
	if _, err := os.Stat(filepath.Join(dir, "__pycache__")); err == nil {
		t.Errorf("gate must not write __pycache__ into the repo")
	}
}

// SC-2: valid Python, no linter config → pass with LOW "no linter configured".
func TestGate_PythonNoLinterIsLowNote(t *testing.T) {
	pyAvailable(t)
	dir, base := gitInit(t)
	writeFile(t, dir, "requirements.txt", "fastapi\n")
	writeFile(t, dir, "ok.py", "x = 1\n")
	gitCommitAll(t, dir)
	res := runGate(t, dir, base)
	if res.Verdict != "pass" {
		t.Errorf("verdict=%q, want pass", res.Verdict)
	}
	f := findingByTitle(res.Findings, "no linter configured")
	if f == nil || f["severity"] != "LOW" {
		t.Errorf("want LOW no linter configured, got %v", res.Findings)
	}
}

// SC-3: ruff.toml present but no ruff binary → pass with LOW skip lint.
func TestGate_PythonRuffConfiguredButMissing(t *testing.T) {
	pyAvailable(t)
	if _, err := exec.LookPath("ruff"); err == nil {
		t.Skip("ruff is on PATH; this case needs it absent")
	}
	dir, base := gitInit(t)
	writeFile(t, dir, "pyproject.toml", "[project]\nname = \"t\"\n")
	writeFile(t, dir, "ruff.toml", "line-length = 100\n")
	writeFile(t, dir, "ok.py", "x = 1\n")
	gitCommitAll(t, dir)
	res := runGate(t, dir, base)
	if res.Verdict != "pass" {
		t.Errorf("verdict=%q, want pass", res.Verdict)
	}
	f := findingByTitle(res.Findings, "skip lint")
	if f == nil || f["severity"] != "LOW" || !strings.Contains(fmt.Sprint(f["issue"]), "ruff unavailable") {
		t.Errorf("want LOW skip lint (ruff unavailable), got %v", res.Findings)
	}
}
