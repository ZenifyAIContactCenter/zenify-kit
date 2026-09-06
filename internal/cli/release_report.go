package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/release"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/workspace"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/wt"
	"github.com/spf13/cobra"
)

// defaultOutSubdir là nơi ghi report mặc định — quy ước record-layer của workspace (M6a).
// Override bằng cờ --out-dir cho workspace khác (giữ kit project-agnostic).
const defaultOutRepo = "docs"
const defaultOutSub = "releases"

// runReleaseReport là lõi test được. FAIL-OPEN: luôn trả nil; mọi lỗi thành note in ra stderr.
// outDir rỗng → mặc định repo docs/releases (đường dẫn repo tự tìm theo layout,
// phẳng hoặc repos/<repo> sau `zenify migrate`).
func runReleaseReport(workspaceDir string, n int, noFetch bool, outDir string, r gitx.Runner, stdout, stderr io.Writer) error {
	loadPatterns := func(dir string) []string {
		c, err := wt.Load(dir)
		if err != nil {
			return nil
		}
		return c.GateAccessPatterns
	}
	disc := workspace.Discover(workspaceDir, workspace.DefaultMaxDepth, os.ReadDir)
	resolve := func(name string) (string, bool) {
		return workspace.Resolve(workspaceDir, name, workspace.DefaultMaxDepth, os.ReadDir)
	}
	repos, err := release.Resolve(r, workspaceDir, n, os.ReadFile, disc)
	if err != nil {
		fmt.Fprintf(stderr, "release-report: không phân giải repo: %v (fail-open)\n", err)
		return nil
	}
	if !noFetch {
		for _, name := range repos {
			if dir, ok := resolve(name); ok {
				_ = release.Fetch(r, dir, fmt.Sprintf("release%d", n), "staging")
			}
		}
	}
	rep := release.Build(r, resolve, repos, n, loadPatterns)
	out := release.Render(rep)
	if outDir == "" {
		outDir = resolveWorkspaceRepoDir(workspaceDir, defaultOutRepo, defaultOutSub, os.ReadDir)
	}
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
	var workspaceDir string
	var noFetch bool
	var outDir string
	cmd := &cobra.Command{
		Use:   "release-report [N]",
		Short: "sinh report rủi ro cho một release (chỉ-đọc, ghi docs/releases/R<N>.md)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := gitx.ExecRunner()
			if workspaceDir == "" {
				workspaceDir, _ = os.Getwd()
			}
			n := 0
			if len(args) == 1 {
				fmt.Sscanf(args[0], "%d", &n)
			} else {
				for _, rp := range workspace.Discover(workspaceDir, workspace.DefaultMaxDepth, os.ReadDir) {
					if nums, err := release.ReleaseNums(r, rp.Path); err == nil {
						for _, x := range nums {
							if x > n {
								n = x
							}
						}
					}
				}
			}
			if n <= 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "release-report: không xác định được release N (fail-open)")
				return nil
			}
			return runReleaseReport(workspaceDir, n, noFetch, outDir, r, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&workspaceDir, "workspace", "", "thư mục workspace (mặc định cwd)")
	cmd.Flags().BoolVar(&noFetch, "no-fetch", false, "bỏ git fetch, dùng ref local")
	cmd.Flags().StringVar(&outDir, "out-dir", "", "thư mục ghi report (mặc định repo docs/releases, tự tìm theo layout)")
	return cmd
}
