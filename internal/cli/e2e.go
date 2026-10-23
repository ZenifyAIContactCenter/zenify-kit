package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/brokerclient"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/devenv"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/e2e"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/pwdocker"
	"github.com/spf13/cobra"
)

// e2eEnvKeys are the credentials a Playwright child process needs (docker run -e E2E_*).
var e2eEnvKeys = []string{"E2E_DOMAIN", "E2E_EMAIL", "E2E_PASSWORD"}

// credLookup resolves a key from the environment first, then the credential
// file at envPath (FR-7.2). An unreadable file counts as absent.
func credLookup(envPath string, getenv func(string) string) func(string) string {
	var f *devenv.File
	if envPath != "" {
		f, _ = devenv.Load(envPath)
	}
	return devenv.Lookup(getenv, f)
}

// e2eAccountTimeout bounds the broker account lookup, gh included.
const e2eAccountTimeout = 30 * time.Second

// brokerLookup fetches the E2E account for role (empty = administrator) from the
// broker. The returned lookup serves E2E_DOMAIN/E2E_EMAIL/E2E_PASSWORD from the
// response and falls back to the environment for every other key. The values
// live in memory and reach only the child process env (FR-10.2).
func brokerLookup(ctx context.Context, c *brokerclient.Client, role string) (func(string) string, error) {
	if role == "" {
		role = "administrator"
	}
	if !slices.Contains(api.E2ERoles, role) {
		return nil, exitcode.New(exitcode.BadArgs,
			fmt.Errorf("--role phải là một trong %s", strings.Join(api.E2ERoles, ", "))) //znf:allow-lang
	}
	ctx, cancel := context.WithTimeout(ctx, e2eAccountTimeout)
	defer cancel()
	var resp api.E2EAccountResponse
	if err := c.Post(ctx, "/e2e/account", api.E2EAccountRequest{Role: role}, &resp); err != nil {
		return nil, brokerCallErr(err)
	}
	return func(k string) string {
		switch k {
		case "E2E_DOMAIN":
			return resp.Domain
		case "E2E_EMAIL":
			return resp.Email
		case "E2E_PASSWORD":
			return resp.Password
		}
		return os.Getenv(k)
	}, nil
}

// e2eBroker returns the broker client for E2E accounts, or nil when the
// manifest declares none. A --role without a broker is a usage error.
func e2eBroker(role string) (*brokerclient.Client, error) {
	c, ok, err := brokerFromFlags("", "")
	if err != nil {
		return nil, err
	}
	if !ok {
		if role != "" {
			return nil, exitcode.New(exitcode.BadArgs, errors.New("--role cần broker: manifest không khai báo broker")) //znf:allow-lang
		}
		return nil, nil
	}
	return c, nil
}

// e2eLookup resolves credentials: from the broker when c is set, else from the
// environment and the workspace credential file (the v0.38.0 behaviour).
func e2eLookup(ctx context.Context, c *brokerclient.Client, role string) (func(string) string, error) {
	if c == nil {
		return credLookup(workspaceEnvPath(), os.Getenv), nil
	}
	return brokerLookup(ctx, c, role)
}

// Seams for tests: the child process runner and the docker readiness check.
var (
	execChild       = func(c *exec.Cmd) error { return c.Run() }
	preflightDocker = func() error {
		return dockerPreflight(exec.LookPath, func() error {
			return exec.Command("docker", "info").Run() //nolint:gosec // G204 -- fixed args
		})
	}
)

// childCmd builds a child process. Only the child that runs the journey
// (`docker run`, `npx playwright test`) gets E2E_* from lookup, so the
// credentials reach it without being inherited from the shell; install steps
// (`npm install`, `npx playwright install`) never see them, not even a value
// exported in the shell.
func childCmd(name string, args, extraEnv []string, lookup func(string) string) *exec.Cmd {
	c := exec.Command(name, args...) //nolint:gosec // G204 -- name is fixed npm/npx/docker; args computed internally
	env := append(os.Environ(), extraEnv...)
	if runsJourney(name, args) {
		c.Env = devenv.ChildEnv(env, e2eEnvKeys, lookup)
		return c
	}
	c.Env = []string{} // non-nil: a nil Env would inherit the parent's
	cred := map[string]bool{}
	for _, k := range e2eEnvKeys {
		cred[k] = true
	}
	for _, kv := range env {
		if k, _, _ := strings.Cut(kv, "="); !cred[k] {
			c.Env = append(c.Env, kv)
		}
	}
	return c
}

// runsJourney reports whether name+args is the step that runs Playwright tests.
func runsJourney(name string, args []string) bool {
	switch name {
	case "docker":
		return len(args) > 0 && args[0] == "run"
	case "npx":
		return len(args) > 1 && args[0] == "playwright" && args[1] == "test"
	}
	return false
}

func e2eDir(repo string) string { return filepath.Join(repo, ".znf", "e2e") }

// runE2eLint scans the repo's .znf/e2e via e2e.Lint, mapping the exit code.
func runE2eLint(dir string, out io.Writer) error {
	_, err := e2e.Lint(dir, out)
	return err
}

// runE2eRun writes the harness to tmp, builds a RunConfig, and calls e2e.Check.
func runE2eRun(runner func(string, []string) error, lookup func(string) string, goos, repo string, port int, baseURL string, out io.Writer) error {
	dir := e2eDir(repo)
	if _, err := os.Stat(filepath.Join(dir, "e2e.config.json")); err != nil {
		return exitcode.New(exitcode.BadArgs,
			fmt.Errorf("not found %s — repo has no e2e configured", filepath.Join(dir, "e2e.config.json")))
	}
	if baseURL != "" {
		if err := e2e.CheckBaseURL(dir, baseURL); err != nil {
			return err
		}
	}
	// lint before running: a shallow journey isn't worth spending a Docker run on.
	if _, err := e2e.Lint(dir, out); err != nil {
		return err
	}
	harnessDir, err := os.MkdirTemp("", "znf-e2e-*")
	if err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	defer func() { _ = os.RemoveAll(harnessDir) }()
	if err := e2e.WriteHarness(harnessDir); err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	o := pwdocker.Options{Runner: runner, Getenv: lookup, GOOS: goos, Stdout: out}
	cfg := e2e.RunConfig{HarnessDir: harnessDir, E2EDir: dir, Port: port, BaseURL: baseURL}
	if err := e2e.Check(o, cfg); err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	return nil
}

// newE2eLoginCmd logs in on the host (not Docker) and writes the Playwright storage state.
func newE2eLoginCmd() *cobra.Command {
	var url, role string
	var port int
	c := &cobra.Command{
		Use:   "login",
		Short: "Đăng nhập trên host, ghi storage state cho Playwright MCP (đọc E2E_* từ env)", //znf:allow-lang
		RunE: func(cmd *cobra.Command, _ []string) error {
			bc, err := e2eBroker(role)
			if err != nil {
				return err
			}
			if (url == "") == (port == 0) { // port 0 = unset
				return exitcode.New(exitcode.BadArgs, fmt.Errorf("need exactly one of --url or --port"))
			}
			if port != 0 && (port < 1 || port > 65535) {
				return exitcode.New(exitcode.BadArgs, fmt.Errorf("--port must be 1..65535"))
			}
			if port != 0 {
				url = fmt.Sprintf("http://localhost:%d", port)
			}
			if err := e2e.CheckLoopbackURL(url); err != nil { // before any broker call
				return err
			}
			home, err := os.UserHomeDir()
			if err != nil {
				return exitcode.New(exitcode.Fail, err)
			}
			lookup, err := e2eLookup(cmd.Context(), bc, role)
			if err != nil {
				return err
			}
			runner := func(dir, name string, args []string, extraEnv []string) error {
				cc := childCmd(name, args, extraEnv, lookup)
				cc.Dir = dir
				cc.Stdout = os.Stderr
				cc.Stderr = os.Stderr
				return execChild(cc)
			}
			_, err = e2e.Login(e2e.LoginOptions{Home: home, BaseURL: url, Role: role, Getenv: lookup,
				Run: runner, Stdout: cmd.OutOrStdout()})
			return err
		},
	}
	c.Flags().StringVar(&url, "url", "", "origin của web app (vd http://localhost:3327)")                          //znf:allow-lang
	c.Flags().IntVar(&port, "port", 0, "port web app trên localhost")                                              //znf:allow-lang
	c.Flags().StringVar(&role, "role", "", "role tài khoản E2E lấy từ broker, ghi state-<role>.json (cần broker)") //znf:allow-lang
	return c
}

func newE2eCmd() *cobra.Command {
	var repo, role, baseURL string
	var port int
	c := &cobra.Command{
		Use:   "e2e",
		Short: "E2E functional (Playwright journey thật trong Docker) + lint chống test hợt", //znf:allow-lang
	}
	resolveRepo := func() string {
		if repo == "" {
			repo, _ = os.Getwd()
		}
		return repo
	}
	run := &cobra.Command{
		Use:   "run",
		Short: "Chạy journey .znf/e2e trong Docker (cần --port dev-server host hoặc --base-url trong allowlist)", //znf:allow-lang
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			bc, err := e2eBroker(role)
			if err != nil {
				return err
			}
			if (port == 0) == (baseURL == "") {
				return exitcode.New(exitcode.BadArgs, fmt.Errorf("need exactly one of --port or --base-url"))
			}
			if err := preflightDocker(); err != nil {
				return err
			}
			lookup, err := e2eLookup(cmd.Context(), bc, role)
			if err != nil {
				return err
			}
			runner := func(name string, args []string) error {
				cc := childCmd(name, args, nil, lookup)
				cc.Stdout = out
				cc.Stderr = os.Stderr
				return execChild(cc)
			}
			return runE2eRun(runner, lookup, runtime.GOOS, resolveRepo(), port, baseURL, out)
		},
	}
	lint := &cobra.Command{
		Use:   "lint",
		Short: "Chặn cơ học journey 'hợt' (thiếu re-fetch/assert/cleanup, dùng anti-pattern)", //znf:allow-lang
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runE2eLint(e2eDir(resolveRepo()), cmd.OutOrStdout())
		},
	}
	run.Flags().StringVar(&repo, "repo", "", "target repo path (mặc định cwd)")                                      //znf:allow-lang
	run.Flags().IntVar(&port, "port", 0, "port dev-server trên host")                                                //znf:allow-lang
	run.Flags().StringVar(&baseURL, "base-url", "", "origin cần test (phải nằm trong baseUrls của e2e.config.json)") //znf:allow-lang
	run.Flags().StringVar(&role, "role", "", "role tài khoản E2E lấy từ broker (cần broker)")                        //znf:allow-lang
	lint.Flags().StringVar(&repo, "repo", "", "target repo path (mặc định cwd)")                                     //znf:allow-lang
	c.AddCommand(run, lint, newE2eLoginCmd())
	return c
}
