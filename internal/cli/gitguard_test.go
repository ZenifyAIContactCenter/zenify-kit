package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/devenv"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitguard"
)

func gitInit(t *testing.T, dir, branch string, deny []string) {
	t.Helper()
	run := func(a ...string) {
		c := exec.Command("git", a...) //nolint:gosec // G204 -- fixed trusted binary, args are internally-computed subcommands, not attacker-controlled shell input
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", a, err, out)
		}
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	run("init", "-q", "-b", branch)
	run("config", "user.email", "t@e.com")
	run("config", "user.name", "T")
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("add", "f")
	run("commit", "-q", "-m", "init")
	if deny != nil {
		if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o750); err != nil {
			t.Fatal(err)
		}
		body := ""
		for _, b := range deny {
			body += b + "\n"
		}
		if err := os.WriteFile(filepath.Join(dir, ".claude", "deploy-branches"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// isolateHome keeps a hook test off the developer's real workspace: no HOME
// pointer file and no ZENIFY_HOME override can resolve to it.
func isolateHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ZENIFY_HOME", t.TempDir())
}

func TestDecideFromPayload(t *testing.T) {
	isolateHome(t)
	repo := filepath.Join(t.TempDir(), "r")
	gitInit(t, repo, "main", []string{"main"})
	getenv := func(string) string { return "" }

	deny := `{"cwd":"` + repo + `","tool_input":{"command":"git commit -m x"}}`
	if !decideFromPayload([]byte(deny), getenv).Deny {
		t.Error("commit on main must deny")
	}
	allow := `{"cwd":"` + repo + `","tool_input":{"command":"git status"}}`
	if decideFromPayload([]byte(allow), getenv).Deny {
		t.Error("git status must allow")
	}
	// Fail-open: broken JSON → allow.
	if decideFromPayload([]byte("{not json"), getenv).Deny {
		t.Error("broken JSON must fail-open (allow)")
	}
}

// Degenerate input must not hang/panic (port of test-hooks-hang.sh).
func TestDecideDegenerate(t *testing.T) {
	isolateHome(t)
	t.Chdir(t.TempDir()) // cwd "." must not walk up into the real workspace
	getenv := func(string) string { return "" }
	for _, cmd := range []string{"sudo", "env", "-", "git", "cd", "cd &&", "git -C", "git -C -C -C push", "cd cd cd && git push"} {
		p := `{"cwd":".","tool_input":{"command":"` + cmd + `"}}`
		_ = decideFromPayload([]byte(p), getenv) // only needs to not panic/hang
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
		t.Fatalf("panic must fail-open (exit 0), got exit %d", code)
	}
	if strings.Contains(stderr.String(), "boom") {
		t.Fatalf("stderr must not leak panic details (payload/secret): %q", stderr.String())
	}
}

func TestGitGuardCmdExit2(t *testing.T) {
	home, zhome := t.TempDir(), t.TempDir()
	isoEnv := append(os.Environ(), "HOME="+home, "ZENIFY_HOME="+zhome)
	repo := filepath.Join(t.TempDir(), "r")
	gitInit(t, repo, "main", []string{"main"})
	bin := filepath.Join(t.TempDir(), "zenify")
	if out, err := exec.Command("go", "build", "-o", bin, "../../cmd/zenify").CombinedOutput(); err != nil { //nolint:gosec // G204 -- fixed trusted binary, args are internally-computed subcommands, not attacker-controlled shell input
		t.Fatalf("build: %v\n%s", err, out)
	}
	c := exec.Command(bin, "git-guard") //nolint:gosec // G204 -- fixed trusted binary, args are internally-computed subcommands, not attacker-controlled shell input
	c.Env = isoEnv
	c.Stdin = strings.NewReader(`{"cwd":"` + repo + `","tool_input":{"command":"git commit -m x"}}`)
	err := c.Run()
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 2 {
		t.Fatalf("must exit 2 (deny), got %v", err)
	}
	for cmdline, want := range map[string]int{"gh pr merge 1": 2, "gh pr create --title t": 0} {
		c := exec.Command(bin, "git-guard") //nolint:gosec // G204 -- fixed trusted binary, args are internally-computed subcommands, not attacker-controlled shell input
		c.Env = isoEnv
		c.Stdin = strings.NewReader(`{"cwd":"` + repo + `","tool_input":{"command":"` + cmdline + `"}}`)
		got := 0
		if err := c.Run(); err != nil {
			ee, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatalf("%q: %v", cmdline, err)
			}
			got = ee.ExitCode()
		}
		if got != want {
			t.Errorf("%q: exit %d, want %d", cmdline, got, want)
		}
	}
}

const fakeProdEnv = "MONGO_URL_PRD='mongodb://u:p@Prod-Host.example:27017/db'\n" +
	"REDIS_URL_PRD='redis://127.0.0.1:6379'\n" +
	"QDRANT_URL_PRD=''\n" +
	"MONGO_URL='mongodb://staging-host/db'\n" +
	"BAD_PRD='not a url'\n"

func TestProdHosts(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(p, []byte(fakeProdEnv+"DUP_PRD='mongodb://PROD-host.example/x'\nLOCAL_PRD='http://localhost:1'\nV6_PRD='http://[::1]:1'\nZ_PRD='http://0.0.0.0:1'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := devenv.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	got := prodHosts(f)
	want := []gitguard.ProdHost{{Key: "MONGO_URL_PRD", Host: "prod-host.example"}}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("prodHosts = %v, want %v", got, want)
	}
}

func TestDecideFromPayload_ProdHost(t *testing.T) {
	isolateHome(t)
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".zenify"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".zenify", "manifest.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".zenify", ".env"), []byte(fakeProdEnv), 0o600); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(ws, "repos", "r")
	gitInit(t, repo, "feat/x", nil)
	getenv := func(string) string { return "" }
	run := func(cwd, cmd string) gitguard.Decision {
		return decideFromPayload([]byte(`{"cwd":"`+cwd+`","tool_input":{"command":"`+cmd+`"}}`), getenv)
	}
	d := run(repo, "mongosh mongodb://prod-host.example/db")
	if !d.Deny {
		t.Fatal("prod host must deny")
	}
	if strings.Contains(d.Message, "prod-host.example") {
		t.Errorf("message leaks host: %q", d.Message)
	}
	if run(repo, "mongosh mongodb://staging-host/db").Deny {
		t.Error("staging host must allow")
	}
	if !run(ws, "cat .zenify/.env").Deny {
		t.Error("reading the env file must deny")
	}
}

// TestDecideFromPayload_SecretsIntegration closes the Task 2 review carry-forward:
// Staged has no direct test coverage. This wires onCommit (built inside
// decideFromPayload) through a real git repo with a real staged secret, and
// asserts the returned message never contains the raw secret string.
func TestDecideFromPayload_SecretsIntegration(t *testing.T) {
	isolateHome(t)
	getenv := func(string) string { return "" }
	// Split so no contiguous 20-char AKIA... literal sits in source (the
	// repo's own `secret-scan .` CI step would otherwise flag this file);
	// the concatenation still equals the full key at runtime.
	const secret = "AKIA" + "QWERTYUIOPASDFGH"

	t.Run("deny on staged secret", func(t *testing.T) {
		repo := filepath.Join(t.TempDir(), "r")
		gitInit(t, repo, "work", nil) // no deploy-branches file: commit itself is allowed on this branch
		_ = os.WriteFile(filepath.Join(repo, "creds.txt"), []byte("aws_key = \""+secret+"\"\n"), 0o600)
		run := func(a ...string) {
			c := exec.Command("git", a...) //nolint:gosec // G204 -- fixed trusted binary, args are internally-computed subcommands, not attacker-controlled shell input
			c.Dir = repo
			if out, err := c.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", a, err, out)
			}
		}
		run("add", "creds.txt")

		p := `{"cwd":"` + repo + `","tool_input":{"command":"git commit -m x"}}`
		d := decideFromPayload([]byte(p), getenv)
		if !d.Deny {
			t.Fatal("staged secret must deny")
		}
		if strings.Contains(d.Message, secret) {
			t.Fatalf("deny message contains the raw secret: %q", d.Message)
		}
	})

	t.Run("allow on clean staged file", func(t *testing.T) {
		repo := filepath.Join(t.TempDir(), "r")
		gitInit(t, repo, "work", nil)
		_ = os.WriteFile(filepath.Join(repo, "readme.txt"), []byte("hello world\n"), 0o600)
		run := func(a ...string) {
			c := exec.Command("git", a...) //nolint:gosec // G204 -- fixed trusted binary, args are internally-computed subcommands, not attacker-controlled shell input
			c.Dir = repo
			if out, err := c.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", a, err, out)
			}
		}
		run("add", "readme.txt")

		p := `{"cwd":"` + repo + `","tool_input":{"command":"git commit -m x"}}`
		d := decideFromPayload([]byte(p), getenv)
		if d.Deny {
			t.Fatalf("clean staged file must allow, got deny: %q", d.Message)
		}
	})
}
