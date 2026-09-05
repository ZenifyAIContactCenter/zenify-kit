package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/release"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/wt"
	"github.com/spf13/cobra"
)

// runReleaseReport là lõi test được. FAIL-OPEN: luôn trả nil; mọi lỗi thành note in ra stderr.
func runReleaseReport(workspace string, n int, noFetch bool, r gitx.Runner, stdout, stderr io.Writer) error {
	loadPatterns := func(dir string) []string {
		c, err := wt.Load(dir)
		if err != nil {
			return nil
		}
		return c.GateAccessPatterns
	}
	repos, err := release.Resolve(r, workspace, n, os.ReadFile, func(p string) ([]string, error) {
		es, err := os.ReadDir(p)
		if err != nil {
			return nil, err
		}
		var ds []string
		for _, e := range es {
			if e.IsDir() {
				ds = append(ds, e.Name())
			}
		}
		return ds, nil
	})
	if err != nil {
		fmt.Fprintf(stderr, "release-report: không phân giải repo: %v (fail-open)\n", err)
		return nil
	}
	if !noFetch {
		for _, name := range repos {
			_ = release.Fetch(r, filepath.Join(workspace, name), fmt.Sprintf("release%d", n), "staging")
		}
	}
	rep := release.Build(r, workspace, repos, n, loadPatterns)
	out := release.Render(rep)
	outDir := filepath.Join(workspace, "zenify-knowledge", "releases")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "release-report: không tạo được thư mục out: %v (fail-open)\n", err)
		return nil
	}
	path := filepath.Join(outDir, fmt.Sprintf("R%d.md", n))
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		fmt.Fprintf(stderr, "release-report: không ghi được report: %v (fail-open)\n", err)
		return nil
	}
	fmt.Fprintln(stdout, path)
	return nil
}

func newReleaseReportCmd() *cobra.Command {
	var workspace string
	var noFetch bool
	cmd := &cobra.Command{
		Use:   "release-report [N]",
		Short: "sinh report rủi ro cho một release (chỉ-đọc, ghi zenify-knowledge/releases/R<N>.md)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := gitx.ExecRunner()
			if workspace == "" {
				workspace, _ = os.Getwd()
			}
			n := 0
			if len(args) == 1 {
				fmt.Sscanf(args[0], "%d", &n)
			} else {
				es, _ := os.ReadDir(workspace)
				for _, e := range es {
					if !e.IsDir() {
						continue
					}
					if nums, err := release.ReleaseNums(r, filepath.Join(workspace, e.Name())); err == nil {
						for _, x := range nums {
							if x > n {
								n = x
							}
						}
					}
				}
			}
			if n == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "release-report: không xác định được release N (fail-open)")
				return nil
			}
			return runReleaseReport(workspace, n, noFetch, r, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "thư mục workspace (mặc định cwd)")
	cmd.Flags().BoolVar(&noFetch, "no-fetch", false, "bỏ git fetch, dùng ref local")
	return cmd
}
