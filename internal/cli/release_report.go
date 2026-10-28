package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/release"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ui"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/workspace"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/wt"
	"github.com/spf13/cobra"
)

// defaultOutSub is where the report is written by default — the workspace's record-layer
// convention (M6a). The store is found via resolveDocsStore (Task 1). Override with the
// --out-dir flag for a different workspace (keeps the kit project-agnostic).
const defaultOutSub = "releases"

// releaseTagRe validates the tag argument: a release tag is vYYMMDD.n.
var releaseTagRe = regexp.MustCompile(`^v\d{6}\.\d+$`)

// runReleaseReport is the testable core. FAIL-OPEN: always returns nil; every error becomes
// a note printed to stderr. tag "" = each repo's newest vYYMMDD.n tag.
// outDir empty → defaults to the repo's docs/releases (the repo path is auto-discovered per
// layout, flat or repos/<repo> after `zenify migrate`).
func runReleaseReport(workspaceDir string, tag string, noFetch bool, outDir string, verbose bool, r gitx.Runner, stdout, stderr io.Writer) error {
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
	// fetch tags first: auto-detect and the default tag read local tags.
	if !noFetch {
		for _, rp := range disc {
			_ = release.Fetch(r, rp.Path, "--tags", "staging")
		}
	}
	repos, err := release.Resolve(r, workspaceDir, os.ReadFile, disc)
	if err != nil {
		fmt.Fprintf(stderr, "release-report: không phân giải repo: %v (fail-open)\n", err) //znf:allow-lang
		return nil
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
	rep := release.Build(r, resolve, repos, tag, loadPatterns, loadSpecs)
	if rep.Tag == "" {
		fmt.Fprintln(stderr, "release-report: chưa có tag vYYMMDD.n trong repo nào (fail-open)") //znf:allow-lang
		for _, rr := range rep.Repos {
			fmt.Fprintf(stderr, "  %s: %s\n", rr.Name, rr.Err)
		}
		return nil
	}
	out := release.Render(rep, verbose)
	if outDir == "" {
		outDir = filepath.Join(resolveDocsStore(workspaceDir, os.Getenv, os.UserHomeDir, os.Stat, os.ReadDir), defaultOutSub)
	}
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		fmt.Fprintf(stderr, "release-report: không tạo được thư mục out: %v (fail-open)\n", err) //znf:allow-lang
		return nil
	}
	path := filepath.Join(outDir, rep.Tag+".md")
	if err := os.WriteFile(path, []byte(out), 0o600); err != nil {
		fmt.Fprintf(stderr, "release-report: không ghi được report: %v (fail-open)\n", err) //znf:allow-lang
		return nil
	}
	ui.New(stdout).Step(ui.StatusOK, "release report written", path)
	return nil
}

func newReleaseReportCmd() *cobra.Command {
	var workspaceDir string
	var noFetch bool
	var outDir string
	var verbose bool
	cmd := &cobra.Command{
		Use:   "release-report [tag]",
		Short: "sinh report rủi ro cho một release tag vYYMMDD.n (chỉ-đọc, ghi docs/releases/<tag>.md)", //znf:allow-lang
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if workspaceDir == "" {
				workspaceDir, _ = os.Getwd()
			}
			tag := ""
			if len(args) == 1 {
				tag = args[0]
				if !releaseTagRe.MatchString(tag) {
					return fmt.Errorf("invalid release tag %q: want vYYMMDD.n", tag)
				}
			}
			return runReleaseReport(workspaceDir, tag, noFetch, outDir, verbose, gitx.ExecRunner(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&workspaceDir, "workspace", "", "thư mục workspace (mặc định cwd)")                             //znf:allow-lang
	cmd.Flags().BoolVar(&noFetch, "no-fetch", false, "bỏ git fetch, dùng ref local")                                      //znf:allow-lang
	cmd.Flags().StringVar(&outDir, "out-dir", "", "thư mục ghi report (mặc định repo docs/releases, tự tìm theo layout)") //znf:allow-lang
	cmd.Flags().BoolVar(&verbose, "verbose", false, "hiện chore + commit chi tiết")                                       //znf:allow-lang
	return cmd
}
