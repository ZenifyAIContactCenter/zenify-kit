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

func TestRunE2eLint_NoSpecs_BadArgs(t *testing.T) {
	dir := t.TempDir()
	err := runE2eLint(dir, &bytes.Buffer{})
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Fatalf("missing spec should be BadArgs, got %v", err)
	}
}

func TestRunE2eLint_Violations_Fail(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "x.spec.ts"),
		[]byte("test('x', async ({page}) => { await page.waitForTimeout(1); });"), 0o644)
	err := runE2eLint(dir, &bytes.Buffer{})
	if exitcode.Code(err) != exitcode.Fail {
		t.Fatalf("violation should be Fail, got %v", err)
	}
}

func TestRunE2eLint_Clean_OK(t *testing.T) {
	dir := t.TempDir()
	clean := `test('ok [FR-1]', async ({page, apiClient, cleanupTracker}) => {
  await page.getByRole('button').click();
  // @domain-assert:ticket
  const r = await apiClient.get('/v2/ticket/1'); expect((await r.json()).subject).toBe('x');
  cleanupTracker.add(async () => {});
});`
	os.WriteFile(filepath.Join(dir, "ok.spec.ts"), []byte(clean), 0o644)
	if err := runE2eLint(dir, &bytes.Buffer{}); err != nil {
		t.Fatalf("clean spec should be OK, got %v", err)
	}
}

func TestE2eLogin_NeedsExactlyOneOfUrlPort(t *testing.T) {
	for _, args := range [][]string{{"login"}, {"login", "--url", "x", "--port", "1"}} {
		c := newE2eCmd()
		c.SetArgs(args)
		c.SetOut(&bytes.Buffer{})
		c.SetErr(&bytes.Buffer{})
		if err := c.Execute(); exitcode.Code(err) != exitcode.BadArgs {
			t.Fatalf("%v: want BadArgs, got %v", args, err)
		}
	}
}

func TestE2eLogin_PortOutOfRange(t *testing.T) {
	for _, p := range []string{"-1", "70000"} {
		c := newE2eCmd()
		c.SetArgs([]string{"login", "--port", p})
		c.SetOut(&bytes.Buffer{})
		c.SetErr(&bytes.Buffer{})
		if err := c.Execute(); exitcode.Code(err) != exitcode.BadArgs {
			t.Fatalf("--port %s: want BadArgs, got %v", p, err)
		}
	}
}

// SC-9 / FR-7.2: E2E_* that exist only in the credential file reach the child's Env.
func TestE2ERunner_InjectsE2EEnv(t *testing.T) {
	for _, k := range e2eEnvKeys {
		t.Setenv(k, "")
	}
	envFile := filepath.Join(t.TempDir(), ".zenify", ".env")
	if err := os.MkdirAll(filepath.Dir(envFile), 0o700); err != nil {
		t.Fatal(err)
	}
	body := "E2E_DOMAIN=fake.example\nE2E_EMAIL=fake@example.com\nE2E_PASSWORD='fake-pw'\n"
	if err := os.WriteFile(envFile, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	lookup := credLookup(envFile, os.Getenv)
	if got := lookup("E2E_PASSWORD"); got != "fake-pw" {
		t.Fatalf("lookup = %q, want the file value", got)
	}
	c := childCmd("docker", []string{"run"}, []string{"BASE_URL=http://localhost:1"}, lookup)
	last := map[string]string{}
	for _, kv := range c.Env {
		if k, v, ok := strings.Cut(kv, "="); ok {
			last[k] = v // exec keeps the last duplicate
		}
	}
	if last["E2E_PASSWORD"] != "fake-pw" || last["E2E_EMAIL"] != "fake@example.com" || last["E2E_DOMAIN"] != "fake.example" {
		t.Errorf("child Env lacks the file's E2E_* values")
	}
	if last["BASE_URL"] != "http://localhost:1" {
		t.Errorf("extraEnv dropped: BASE_URL=%q", last["BASE_URL"])
	}
}

// Live env wins over the file.
func TestE2ERunner_EnvWinsOverFile(t *testing.T) {
	t.Setenv("E2E_PASSWORD", "from-env")
	envFile := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envFile, []byte("E2E_PASSWORD=from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := credLookup(envFile, os.Getenv)("E2E_PASSWORD"); got != "from-env" {
		t.Fatalf("lookup = %q, want live env to win", got)
	}
}

// E2E_* reach only the child that runs the journey, never an install step.
func TestChildCmd_CredsOnlyForJourney(t *testing.T) {
	for _, k := range e2eEnvKeys {
		t.Setenv(k, "")
	}
	lookup := func(k string) string { return "fake-" + k }
	envOf := func(c *exec.Cmd) map[string]string {
		last := map[string]string{}
		for _, kv := range c.Env {
			if k, v, ok := strings.Cut(kv, "="); ok {
				last[k] = v
			}
		}
		return last
	}
	for _, tc := range []struct {
		name string
		args []string
		want bool
	}{
		{"npm", []string{"install", "--no-audit", "--no-fund", "--silent"}, false},
		{"npx", []string{"playwright", "install", "chromium"}, false},
		{"npx", []string{"playwright", "test", "--project=setup"}, true},
		{"docker", []string{"run", "--rm", "img"}, true},
	} {
		got := envOf(childCmd(tc.name, tc.args, nil, lookup))["E2E_PASSWORD"] != ""
		if got != tc.want {
			t.Errorf("%s %v: has E2E_PASSWORD=%v, want %v", tc.name, tc.args, got, tc.want)
		}
	}
}

// A shell-exported E2E_* never reaches an install child either.
func TestChildCmd_StripsShellCredsFromInstall(t *testing.T) {
	t.Setenv("E2E_PASSWORD", "from-shell")
	lookup := credLookup("", os.Getenv)
	has := func(c *exec.Cmd) bool {
		for _, kv := range c.Env {
			if strings.HasPrefix(kv, "E2E_PASSWORD=") {
				return true
			}
		}
		return false
	}
	if has(childCmd("npm", []string{"install"}, nil, lookup)) {
		t.Errorf("npm install child carries E2E_PASSWORD")
	}
	if has(childCmd("npx", []string{"playwright", "install", "chromium"}, []string{"E2E_PASSWORD=x"}, lookup)) {
		t.Errorf("playwright install child carries E2E_PASSWORD")
	}
	if !has(childCmd("npx", []string{"playwright", "test"}, nil, lookup)) {
		t.Errorf("journey child lost E2E_PASSWORD")
	}
}
