package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/e2e"
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

// hermeticE2E keeps an e2e/visual command test away from real accounts and the
// real workspace: cwd, ZENIFY_HOME and HOME are empty temp dirs, E2E_* is blank,
// and cwd holds manifest/repos.yaml (copied from manifestPath when given, else
// a manifest with no broker) so the broker lookup never reads the embedded one.
func hermeticE2E(t *testing.T, manifestPath string) {
	t.Helper()
	cwd := t.TempDir()
	t.Chdir(cwd)
	t.Setenv("ZENIFY_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	for _, k := range e2eEnvKeys {
		t.Setenv(k, "")
	}
	if manifestPath == "" {
		manifestPath = noBrokerManifest(t)
	}
	b, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("manifest", 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("manifest", "repos.yaml"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func e2eAccountHandler(t *testing.T, gotRole *string) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{"/e2e/account": func(w http.ResponseWriter, r *http.Request) {
		var req api.E2EAccountRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		*gotRole = req.Role
		_ = json.NewEncoder(w).Encode(api.E2EAccountResponse{
			Domain: "e2e.test.example", Email: req.Role + "@test.example", Password: "test-pass", Role: req.Role})
	}}
}

func envMap(c *exec.Cmd) map[string]string {
	m := map[string]string{}
	for _, kv := range c.Env {
		if k, v, ok := strings.Cut(kv, "="); ok {
			m[k] = v
		}
	}
	return m
}

// fakeLoginChild stands in for npm/npx: the setup project writes STORAGE_STATE.
func fakeLoginChild(t *testing.T, sawEnv *map[string]string) {
	t.Helper()
	orig := execChild
	t.Cleanup(func() { execChild = orig })
	execChild = func(c *exec.Cmd) error {
		e := envMap(c)
		if e["STORAGE_STATE"] != "" {
			*sawEnv = e
			return os.WriteFile(e["STORAGE_STATE"], []byte("{}"), 0o600)
		}
		return nil
	}
}

func runLogin(t *testing.T, args ...string) (string, error) {
	t.Helper()
	out, _, err := execCmd(newE2eCmd(), append([]string{"login", "--port", "3327"}, args...)...)
	return out, err
}

func TestE2eLogin_BrokerDefaultRoleStateJSON(t *testing.T) {
	var role string
	mp, _ := brokerFixture(t, e2eAccountHandler(t, &role))
	hermeticE2E(t, mp)
	var saw map[string]string
	fakeLoginChild(t, &saw)
	if _, err := runLogin(t); err != nil {
		t.Fatal(err)
	}
	if role != "administrator" {
		t.Errorf("broker got role %q, want administrator", role)
	}
	if _, err := os.Stat(e2e.StatePath(os.Getenv("HOME"))); err != nil {
		t.Errorf("state.json not written: %v", err)
	}
	if saw["E2E_EMAIL"] != "administrator@test.example" || saw["E2E_PASSWORD"] != "test-pass" {
		t.Errorf("login child lacks the broker credentials")
	}
}

func TestE2eLogin_BrokerRoleState(t *testing.T) {
	var role string
	mp, _ := brokerFixture(t, e2eAccountHandler(t, &role))
	hermeticE2E(t, mp)
	var saw map[string]string
	fakeLoginChild(t, &saw)
	if _, err := runLogin(t, "--role", "agent"); err != nil {
		t.Fatal(err)
	}
	if role != "agent" {
		t.Errorf("broker got role %q, want agent", role)
	}
	home := os.Getenv("HOME")
	if _, err := os.Stat(filepath.Join(home, ".zenify", "playwright", "state-agent.json")); err != nil {
		t.Errorf("state-agent.json not written: %v", err)
	}
	if _, err := os.Stat(e2e.StatePath(home)); err == nil {
		t.Errorf("state.json must stay untouched for a role login")
	}
}

func TestE2eLogin_RoleWithoutBroker(t *testing.T) {
	hermeticE2E(t, "")
	var saw map[string]string
	fakeLoginChild(t, &saw)
	_, err := runLogin(t, "--role", "agent")
	if exitcode.Code(err) != exitcode.BadArgs || err == nil || !strings.Contains(err.Error(), "--role cần broker") { //znf:allow-lang
		t.Fatalf("want BadArgs '--role cần broker', got %v", err) //znf:allow-lang
	}
}

func TestE2eLogin_UnknownRoleRejectedLocally(t *testing.T) {
	var role string
	mp, hits := brokerFixture(t, e2eAccountHandler(t, &role))
	hermeticE2E(t, mp)
	var saw map[string]string
	fakeLoginChild(t, &saw)
	_, err := runLogin(t, "--role", "end_user")
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Fatalf("want BadArgs, got %v", err)
	}
	if *hits != 0 {
		t.Errorf("an unknown role must not reach the broker")
	}
}

// Broker credentials live only in the child's env: no .env is written anywhere.
func TestChildCmd_BrokerCredsOnlyInChild(t *testing.T) {
	var role string
	mp, _ := brokerFixture(t, e2eAccountHandler(t, &role))
	hermeticE2E(t, mp)
	bc, err := e2eBroker("agent")
	if err != nil || bc == nil {
		t.Fatalf("e2eBroker: %v %v", bc, err)
	}
	lookup, err := brokerLookup(t.Context(), bc, "agent")
	if err != nil {
		t.Fatal(err)
	}
	env := envMap(childCmd("docker", []string{"run", "img"}, nil, lookup))
	if env["E2E_EMAIL"] != "agent@test.example" || env["E2E_PASSWORD"] != "test-pass" || env["E2E_DOMAIN"] != "e2e.test.example" {
		t.Errorf("journey child lacks broker credentials")
	}
	if envMap(childCmd("npm", []string{"install"}, nil, lookup))["E2E_PASSWORD"] != "" {
		t.Errorf("install child must not carry credentials")
	}
	for _, root := range []string{".", os.Getenv("HOME"), os.Getenv("ZENIFY_HOME")} {
		_ = filepath.WalkDir(root, func(p string, d os.DirEntry, _ error) error {
			if d != nil && !d.IsDir() && d.Name() == ".env" {
				t.Errorf("unexpected .env written: %s", p)
			}
			return nil
		})
	}
}

func TestVisualCheck_BrokerEnvToDocker(t *testing.T) {
	var role string
	mp, _ := brokerFixture(t, e2eAccountHandler(t, &role))
	hermeticE2E(t, mp)
	if err := os.MkdirAll(filepath.Join(".znf", "visual"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".znf", "visual", "routes.json"), []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}
	origExec, origPre := execChild, preflightDocker
	t.Cleanup(func() { execChild, preflightDocker = origExec, origPre })
	var docker map[string]string
	var name string
	execChild = func(c *exec.Cmd) error {
		name, docker = filepath.Base(c.Path), envMap(c)
		return nil
	}
	preflightDocker = func() error { return nil }
	if _, _, err := execCmd(newVisualCmd(), "check", "--port", "3327", "--role", "sale"); err != nil {
		t.Fatal(err)
	}
	if role != "sale" || name != "docker" || docker["E2E_EMAIL"] != "sale@test.example" || docker["E2E_PASSWORD"] != "test-pass" {
		t.Errorf("role=%q child=%q: docker env lacks the broker credentials", role, name)
	}
}

func TestVisualCheck_RoleWithoutBroker(t *testing.T) {
	hermeticE2E(t, "")
	_, _, err := execCmd(newVisualCmd(), "check", "--port", "3327", "--role", "sale")
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Fatalf("want BadArgs, got %v", err)
	}
}
