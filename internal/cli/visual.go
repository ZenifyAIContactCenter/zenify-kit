package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
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
	update bool
	out    io.Writer
}

// runVisualCheck ghi harness ra tmp, dựng RunConfig, gọi visual.Check, map lỗi.
func runVisualCheck(o visualOpts) error {
	snap := filepath.Join(o.repo, ".znf", "visual")
	if _, err := os.Stat(filepath.Join(snap, "routes.json")); err != nil {
		return exitcode.New(exitcode.BadArgs,
			fmt.Errorf("không thấy %s — repo chưa cấu hình visual", filepath.Join(snap, "routes.json")))
	}
	harnessDir, err := os.MkdirTemp("", "znf-visual-*")
	if err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	defer os.RemoveAll(harnessDir)
	if err := visual.WriteHarness(harnessDir); err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	vo := visual.Options{Runner: o.runner, Getenv: os.Getenv, GOOS: o.goos, Stdout: o.out}
	cfg := visual.RunConfig{HarnessDir: harnessDir, SnapshotsDir: snap, Port: o.port, Update: o.update}
	if err := visual.Check(vo, cfg); err != nil {
		if o.update {
			return exitcode.New(exitcode.Fail, err)
		}
		diff := filepath.Join(snap, "__diff__")
		return exitcode.New(exitcode.Fail,
			fmt.Errorf("visual mismatch — xem ảnh diff tại %s: %w", diff, err))
	}
	return nil
}

func newVisualCmd() *cobra.Command {
	var repo string
	var port int
	var update bool
	c := &cobra.Command{
		Use:   "visual",
		Short: "Visual-regression golden-diff (Playwright trong Docker pinned)",
	}
	check := &cobra.Command{
		Use:   "check",
		Short: "So từng route với baseline; --update để chụp lại baseline",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if repo == "" {
				repo, _ = os.Getwd()
			}
			out := cmd.OutOrStdout()
			runner := func(name string, args []string) error {
				c := exec.Command(name, args...) //nolint:gosec // G204 -- fixed "docker"; args internally computed
				c.Stdout = out
				c.Stderr = os.Stderr
				return c.Run()
			}
			return runVisualCheck(visualOpts{
				runner: runner, goos: runtime.GOOS, repo: repo, port: port, update: update,
				out: out,
			})
		},
	}
	check.Flags().StringVar(&repo, "repo", "", "target repo path (mặc định cwd)")
	check.Flags().IntVar(&port, "port", 0, "port dev-server trên host")
	check.Flags().BoolVar(&update, "update", false, "chụp lại baseline thay vì so")
	c.AddCommand(check)
	return c
}
