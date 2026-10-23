package cli

import (
	"bytes"
	"encoding/json"
	"io"
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
	hermeticE2E(t, "")
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
	hermeticE2E(t, "")
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
// real workspace: cwd, ZENIFY_HOME and HOME are empty temp dirs, PATH has no
// gh/npx/docker, E2E_* is blank, and the trusted broker manifest is
// manifestPath when given (else TestMain's no-broker one) — these commands
// have no --manifest flag.
func hermeticE2E(t *testing.T, manifestPath string) {
	t.Helper()
	cwd := t.TempDir()
	t.Chdir(cwd)
	t.Setenv("ZENIFY_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir()) // no gh, no npx, no docker
	for _, k := range e2eEnvKeys {
		t.Setenv(k, "")
	}
	if manifestPath != "" {
		useTrustedManifest(t, manifestPath)
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

// --url is checked before the broker is asked for an account.
func TestE2eLogin_NonLoopbackURLBeforeBroker(t *testing.T) {
	var role string
	mp, hits := brokerFixture(t, e2eAccountHandler(t, &role))
	hermeticE2E(t, mp)
	var saw map[string]string
	fakeLoginChild(t, &saw)
	_, _, err := execCmd(newE2eCmd(), "login", "--url", "https://app.example.com")
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Fatalf("want BadArgs, got %v", err)
	}
	if *hits != 0 {
		t.Errorf("broker got %d requests before the --url check, want 0", *hits)
	}
}

// A broker 400 from /e2e/account is a usage error.
func TestBrokerLookup_400IsBadArgs(t *testing.T) {
	mp, _ := brokerFixture(t, map[string]http.HandlerFunc{"/e2e/account": func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(api.Error{Error: api.CodeUnknownRole, Message: "nope"})
	}})
	hermeticE2E(t, mp)
	bc, err := e2eBroker("agent")
	if err != nil || bc == nil {
		t.Fatalf("e2eBroker: %v %v", bc, err)
	}
	if _, err := brokerLookup(t.Context(), bc, "agent"); exitcode.Code(err) != exitcode.BadArgs {
		t.Fatalf("want BadArgs, got %v", err)
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
	// The scan must be able to fail: a planted file is found first.
	probe := t.TempDir()
	if err := os.WriteFile(filepath.Join(probe, "x"), []byte("pw=test-pass"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := filesContaining(t, probe, "test-pass"); len(got) != 1 {
		t.Fatalf("scan self-check found %v, want the planted file", got)
	}
	for _, root := range []string{".", os.Getenv("HOME"), os.Getenv("ZENIFY_HOME")} {
		if got := filesContaining(t, root, "test-pass"); len(got) > 0 {
			t.Errorf("broker password written to disk: %v", got)
		}
	}
}

// filesContaining lists files under root whose name is .env or whose content
// contains needle; a walk error fails the test instead of being skipped.
func filesContaining(t *testing.T, root, needle string) []string {
	t.Helper()
	var hits []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := os.ReadFile(p) //nolint:gosec // test-owned temp dirs
		if err != nil {
			return err
		}
		if d.Name() == ".env" || strings.Contains(string(b), needle) {
			hits = append(hits, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return hits
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

// e2eRunRepo makes cwd a repo with one clean journey and swaps the docker
// seams; the returned map receives the env of the `docker run` child.
func e2eRunRepo(t *testing.T) (docker *map[string]string, ran *bool) {
	t.Helper()
	dir := filepath.Join(".znf", "e2e")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	clean := `test('ok [FR-1]', async ({page, apiClient, cleanupTracker}) => {
  await page.getByRole('button').click();
  // @domain-assert:ticket
  const r = await apiClient.get('/v2/ticket/1'); expect((await r.json()).subject).toBe('x');
  cleanupTracker.add(async () => {});
});`
	for name, body := range map[string]string{"e2e.config.json": "{}", "ok.spec.ts": clean} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	origExec, origPre := execChild, preflightDocker
	t.Cleanup(func() { execChild, preflightDocker = origExec, origPre })
	docker, ran = new(map[string]string), new(bool)
	execChild = func(c *exec.Cmd) error {
		*ran = true
		if filepath.Base(c.Path) == "docker" || len(c.Args) > 0 && c.Args[0] == "docker" {
			*docker = envMap(c)
		}
		return nil
	}
	preflightDocker = func() error { return nil }
	return docker, ran
}

// `e2e run --role sale` asks the broker for the sale account and hands it to
// the docker child only.
func TestE2eRun_BrokerRole(t *testing.T) {
	var role string
	mp, _ := brokerFixture(t, e2eAccountHandler(t, &role))
	hermeticE2E(t, mp)
	docker, _ := e2eRunRepo(t)
	if _, _, err := execCmd(newE2eCmd(), "run", "--port", "3327", "--role", "sale"); err != nil {
		t.Fatal(err)
	}
	if role != "sale" {
		t.Errorf("broker got role %q, want sale", role)
	}
	if (*docker)["E2E_EMAIL"] != "sale@test.example" || (*docker)["E2E_PASSWORD"] != "test-pass" || (*docker)["E2E_DOMAIN"] != "e2e.test.example" {
		t.Errorf("docker child lacks the broker credentials")
	}
}

// An unreadable trusted manifest stops e2e run before docker and before any
// local credential fallback.
func TestE2eRun_InvalidManifestErrors(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "repos.yaml")
	if err := os.WriteFile(bad, []byte("broker: [not a map\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hermeticE2E(t, bad)
	_, ran := e2eRunRepo(t)
	_, _, err := execCmd(newE2eCmd(), "run", "--port", "3327")
	if err == nil || !strings.Contains(err.Error(), "manifest") || *ran {
		t.Fatalf("err=%v ran=%v, want the manifest error and no child", err, *ran)
	}
}

func TestE2eRun_BaseURLRefusedBeforeDocker(t *testing.T) {
	_, ran := e2eRunRepo(t)
	err := os.WriteFile(filepath.Join(".znf", "e2e", "e2e.config.json"), []byte(`{"baseUrls":["https://web-staging.zenify.vn"]}`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	runner := func(string, []string) error { called = true; return nil }
	err = runE2eRun(runner, func(string) string { return "" }, "linux", ".", 0, "https://zenify.vn", io.Discard)
	if exitcode.Code(err) != exitcode.BadArgs || called || *ran {
		t.Fatalf("err=%v called=%v ran=%v, want BadArgs and no docker", err, called, *ran)
	}
}

func TestE2eRun_BaseURLAllowedReachesDocker(t *testing.T) {
	e2eRunRepo(t)
	err := os.WriteFile(filepath.Join(".znf", "e2e", "e2e.config.json"), []byte(`{"baseUrls":["https://web-staging.zenify.vn"]}`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	runner := func(_ string, a []string) error { got = a; return nil }
	if err := runE2eRun(runner, func(string) string { return "" }, "linux", ".", 0, "https://web-staging.zenify.vn", io.Discard); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(got, " "), "BASE_URL=https://web-staging.zenify.vn") {
		t.Fatalf("docker args lack BASE_URL: %v", got)
	}
}

func TestE2eRun_NeedsExactlyOneOfPortBaseURL(t *testing.T) {
	hermeticE2E(t, "")
	e2eRunRepo(t)
	for _, args := range [][]string{{"run"}, {"run", "--port", "3327", "--base-url", "https://web-staging.zenify.vn"}} {
		_, _, err := execCmd(newE2eCmd(), args...)
		if exitcode.Code(err) != exitcode.BadArgs || !strings.Contains(err.Error(), "need exactly one of --port or --base-url") {
			t.Errorf("%v: got %v", args, err)
		}
	}
}
