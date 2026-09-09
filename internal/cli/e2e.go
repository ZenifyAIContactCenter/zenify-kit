package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/e2e"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/pwdocker"
	"github.com/spf13/cobra"
)

func e2eDir(repo string) string { return filepath.Join(repo, ".znf", "e2e") }

// runE2eLint quét .znf/e2e của repo qua e2e.Lint, map exit code.
func runE2eLint(dir string, out io.Writer) error {
	_, err := e2e.Lint(dir, out)
	return err
}

// runE2eRun ghi harness ra tmp, dựng RunConfig, gọi e2e.Check.
func runE2eRun(runner func(string, []string) error, goos, repo string, port int, out io.Writer) error {
	dir := e2eDir(repo)
	if _, err := os.Stat(filepath.Join(dir, "e2e.config.json")); err != nil {
		return exitcode.New(exitcode.BadArgs,
			fmt.Errorf("không thấy %s — repo chưa cấu hình e2e", filepath.Join(dir, "e2e.config.json")))
	}
	// lint trước khi chạy: một journey 'hợt' không đáng tốn một lần chạy Docker.
	if _, err := e2e.Lint(dir, out); err != nil {
		return err
	}
	harnessDir, err := os.MkdirTemp("", "znf-e2e-*")
	if err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	defer os.RemoveAll(harnessDir)
	if err := e2e.WriteHarness(harnessDir); err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	o := pwdocker.Options{Runner: runner, Getenv: os.Getenv, GOOS: goos, Stdout: out}
	cfg := e2e.RunConfig{HarnessDir: harnessDir, E2EDir: dir, Port: port}
	if err := e2e.Check(o, cfg); err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	return nil
}

func newE2eCmd() *cobra.Command {
	var repo string
	var port int
	c := &cobra.Command{
		Use:   "e2e",
		Short: "E2E functional (Playwright journey thật trong Docker) + lint chống test hợt",
	}
	resolveRepo := func() string {
		if repo == "" {
			repo, _ = os.Getwd()
		}
		return repo
	}
	run := &cobra.Command{
		Use:   "run",
		Short: "Chạy journey .znf/e2e trong Docker (cần --port dev-server host)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			if port == 0 {
				return exitcode.New(exitcode.BadArgs, fmt.Errorf("cần --port <port dev-server host>"))
			}
			if err := dockerPreflight(exec.LookPath, func() error {
				return exec.Command("docker", "info").Run() //nolint:gosec // G204 -- fixed args
			}); err != nil {
				return err
			}
			runner := func(name string, args []string) error {
				cc := exec.Command(name, args...) //nolint:gosec // G204 -- fixed "docker"; args computed
				cc.Stdout = out
				cc.Stderr = os.Stderr
				return cc.Run()
			}
			return runE2eRun(runner, runtime.GOOS, resolveRepo(), port, out)
		},
	}
	lint := &cobra.Command{
		Use:   "lint",
		Short: "Chặn cơ học journey 'hợt' (thiếu re-fetch/assert/cleanup, dùng anti-pattern)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runE2eLint(e2eDir(resolveRepo()), cmd.OutOrStdout())
		},
	}
	run.Flags().StringVar(&repo, "repo", "", "target repo path (mặc định cwd)")
	run.Flags().IntVar(&port, "port", 0, "port dev-server trên host")
	lint.Flags().StringVar(&repo, "repo", "", "target repo path (mặc định cwd)")
	c.AddCommand(run, lint)
	return c
}
