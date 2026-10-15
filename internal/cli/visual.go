package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/visual"
	"github.com/spf13/cobra"
)

type visualOpts struct {
	runner func(name string, args []string) error
	goos   string
	repo   string
	port   int
	getenv func(string) string // credential lookup; nil → os.Getenv
	update bool
	out    io.Writer
}

// runVisualCheck writes the harness to tmp, builds a RunConfig, calls visual.Check, maps the error.
func runVisualCheck(o visualOpts) error {
	snap := filepath.Join(o.repo, ".znf", "visual")
	if _, err := os.Stat(filepath.Join(snap, "routes.json")); err != nil {
		return exitcode.New(exitcode.BadArgs,
			fmt.Errorf("%s not found — repo has no visual config", filepath.Join(snap, "routes.json")))
	}
	harnessDir, err := os.MkdirTemp("", "znf-visual-*")
	if err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	defer func() { _ = os.RemoveAll(harnessDir) }()
	if err := visual.WriteHarness(harnessDir); err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	getenv := o.getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	vo := visual.Options{Runner: o.runner, Getenv: getenv, GOOS: o.goos, Stdout: o.out}
	cfg := visual.RunConfig{HarnessDir: harnessDir, SnapshotsDir: snap, Port: o.port, Update: o.update}
	if err := visual.Check(vo, cfg); err != nil {
		if o.update {
			return exitcode.New(exitcode.Fail, err)
		}
		diff := filepath.Join(snap, "__diff__")
		return exitcode.New(exitcode.Fail,
			fmt.Errorf("visual mismatch — see diff image at %s: %w", diff, err))
	}
	return nil
}

// dockerPreflight checks docker is ready BEFORE running the container (FR-5.5): a missing
// binary or a daemon that isn't running → report clearly instead of an infra error
// disguised as "visual mismatch".
func dockerPreflight(lookPath func(string) (string, error), info func() error) error {
	if _, err := lookPath("docker"); err != nil {
		return exitcode.New(exitcode.BadArgs,
			fmt.Errorf("docker not installed — install Docker then run `zenify doctor` to check"))
	}
	if err := info(); err != nil {
		return exitcode.New(exitcode.BadArgs,
			fmt.Errorf("docker daemon not running — start Docker Desktop then try again"))
	}
	return nil
}

func newVisualCmd() *cobra.Command {
	var repo string
	var port int
	var update bool
	var role string
	c := &cobra.Command{
		Use:   "visual",
		Short: "Visual-regression golden-diff (Playwright chạy trong Docker, đã ghim phiên bản)", //znf:allow-lang
	}
	check := &cobra.Command{
		Use:   "check",
		Short: "So từng route với baseline; --update để chụp lại baseline", //znf:allow-lang
		RunE: func(cmd *cobra.Command, _ []string) error {
			if repo == "" {
				repo, _ = os.Getwd()
			}
			out := cmd.OutOrStdout()
			bc, err := e2eBroker(role)
			if err != nil {
				return err
			}
			if port == 0 {
				return exitcode.New(exitcode.BadArgs,
					fmt.Errorf("need --port <host dev-server port>"))
			}
			if err := preflightDocker(); err != nil {
				return err
			}
			lookup, err := e2eLookup(cmd.Context(), bc, role)
			if err != nil {
				return err
			}
			runner := func(name string, args []string) error {
				c := childCmd(name, args, nil, lookup)
				c.Stdout = out
				c.Stderr = os.Stderr
				return execChild(c)
			}
			return runVisualCheck(visualOpts{
				runner: runner, getenv: lookup, goos: runtime.GOOS, repo: repo, port: port, update: update,
				out: out,
			})
		},
	}
	check.Flags().StringVar(&repo, "repo", "", "target repo path (mặc định cwd)")               //znf:allow-lang
	check.Flags().IntVar(&port, "port", 0, "port dev-server trên host")                         //znf:allow-lang
	check.Flags().BoolVar(&update, "update", false, "chụp lại baseline thay vì so")             //znf:allow-lang
	check.Flags().StringVar(&role, "role", "", "role tài khoản E2E lấy từ broker (cần broker)") //znf:allow-lang
	c.AddCommand(check)
	return c
}
