package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitguard"
)

func gitInit(t *testing.T, dir, branch string, deny []string) {
	t.Helper()
	run := func(a ...string) {
		c := exec.Command("git", a...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", a, err, out)
		}
	}
	os.MkdirAll(dir, 0o755)
	run("init", "-q", "-b", branch)
	run("config", "user.email", "t@e.com")
	run("config", "user.name", "T")
	os.WriteFile(filepath.Join(dir, "f"), []byte("x\n"), 0o644)
	run("add", "f")
	run("commit", "-q", "-m", "init")
	if deny != nil {
		os.MkdirAll(filepath.Join(dir, ".claude"), 0o755)
		body := ""
		for _, b := range deny {
			body += b + "\n"
		}
		os.WriteFile(filepath.Join(dir, ".claude", "deploy-branches"), []byte(body), 0o644)
	}
}

func TestDecideFromPayload(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "r")
	gitInit(t, repo, "main", []string{"main"})
	getenv := func(string) string { return "" }

	deny := `{"cwd":"` + repo + `","tool_input":{"command":"git commit -m x"}}`
	if !decideFromPayload([]byte(deny), getenv).Deny {
		t.Error("commit trên main phải deny")
	}
	allow := `{"cwd":"` + repo + `","tool_input":{"command":"git status"}}`
	if decideFromPayload([]byte(allow), getenv).Deny {
		t.Error("git status phải allow")
	}
	// Fail-open: JSON hỏng → allow.
	if decideFromPayload([]byte("{not json"), getenv).Deny {
		t.Error("JSON hỏng phải fail-open (allow)")
	}
}

// Degenerate input không được treo/panic (port test-hooks-hang.sh).
func TestDecideDegenerate(t *testing.T) {
	getenv := func(string) string { return "" }
	for _, cmd := range []string{"sudo", "env", "-", "git", "cd", "cd &&", "git -C", "git -C -C -C push", "cd cd cd && git push"} {
		p := `{"cwd":".","tool_input":{"command":"` + cmd + `"}}`
		_ = decideFromPayload([]byte(p), getenv) // chỉ cần không panic/treo
	}
}

// TestRunGitGuardPanicFailsOpen closes the fail-open invariant: a panic
// anywhere inside decide (gitguard.Decide / secretscan) must never surface
// as exit code 2 (a bare panic's default exit code, indistinguishable from
// a deliberate DENY to the hook contract). It must fall open to allow (0).
// The `decide` parameter of runGitGuard is a test-only injection seam — the
// production call site (newGitGuardCmd's RunE) always passes decideFromPayload.
func TestRunGitGuardPanicFailsOpen(t *testing.T) {
	panicky := func([]byte, func(string) string) gitguard.Decision {
		panic("boom: simulated internal panic")
	}
	var stderr strings.Builder
	code := runGitGuard(
		strings.NewReader(`{"cwd":".","tool_input":{"command":"git commit -m x"}}`),
		&stderr,
		func(string) string { return "" },
		panicky,
	)
	if code != 0 {
		t.Fatalf("panic phải fail-open (exit 0), được exit %d", code)
	}
	if strings.Contains(stderr.String(), "boom") {
		t.Fatalf("stderr không được lộ chi tiết panic (payload/secret): %q", stderr.String())
	}
}

func TestGitGuardCmdExit2(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "r")
	gitInit(t, repo, "main", []string{"main"})
	bin := filepath.Join(t.TempDir(), "zenify")
	if out, err := exec.Command("go", "build", "-o", bin, "../../cmd/zenify").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	c := exec.Command(bin, "git-guard")
	c.Stdin = strings.NewReader(`{"cwd":"` + repo + `","tool_input":{"command":"git commit -m x"}}`)
	err := c.Run()
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 2 {
		t.Fatalf("phải exit 2 (deny), được %v", err)
	}
}

// TestDecideFromPayload_SecretsIntegration closes the Task 2 review carry-forward:
// Staged has no direct test coverage. This wires onCommit (built inside
// decideFromPayload) through a real git repo with a real staged secret, and
// asserts the returned message never contains the raw secret string.
func TestDecideFromPayload_SecretsIntegration(t *testing.T) {
	getenv := func(string) string { return "" }
	const secret = "AKIAQWERTYUIOPASDFGH"

	t.Run("deny on staged secret", func(t *testing.T) {
		repo := filepath.Join(t.TempDir(), "r")
		gitInit(t, repo, "work", nil) // no deploy-branches file: commit itself is allowed on this branch
		os.WriteFile(filepath.Join(repo, "creds.txt"), []byte("aws_key = \""+secret+"\"\n"), 0o644)
		run := func(a ...string) {
			c := exec.Command("git", a...)
			c.Dir = repo
			if out, err := c.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", a, err, out)
			}
		}
		run("add", "creds.txt")

		p := `{"cwd":"` + repo + `","tool_input":{"command":"git commit -m x"}}`
		d := decideFromPayload([]byte(p), getenv)
		if !d.Deny {
			t.Fatal("staged secret phải deny")
		}
		if strings.Contains(d.Message, secret) {
			t.Fatalf("deny message chứa raw secret: %q", d.Message)
		}
	})

	t.Run("allow on clean staged file", func(t *testing.T) {
		repo := filepath.Join(t.TempDir(), "r")
		gitInit(t, repo, "work", nil)
		os.WriteFile(filepath.Join(repo, "readme.txt"), []byte("hello world\n"), 0o644)
		run := func(a ...string) {
			c := exec.Command("git", a...)
			c.Dir = repo
			if out, err := c.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", a, err, out)
			}
		}
		run("add", "readme.txt")

		p := `{"cwd":"` + repo + `","tool_input":{"command":"git commit -m x"}}`
		d := decideFromPayload([]byte(p), getenv)
		if d.Deny {
			t.Fatalf("clean staged file phải allow, được deny: %q", d.Message)
		}
	})
}
