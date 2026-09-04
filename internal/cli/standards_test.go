package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRunStandards_ReportsUntestedFR(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	plan := filepath.Join(dir, "plan.md")
	os.WriteFile(spec, []byte("**FR-1.** x.\n"), 0o644)
	os.WriteFile(plan, []byte("### Task 1: X\n**Files:**\n- Create: `a.go`\n- `_Requirements: FR-1_`\n"), 0o644)
	var out, errb bytes.Buffer
	if err := runStandards(spec, plan, dir, false, os.ReadFile, &out, &errb); err != nil {
		t.Fatalf("fail-open violated: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("untested-fr")) {
		t.Fatalf("expected untested-fr in output, got: %s", out.String())
	}
}

func TestRunStandards_FailOpen_OnUnreadableInputs(t *testing.T) {
	var out, errb bytes.Buffer
	// both paths unreadable — must still return nil (exit 0), no panic.
	if err := runStandards("/no/spec.md", "/no/plan.md", ".", false, os.ReadFile, &out, &errb); err != nil {
		t.Fatalf("fail-open violated: %v", err)
	}
}

func TestRunStandards_JSON(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	plan := filepath.Join(dir, "plan.md")
	os.WriteFile(spec, []byte("**FR-1.** x.\n"), 0o644)
	os.WriteFile(plan, []byte("### Task 1: X\n**Files:**\n- Test: `missing_test.go`\n- `_Requirements: FR-1_`\n"), 0o644)
	var out, errb bytes.Buffer
	if err := runStandards(spec, plan, dir, true, os.ReadFile, &out, &errb); err != nil {
		t.Fatalf("fail-open violated: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte(`"findings"`)) {
		t.Fatalf("expected JSON with findings, got: %s", out.String())
	}
}
