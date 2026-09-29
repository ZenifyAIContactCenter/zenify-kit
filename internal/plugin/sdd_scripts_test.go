package plugin

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
