package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

func TestUIVerifyCmd_Registered(t *testing.T) {
	root := NewRootCmd()
	var found bool
	for _, c := range root.Commands() {
		if c.Name() == "ui-verify" {
			found = true
		}
	}
	if !found {
		t.Fatal("`ui-verify` command not registered on root")
	}
}

// initUIVerifyRepo creates a TempDir git repo with one commit (a
// non-rendering file), so tests exercise the real production RunGit against
// a real git binary.
func initUIVerifyRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...) //nolint:gosec // G204 -- test fixture, fixed argv
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com")
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out.String())
		}
	}
	run("init")
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("add", "main.go")
	run("commit", "-m", "init")
	return repo
}

func runUIVerifyCheck(repo, base string) (string, error) {
	c := newUIVerifyCheckCmd()
	buf := &bytes.Buffer{}
	c.SetOut(buf)
	c.SetErr(buf)
	c.SetArgs([]string{"--repo", repo, "--base", base})
	err := c.Execute()
	return buf.String(), err
}

func TestUIVerifyCheck_NoRenderChanges_ExitZero(t *testing.T) {
	repo := initUIVerifyRepo(t)
	out, err := runUIVerifyCheck(repo, "HEAD")
	if err != nil {
		t.Fatalf("expected exit 0, got err=%v (out=%s)", err, out)
	}
	if !strings.Contains(out, "not_required") {
		t.Errorf("expected not_required state in output, got %q", out)
	}
}

func TestUIVerifyCheck_RenderChangedNoArtifact_Fail(t *testing.T) {
	repo := initUIVerifyRepo(t)
	compDir := filepath.Join(repo, "src", "components")
	if err := os.MkdirAll(compDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(compDir, "Foo.tsx"), []byte("export {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := runUIVerifyCheck(repo, "HEAD")
	if err == nil {
		t.Fatal("expected an error when a rendering file changed with no recorded artifact")
	}
	if exitcode.Code(err) != exitcode.Fail {
		t.Errorf("expected exitcode.Fail, got %d (%v)", exitcode.Code(err), err)
	}
}

func TestUIVerifyCheck_MissingBase_BadArgs(t *testing.T) {
	repo := initUIVerifyRepo(t)
	c := newUIVerifyCheckCmd()
	buf := &bytes.Buffer{}
	c.SetOut(buf)
	c.SetErr(buf)
	c.SetArgs([]string{"--repo", repo})
	err := c.Execute()
	if err == nil {
		t.Fatal("expected an error when --base is missing")
	}
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Errorf("expected exitcode.BadArgs, got %d (%v)", exitcode.Code(err), err)
	}
}

// TestRealRunGit_StdinPiping is the load-bearing contract: the trailing arg
// after "--stdin" must be piped to the git process's stdin (not passed as an
// argv pathspec), and the returned hash must be untrimmed raw stdout apart
// from what git itself outputs — proven here by cross-checking against a
// direct `git hash-object --stdin` invocation with the same content on
// stdin.
func TestRealRunGit_StdinPiping(t *testing.T) {
	repo := initUIVerifyRepo(t)
	content := "hello uiverify\nsecond line\n"

	got, err := realRunGit(repo, "hash-object", "--stdin", content)
	if err != nil {
		t.Fatalf("realRunGit hash-object --stdin: %v", err)
	}

	cmd := exec.Command("git", "-C", repo, "hash-object", "--stdin") //nolint:gosec // G204 -- test fixture
	cmd.Stdin = strings.NewReader(content)
	want, err := cmd.Output()
	if err != nil {
		t.Fatalf("reference git hash-object --stdin: %v", err)
	}
	if got != string(want) {
		t.Errorf("realRunGit hash-object output = %q, want %q (content must be piped to stdin, not passed as argv)", got, string(want))
	}
}

// TestRealRunGit_RawStdoutNoTrim proves rev-parse output keeps its trailing
// newline — Fingerprint relies on concatenating raw RunGit outputs.
func TestRealRunGit_RawStdoutNoTrim(t *testing.T) {
	repo := initUIVerifyRepo(t)
	out, err := realRunGit(repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("realRunGit rev-parse HEAD: %v", err)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("expected raw stdout with trailing newline, got %q", out)
	}
}
