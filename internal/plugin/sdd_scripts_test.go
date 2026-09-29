package plugin

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// runSDDScript copies the three SDD scripts into a temp dir with mode 0600 (what
// sync.go installs) and runs the named one through bash with Dir=dir. It returns
// stdout+stderr and the exit code, and never fails the test on a non-zero exit.
func runSDDScript(t *testing.T, dir, name string, args ...string) (stdout string, exit int) {
	t.Helper()
	src := filepath.Join("assets", "znf", "skills", "subagent-driven-development", "scripts")
	scratch := t.TempDir()
	for _, n := range []string{"sdd-workspace", "task-brief", "review-package"} {
		b, err := os.ReadFile(filepath.Join(src, n))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(scratch, n), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("bash", append([]string{filepath.Join(scratch, name)}, args...)...)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return buf.String(), ee.ExitCode()
	}
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return buf.String(), 0
}

// newTempRepo makes a git repo with one commit and returns its path.
func newTempRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-q", "-m", "c1")
	return dir
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-c", "user.email=t@example.com", "-c", "user.name=t", "-c", "commit.gpgsign=false"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestSDDScripts_Run0600(t *testing.T) {
	repo := newTempRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "plan.md"), []byte("### Task 1: x\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sha1 := gitIn(t, repo, "rev-parse", "HEAD")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-q", "-m", "c2")
	sha2 := gitIn(t, repo, "rev-parse", "HEAD")

	cases := [][]string{
		{"sdd-workspace", "plan.md"},
		{"task-brief", "plan.md", "1"},
		{"review-package", "plan.md", sha1, sha2},
	}
	for _, c := range cases {
		out, exit := runSDDScript(t, repo, c[0], c[1:]...)
		if exit != 0 || strings.Contains(out, "Permission denied") {
			t.Errorf("%s: exit=%d out=%q", c[0], exit, out)
		}
	}
}

func writePlan(t *testing.T, repo, rel string) {
	t.Helper()
	p := filepath.Join(repo, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("### Task 1: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReviewPackage_NotAncestorExit3(t *testing.T) {
	repo := newTempRepo(t)
	writePlan(t, repo, "plan.md")
	main := gitIn(t, repo, "rev-parse", "--abbrev-ref", "HEAD")
	gitIn(t, repo, "checkout", "-q", "-b", "other")
	gitIn(t, repo, "commit", "-q", "--allow-empty", "-m", "o1")
	other := gitIn(t, repo, "rev-parse", "HEAD")
	gitIn(t, repo, "checkout", "-q", main)
	gitIn(t, repo, "commit", "-q", "--allow-empty", "-m", "m1")
	head := gitIn(t, repo, "rev-parse", "HEAD")
	out, exit := runSDDScript(t, repo, "review-package", "plan.md", other, head)
	if exit != 3 || !strings.Contains(out, "HEAD is not a descendant of BASE") {
		t.Errorf("exit=%d out=%q", exit, out)
	}
}

func TestReviewPackage_EmptyRangeExit3(t *testing.T) {
	repo := newTempRepo(t)
	writePlan(t, repo, "plan.md")
	head := gitIn(t, repo, "rev-parse", "HEAD")
	out, exit := runSDDScript(t, repo, "review-package", "plan.md", head, head)
	if exit != 3 || !strings.Contains(out, "empty commit range") {
		t.Errorf("exit=%d out=%q", exit, out)
	}
}

func TestSDDWorkspace_BasenameCollision(t *testing.T) {
	repo := newTempRepo(t)
	writePlan(t, repo, "a/plan.md")
	writePlan(t, repo, "b/plan.md")
	a, e1 := runSDDScript(t, repo, "sdd-workspace", "a/plan.md")
	b, e2 := runSDDScript(t, repo, "sdd-workspace", "b/plan.md")
	a2, e3 := runSDDScript(t, repo, "sdd-workspace", "a/plan.md")
	if e1 != 0 || e2 != 0 || e3 != 0 {
		t.Fatalf("exits %d %d %d: %q %q %q", e1, e2, e3, a, b, a2)
	}
	a, b, a2 = strings.TrimSpace(a), strings.TrimSpace(b), strings.TrimSpace(a2)
	if a == b {
		t.Fatalf("collision: both %q", a)
	}
	if a2 != a {
		t.Errorf("a/plan.md moved: %q -> %q", a, a2)
	}
	for dir, want := range map[string]string{a: "a/plan.md", b: "b/plan.md"} {
		got, err := os.ReadFile(filepath.Join(dir, "plan-path"))
		if err != nil || strings.TrimSpace(string(got)) != want {
			t.Errorf("%s/plan-path = %q, %v; want %q", dir, got, err, want)
		}
	}
}

func TestSDDWorkspace_AdoptsLegacy(t *testing.T) {
	repo := newTempRepo(t)
	writePlan(t, repo, "plan.md")
	legacy := filepath.Join(repo, ".znf", "sdd", "plan")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	out, exit := runSDDScript(t, repo, "sdd-workspace", "plan.md")
	if exit != 0 {
		t.Fatalf("exit=%d out=%q", exit, out)
	}
	gotDir, _ := filepath.EvalSymlinks(strings.TrimSpace(out))
	wantDir, _ := filepath.EvalSymlinks(legacy)
	if gotDir != wantDir {
		t.Errorf("dir = %q, want legacy %q", gotDir, wantDir)
	}
	got, err := os.ReadFile(filepath.Join(legacy, "plan-path"))
	if err != nil || strings.TrimSpace(string(got)) != "plan.md" {
		t.Errorf("marker = %q, %v", got, err)
	}
}

func TestSDDWorkspace_UnwritableExitsNonZero(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory modes")
	}
	repo := newTempRepo(t)
	sdd := filepath.Join(repo, ".znf", "sdd")
	if err := os.MkdirAll(sdd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "plan.md"), []byte("p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sdd, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sdd, 0o755) })
	type res struct{ exit int }
	ch := make(chan res, 1)
	go func() {
		_, e := runSDDScript(t, repo, "sdd-workspace", "plan.md")
		ch <- res{e}
	}()
	select {
	case r := <-ch:
		if r.exit == 0 {
			t.Error("want non-zero exit for unwritable .znf/sdd")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("sdd-workspace looped instead of failing")
	}
}
