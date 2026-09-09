package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/release"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/speclife"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/workspace"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/wt"
	"github.com/spf13/cobra"
)

// loadAllSpecs walks <store>/specs/<repo>/*-design.md and parses each Brief. Fail-open: an
// unreadable dir/file is skipped, never fatal.
func loadAllSpecs(storeDir string) []release.SpecMeta {
	specsRoot := filepath.Join(storeDir, "specs")
	repoEnts, err := os.ReadDir(specsRoot)
	if err != nil {
		return nil
	}
	var out []release.SpecMeta
	for _, re := range repoEnts {
		if !re.IsDir() {
			continue
		}
		repo := re.Name()
		dir := filepath.Join(specsRoot, repo)
		ents, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if e.IsDir() || !strings.HasSuffix(e.Name(), "-design.md") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // G304 -- path computed from this tool's own store, not external input
			if err != nil {
				continue
			}
			out = append(out, release.ParseSpecBrief(filepath.Join("specs", repo, e.Name()), b))
		}
	}
	return out
}

// planExists reports whether a spec has its mirror plan: specs/<repo>/<date>-<topic>-design.md
// ↔ plans/<repo>/<date>-<topic>.md.
func planExists(storeDir, specPath string) bool {
	repo := speclife.RepoOf(specPath)
	base := strings.TrimSuffix(filepath.Base(specPath), ".md")
	base = strings.TrimSuffix(base, "-design")
	planPath := filepath.Join(storeDir, "plans", repo, base+".md")
	if _, err := os.Stat(planPath); err == nil {
		return true
	}
	return false
}

// resolveBase returns a repo's base ref: its worktree.json baseRef when present, else origin/main
// (the kit's own base, and the safe default for a repo with no wt config).
func resolveBase(dir string) string {
	if c, err := wt.Load(dir); err == nil && c.BaseRef != "" {
		return c.BaseRef
	}
	return "origin/main"
}

// runSpecStatus is the testable core (FAIL-OPEN: always returns nil). resolve/getBase/r are
// injected so tests need no real git or filesystem layout.
func runSpecStatus(
	storeDir, workspaceDir string,
	noFetch, jsonOut bool,
	base string, active bool,
	resolve func(string) (string, bool),
	getBase func(string) string,
	r gitx.Runner,
	stdout, stderr io.Writer,
) error {
	specs := loadAllSpecs(storeDir)

	hasPlan := map[string]bool{}
	reposSeen := map[string]bool{}
	for _, s := range specs {
		hasPlan[s.Path] = planExists(storeDir, s.Path)
		if repo := speclife.RepoOf(s.Path); repo != "" {
			reposSeen[repo] = true
		}
	}

	changesByRepo := map[string][]release.Change{}
	notesByRepo := map[string]map[string]release.RiskMeta{}
	gitEvaluable := map[string]bool{}
	for repo := range reposSeen {
		dir, ok := resolve(repo)
		if !ok {
			gitEvaluable[repo] = false
			continue
		}
		ref := base
		if ref == "" {
			ref = getBase(dir)
		}
		if !noFetch {
			_ = release.Fetch(r, dir, strings.TrimPrefix(ref, "origin/"))
		}
		commits, err := release.ReachableCommits(r, dir, ref)
		if err != nil {
			gitEvaluable[repo] = false
			fmt.Fprintf(stderr, "spec status: %s không đọc được git (%s): %v (fail-open)\n", repo, ref, err) //znf:allow-lang
			continue
		}
		gitEvaluable[repo] = true
		var notes, feats []release.Commit
		for _, c := range commits {
			if release.IsReleaseNote(c) {
				notes = append(notes, c)
			} else {
				feats = append(feats, c)
			}
		}
		changesByRepo[repo] = release.Aggregate(feats, nil)
		notesByRepo[repo] = release.NoteRiskBySlug(notes)
	}

	statuses := speclife.Statuses(specs, hasPlan, changesByRepo, notesByRepo, gitEvaluable)
	if active {
		var kept []speclife.Status
		for _, s := range statuses {
			if s.State != speclife.StateSuperseded {
				kept = append(kept, s)
			}
		}
		statuses = kept
	}
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].Path < statuses[j].Path })

	if jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(statuses); err != nil {
			fmt.Fprintf(stderr, "spec status: không mã hoá JSON: %v (fail-open)\n", err) //znf:allow-lang
		}
		return nil
	}
	if len(statuses) == 0 {
		fmt.Fprintln(stdout, "không có spec trong store.") //znf:allow-lang
		return nil
	}
	for _, s := range statuses {
		fmt.Fprintf(stdout, "%-12s %-40s %s\n", s.State, s.Slug, s.Path)
	}
	return nil
}

// runSpecContracts is the testable core for the registry view (FAIL-OPEN).
func runSpecContracts(storeDir, repo, collection string, jsonOut bool, stdout, stderr io.Writer) error {
	specs := loadAllSpecs(storeDir)
	contracts, skipped := speclife.BuildContracts(specs)
	if repo != "" {
		contracts = speclife.FilterByRepo(contracts, repo)
	}
	if collection != "" {
		contracts = speclife.FilterByCollection(contracts, collection)
	}
	sort.Slice(contracts, func(i, j int) bool { return contracts[i].SpecPath < contracts[j].SpecPath })

	if jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(contracts); err != nil {
			fmt.Fprintf(stderr, "spec contracts: không mã hoá JSON: %v (fail-open)\n", err) //znf:allow-lang
		}
		return nil
	}
	if len(contracts) == 0 {
		fmt.Fprintf(stdout, "không có contract (%d spec bỏ qua — thiếu tag _Blast-radius:/_DB:).\n", skipped) //znf:allow-lang
		return nil
	}
	for _, c := range contracts {
		fmt.Fprintf(stdout, "%-18s blast=%q db=%q  %s\n", c.Repo, c.BlastRadius, c.DB, c.SpecPath)
	}
	if skipped > 0 {
		fmt.Fprintf(stdout, "(%d spec bỏ qua — thiếu tag _Blast-radius:/_DB:)\n", skipped) //znf:allow-lang
	}
	return nil
}

func newSpecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spec",
		Short: "soi vòng đời spec (planned/built) và registry contract từ store spec", //znf:allow-lang
	}
	cmd.AddCommand(newSpecStatusCmd())
	cmd.AddCommand(newSpecContractsCmd())
	return cmd
}

func newSpecStatusCmd() *cobra.Command {
	var workspaceDir, base string
	var noFetch, jsonOut, active bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "liệt kê trạng thái vòng đời từng spec (planned/in-progress/built/built?/superseded/unknown)", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if workspaceDir == "" {
				workspaceDir, _ = os.Getwd()
			}
			r := gitx.ExecRunner()
			store := resolveDocsStore(workspaceDir, os.Getenv, os.UserHomeDir, os.Stat, os.ReadDir)
			resolve := func(name string) (string, bool) {
				return workspace.Resolve(workspaceDir, name, workspace.DefaultMaxDepth, os.ReadDir)
			}
			return runSpecStatus(store, workspaceDir, noFetch, jsonOut, base, active,
				resolve, resolveBase, r, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&workspaceDir, "workspace", "", "thư mục workspace (mặc định cwd)")              //znf:allow-lang
	cmd.Flags().StringVar(&base, "base", "", "ref cơ sở dùng chung mọi repo (mặc định: baseRef mỗi repo)") //znf:allow-lang
	cmd.Flags().BoolVar(&noFetch, "no-fetch", false, "bỏ git fetch, dùng ref local")                       //znf:allow-lang
	cmd.Flags().BoolVar(&jsonOut, "json", false, "in JSON thay vì bảng")                                   //znf:allow-lang
	cmd.Flags().BoolVar(&active, "active", false, "ẩn spec đã superseded")                                 //znf:allow-lang
	return cmd
}

func newSpecContractsCmd() *cobra.Command {
	var workspaceDir, repo, collection string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "contracts",
		Short: "registry contract: repo/collection mỗi spec khai qua _Blast-radius:/_DB:", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if workspaceDir == "" {
				workspaceDir, _ = os.Getwd()
			}
			store := resolveDocsStore(workspaceDir, os.Getenv, os.UserHomeDir, os.Stat, os.ReadDir)
			return runSpecContracts(store, repo, collection, jsonOut, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&workspaceDir, "workspace", "", "thư mục workspace (mặc định cwd)") //znf:allow-lang
	cmd.Flags().StringVar(&repo, "repo", "", "lọc theo repo trong _Blast-radius:")            //znf:allow-lang
	cmd.Flags().StringVar(&collection, "collection", "", "lọc theo collection trong _DB:")    //znf:allow-lang
	cmd.Flags().BoolVar(&jsonOut, "json", false, "in JSON thay vì bảng")                      //znf:allow-lang
	return cmd
}
