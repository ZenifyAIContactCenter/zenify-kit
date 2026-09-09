package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/release"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/workspace"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/wt"
	"github.com/spf13/cobra"
)

// defaultOutSub is where the report is written by default — the workspace's record-layer
// convention (M6a). The store is found via resolveDocsStore (Task 1). Override with the
// --out-dir flag for a different workspace (keeps the kit project-agnostic).
const defaultOutSub = "releases"

// runReleaseReport is the testable core. FAIL-OPEN: always returns nil; every error becomes
// a note printed to stderr.
// outDir empty → defaults to the repo's docs/releases (the repo path is auto-discovered per
// layout, flat or repos/<repo> after `zenify migrate`).
func runReleaseReport(workspaceDir string, n int, noFetch bool, outDir string, verbose, unreleased bool, r gitx.Runner, stdout, stderr io.Writer) error {
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
	// unreleased view = every deploy repo (ResolveUnreleased); finalize = repos with release<n> (Resolve).
	var repos []string
	var err error
	if unreleased {
		repos, err = release.ResolveUnreleased(r, workspaceDir, os.ReadFile, disc)
	} else {
		repos, err = release.Resolve(r, workspaceDir, n, os.ReadFile, disc)
	}
	if err != nil {
		fmt.Fprintf(stderr, "release-report: không phân giải repo: %v (fail-open)\n", err) //znf:allow-lang
		return nil
	}
	if !noFetch {
		for _, name := range repos {
			if dir, ok := resolve(name); ok {
				if unreleased {
					// unreleased only needs staging fresh (the base marker = old release, nearly
					// immutable, already local). Fetching release<n> is meaningless for a repo
					// that hasn't cut release<n> this week.
					_ = release.Fetch(r, dir, "staging")
				} else {
					_ = release.Fetch(r, dir, fmt.Sprintf("release%d", n), "staging")
				}
			}
		}
	}
	// loadSpecs reads specs/<repo>/*-design.md from the docs-store, parses the Brief. Fail-open: error → nil.
	storeSpecs := filepath.Join(resolveDocsStore(workspaceDir, os.Getenv, os.UserHomeDir, os.Stat, os.ReadDir), "specs")
	loadSpecs := func(repo string) []release.SpecMeta {
		dir := filepath.Join(storeSpecs, repo)
		ents, err := os.ReadDir(dir)
		if err != nil {
			return nil
		}
		var out []release.SpecMeta
		for _, e := range ents {
			if e.IsDir() || !strings.HasSuffix(e.Name(), "-design.md") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
			if err != nil {
				continue
			}
			out = append(out, release.ParseSpecBrief(filepath.Join("specs", repo, e.Name()), b))
		}
		return out
	}
	var rep release.Report
	if unreleased {
		rep = release.BuildUnreleased(r, resolve, repos, n, loadPatterns, loadSpecs)
	} else {
		rep = release.Build(r, resolve, repos, n, loadPatterns, loadSpecs)
	}
	out := release.Render(rep, verbose)
	if outDir == "" {
		outDir = filepath.Join(resolveDocsStore(workspaceDir, os.Getenv, os.UserHomeDir, os.Stat, os.ReadDir), defaultOutSub)
	}
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		fmt.Fprintf(stderr, "release-report: không tạo được thư mục out: %v (fail-open)\n", err) //znf:allow-lang
		return nil
	}
	fname := fmt.Sprintf("R%d.md", n)
	if unreleased {
		fname = "unreleased.md"
	}
	path := filepath.Join(outDir, fname)
	if err := os.WriteFile(path, []byte(out), 0o600); err != nil {
		fmt.Fprintf(stderr, "release-report: không ghi được report: %v (fail-open)\n", err) //znf:allow-lang
		return nil
	}
	fmt.Fprintln(stdout, path)
	// FR-4.3: at finalize (finalize, not --unreleased), close out unreleased.md — regenerate the
	// view against the new marker (range release<n>..staging, nearly empty right after the cut).
	// Best-effort, fail-open.
	if !unreleased {
		// reset uses the UNRELEASED scope (every deploy repo), NOT `repos` finalize (only repos
		// with release<n>) — otherwise unreleased.md after reset would miss repos that haven't
		// cut release<n> yet.
		unrepos, _ := release.ResolveUnreleased(r, workspaceDir, os.ReadFile, disc)
		repFresh := release.BuildUnreleased(r, resolve, unrepos, n, loadPatterns, loadSpecs)
		if e := os.WriteFile(filepath.Join(outDir, "unreleased.md"), []byte(release.Render(repFresh, false)), 0o600); e != nil {
			fmt.Fprintf(stderr, "release-report: không reset được unreleased.md: %v (fail-open)\n", e) //znf:allow-lang
		}
	}
	return nil
}

func newReleaseReportCmd() *cobra.Command {
	var workspaceDir string
	var noFetch bool
	var outDir string
	var verbose bool
	var unreleased bool
	cmd := &cobra.Command{
		Use:   "release-report [N]",
		Short: "sinh report rủi ro cho một release (chỉ-đọc, ghi docs/releases/R<N>.md)", //znf:allow-lang
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := gitx.ExecRunner()
			if workspaceDir == "" {
				workspaceDir, _ = os.Getwd()
			}
			n := 0
			if len(args) == 1 {
				if _, err := fmt.Sscanf(args[0], "%d", &n); err != nil {
					return fmt.Errorf("invalid release number %q: %w", args[0], err)
				}
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
				fmt.Fprintln(cmd.ErrOrStderr(), "release-report: không xác định được release N (fail-open)") //znf:allow-lang
				return nil
			}
			return runReleaseReport(workspaceDir, n, noFetch, outDir, verbose, unreleased, r, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&workspaceDir, "workspace", "", "thư mục workspace (mặc định cwd)")                                             //znf:allow-lang
	cmd.Flags().BoolVar(&noFetch, "no-fetch", false, "bỏ git fetch, dùng ref local")                                                      //znf:allow-lang
	cmd.Flags().StringVar(&outDir, "out-dir", "", "thư mục ghi report (mặc định repo docs/releases, tự tìm theo layout)")                 //znf:allow-lang
	cmd.Flags().BoolVar(&verbose, "verbose", false, "hiện chore + commit chi tiết")                                                       //znf:allow-lang
	cmd.Flags().BoolVar(&unreleased, "unreleased", false, "ghi view release đang hình thành (release<latest>..staging) ra unreleased.md") //znf:allow-lang
	return cmd
}
