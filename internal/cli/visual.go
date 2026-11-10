package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

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
	git    visual.GitRunner // nil → skip the tracked-baseline check
}

// runVisualCheck writes the harness to tmp, builds a RunConfig, calls visual.Check, maps the error.
func runVisualCheck(o visualOpts) error {
	snap := filepath.Join(o.repo, ".znf", "visual")
	routes, err := visual.LoadRoutes(o.repo)
	if err != nil {
		return err
	}
	if len(routes) == 0 {
		_, _ = fmt.Fprintln(o.out, "0 routes")
		return nil
	}
	if !o.update && o.git != nil {
		untracked, checked := visual.UntrackedBaselines(o.git, o.repo, routes)
		if !checked {
			_, _ = fmt.Fprintln(o.out, "warning: cannot check that baselines are tracked (not a git repo?)")
		} else if len(untracked) > 0 {
			return exitcode.New(exitcode.Fail,
				fmt.Errorf("baseline not tracked by git — run: git -C %s add -f %s", o.repo, strings.Join(untracked, " ")))
		}
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
			// Fail on a broken routes.json before any broker/docker call can mask it.
			if _, err := visual.LoadRoutes(repo); err != nil {
				return err
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
				out: out, git: gitOutput,
			})
		},
	}
	check.Flags().StringVar(&repo, "repo", "", "target repo path (mặc định cwd)")               //znf:allow-lang
	check.Flags().IntVar(&port, "port", 0, "port dev-server trên host")                         //znf:allow-lang
	check.Flags().BoolVar(&update, "update", false, "chụp lại baseline thay vì so")             //znf:allow-lang
	check.Flags().StringVar(&role, "role", "", "role tài khoản E2E lấy từ broker (cần broker)") //znf:allow-lang
	c.AddCommand(check, newVisualAddRouteCmd())
	return c
}

func gitOutput(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output() //nolint:gosec // G204 -- fixed binary; args built by UntrackedBaselines
	return string(out), err
}

// emptyFlag reports whether a passed --wait-for/--aria/--mask carries an empty value.
func emptyFlag(name string, r *visual.Route) bool {
	switch name {
	case "wait-for":
		return r.WaitFor == ""
	case "aria":
		return r.Aria == ""
	}
	for _, m := range r.Mask {
		if m == "" {
			return true
		}
	}
	return false
}

func newVisualAddRouteCmd() *cobra.Command {
	var repo string
	r := visual.Route{}
	c := &cobra.Command{
		Use:   "add-route",
		Short: "Thêm một route vào .znf/visual/routes.json (đã validate)", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if repo == "" {
				repo, _ = os.Getwd()
			}
			for _, name := range []string{"wait-for", "aria", "mask"} {
				if cmd.Flags().Changed(name) && emptyFlag(name, &r) {
					return exitcode.New(exitcode.BadArgs, fmt.Errorf("--%s must not be empty", name))
				}
			}
			if err := visual.AddRoute(repo, r); err != nil {
				return err
			}
			rs, err := visual.LoadRoutes(repo)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "added route %s (%d routes)\n", r.Name, len(rs))
			_, _ = fmt.Fprintf(out, "next: zenify visual check --repo %s --port <P> --update\n", repo)
			files := append([]string{".znf/visual/routes.json"}, visual.BaselineFiles(r)...)
			_, _ = fmt.Fprintf(out, "then: git -C %s add -f %s\n", repo, strings.Join(files, " "))
			return nil
		},
	}
	f := c.Flags()
	f.StringVar(&repo, "repo", "", "target repo path (mặc định cwd)")                      //znf:allow-lang
	f.StringVar(&r.Name, "name", "", "tên route, khớp [A-Za-z0-9_-]+")                     //znf:allow-lang
	f.StringVar(&r.Path, "path", "", "đường dẫn route, bắt đầu bằng /")                    //znf:allow-lang
	f.StringVar(&r.WaitFor, "wait-for", "", "selector cần chờ trước khi chụp")             //znf:allow-lang
	f.StringArrayVar(&r.Mask, "mask", nil, "selector vùng động cần che (lặp được)")        //znf:allow-lang
	f.StringVar(&r.Aria, "aria", "", "selector gốc để so snapshot aria (bật so cấu trúc)") //znf:allow-lang
	_ = c.MarkFlagRequired("name")
	_ = c.MarkFlagRequired("path")
	return c
}
