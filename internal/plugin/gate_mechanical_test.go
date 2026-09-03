package plugin

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// gitInit dựng một repo tạm với 1 commit nền, trả về đường repo và BASE sha.
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
	return dir, base[:len(base)-1] // bỏ newline
}

// gitCommitAll add + commit mọi thay đổi hiện tại trong repo tạm — cần vì gate dùng
// `git diff "$BASE"` (chỉ thấy thay đổi ĐÃ COMMIT/tracked, không thấy file untracked).
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
		t.Fatalf("gate stdout không phải JSON: %v (%q)", err, string(out))
	}
	return res
}

func TestGate_CleanPasses(t *testing.T) {
	dir, base := gitInit(t)
	if err := os.WriteFile(filepath.Join(dir, "ok.txt"), []byte("dòng sạch\n"), 0o600); err != nil {
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

// SC-02: STATIC_OK=1 bỏ qua build → một go.mod hỏng vẫn pass (build không chạy), không có "build fail".
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
		t.Errorf("verdict=%q, want pass (STATIC_OK phải bỏ build)", res.Verdict)
	}
	for _, f := range res.Findings {
		if f["title"] == "build fail" {
			t.Errorf("gặp finding build fail dù STATIC_OK=1 (build đáng ra bị bỏ)")
		}
	}
}

// SC-01: build fail → block. Cần go trên PATH; broken.go làm `go build ./...` fail.
func TestGate_BuildFailBlocks(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go không có trên PATH")
	}
	dir, base := gitInit(t)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module tmptest\n\ngo 1.21\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.go"), []byte("package main\nfunc main() { this is not go }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, dir)
	res := runGate(t, dir, base) // STATIC_OK mặc định 0 → chạy build
	if res.Verdict != "block" {
		t.Errorf("verdict=%q, want block (build fail)", res.Verdict)
	}
}

// FR-03: focused test (.only) + debugger trong diff → có finding (HIGH, không block).
func TestGate_FocusedTestAndDebugger(t *testing.T) {
	dir, base := gitInit(t)
	body := "test.only('x', () => { debugger; });\n"
	if err := os.WriteFile(filepath.Join(dir, "a.test.js"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, dir)
	res := runGate(t, dir, base, "STATIC_OK=1") // bỏ build; chỉ quét anti-pattern
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
		t.Errorf("focused=%v debugger=%v, want cả hai true", focused, dbg)
	}
}
