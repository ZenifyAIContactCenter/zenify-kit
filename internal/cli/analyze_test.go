package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SC-5: spec file does not exist → fail-open: note + NO error, exit via RunE = nil.
func TestRunAnalyze_MissingFileFailOpen(t *testing.T) {
	var out, errb bytes.Buffer
	rf := func(p string) ([]byte, error) { return nil, errors.New("no such file") }
	if err := runAnalyze("/no/spec.md", "/no/plan.md", false, rf, &out, &errb); err != nil {
		t.Fatalf("fail-open violated: runAnalyze returned error %v", err)
	}
	if !strings.Contains(errb.String()+out.String(), "không phân tích được") { //znf:allow-lang
		t.Errorf("missing fail-open note; out=%q err=%q", out.String(), errb.String())
	}
}

// Happy path + --json: orphan FR-2 appears in the JSON output.
func TestRunAnalyze_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	plan := filepath.Join(dir, "plan.md")
	if err := os.WriteFile(spec, []byte("**FR-1.** X\n**FR-2.** Y\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plan, []byte("### Task 1\n_Requirements: FR-1_\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if err := runAnalyze(spec, plan, true, os.ReadFile, &out, &errb); err != nil {
		t.Fatalf("runAnalyze: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "orphan-fr") || !strings.Contains(s, "FR-2") {
		t.Errorf("JSON output missing orphan-fr FR-2: %s", s)
	}
}

// Human output states the finding count.
func TestRunAnalyze_HumanOutput(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	plan := filepath.Join(dir, "plan.md")
	_ = os.WriteFile(spec, []byte("## Brief\n1. a\n**FR-1.** X\n"), 0o600)
	_ = os.WriteFile(plan, []byte("### Task 1\n_Requirements: FR-1_\n"), 0o600)
	var out, errb bytes.Buffer
	if err := runAnalyze(spec, plan, false, os.ReadFile, &out, &errb); err != nil {
		t.Fatalf("runAnalyze: %v", err)
	}
	if !strings.Contains(strings.ToLower(out.String()), "brief") {
		t.Errorf("human output missing Brief line: %s", out.String())
	}
}
