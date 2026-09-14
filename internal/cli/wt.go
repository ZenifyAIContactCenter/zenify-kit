package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/wt"
	"github.com/spf13/cobra"
)

// newWtCmd builds the `wt` command tree. C1 wires only the read-only leaves;
// the mutating `new`/`rm`/`sweep` arrive in later slices.
func newWtCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wt",
		Short: "Quản lý git worktree + môi trường dev: tạo, liệt kê, gỡ, dọn worktree theo slug", //znf:allow-lang
	}
	cmd.AddCommand(newWtPathCmd(), newWtConfigCmd(), newWtNewCmd(), newWtLsCmd(), newWtUrlCmd(), newWtRmCmd(), newWtSweepCmd(), newWtWireCmd(), newWtPromoteCmd())
	return cmd
}

// repoRoot resolves the repo root. A test seam (WT_REPO_ROOT) lets unit tests
// skip git; otherwise it shells `git rev-parse --show-toplevel` from cwd.
func repoRoot() (string, error) {
	if r := os.Getenv("WT_REPO_ROOT"); r != "" {
		return r, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	b, err := gitx.ExecRunner().Run(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("wt: not inside a git repo: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}

func newWtPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path <slug>",
		Short: "In đường dẫn tuyệt đối của worktree theo slug", //znf:allow-lang
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := repoRoot()
			if err != nil {
				return err
			}
			st, err := wt.ReadState(root)
			if err != nil {
				return err
			}
			w, ok := st.Find(args[0])
			if !ok {
				return fmt.Errorf("wt: no worktree %q", args[0])
			}
			// Guard empty Path: filepath.Join(root, "") == root, which would print
			// the repo root as if it were a valid worktree path. A state entry with
			// no path is malformed — fail loudly rather than emit a plausible lie.
			// (C2 starts writing state.json; this closes the hole before then.)
			if strings.TrimSpace(w.Path) == "" {
				return fmt.Errorf("wt: worktree %q has no path in state", args[0])
			}
			// Output contract: EXACTLY the path + one newline, nothing else —
			// /cook captures this stdout as a path. Fprintln adds the newline;
			// keep every diagnostic on stderr (SilenceUsage below).
			p := w.Path
			if !filepath.IsAbs(p) {
				p = filepath.Join(root, p)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), p)
			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
}

func newWtConfigCmd() *cobra.Command {
	var portKey string
	c := &cobra.Command{
		Use:   "config",
		Short: "Hiện worktree.json đã resolve (hoặc --port <key> để xem port đã cấp)", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := repoRoot()
			if err != nil {
				return err
			}
			cfg, err := wt.Load(root)
			if err != nil {
				return err
			}
			if portKey != "" {
				// Build the taken set from state so --port reflects reality.
				st, err := wt.ReadState(root)
				if err != nil {
					return err
				}
				taken := map[int]bool{}
				for _, w := range st.Worktrees {
					for _, p := range w.Ports {
						taken[p] = true
					}
				}
				key := fmt.Sprintf("%s:%s:%s", filepath.Base(root), portKey, cfg.PortEnv)
				p, ok := wt.Allocate(key, cfg.PortRange[0], cfg.PortRange[1], taken)
				if !ok {
					return fmt.Errorf("wt: no free port in %v for %q", cfg.PortRange, portKey)
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), p)
				return nil
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(),
				"abbrev=%s\nbaseRef=%s\nworktreeDir=%s\nportEnv=%s\nportRange=%d %d\ndeps=%s\nuser=%s\n",
				cfg.Abbrev, cfg.BaseRef, cfg.WorktreeDir, cfg.PortEnv,
				cfg.PortRange[0], cfg.PortRange[1], cfg.Deps, cfg.User)
			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	c.Flags().StringVar(&portKey, "port", "", "print the allocated port for this key instead of the full config")
	return c
}

func newWtNewCmd() *cobra.Command {
	var typ, base string
	var forceInstall, another bool
	c := &cobra.Command{
		Use:   "new <slug>",
		Short: "Tạo worktree: branch + port + env đã seed + deps", //znf:allow-lang
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := repoRoot()
			if err != nil {
				return err
			}
			host, _ := os.Hostname()
			return wt.RunNew(wt.NewOptions{
				RepoRoot:     root,
				Slug:         args[0],
				Type:         typ,
				BaseOverride: base,
				ForceInstall: forceInstall,
				Another:      another,
				Host:         host,
				Pid:          os.Getpid(),
				Now:          time.Now().Unix(),
				Runner:       gitx.ExecRunner(),
				Stderr:       cmd.ErrOrStderr(),
			})
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	c.Flags().StringVar(&typ, "type", "feat", "feat|fix|chore|hotfix")
	c.Flags().StringVar(&base, "base", "", "override the base ref (e.g. this week's release)")
	c.Flags().BoolVar(&forceInstall, "install", false, "force deps=install even if config says symlink/clone")
	c.Flags().BoolVar(&another, "another", false, "allow a second concurrent task in this repo")
	return c
}

func newWtLsCmd() *cobra.Command {
	var asJSON, all bool
	c := &cobra.Command{
		Use:   "ls",
		Short: "Liệt kê worktree trong repo này (git ⋈ state), kèm trạng thái running/merged", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if all {
				cwd, _ := os.Getwd()
				ws, ok := wt.FindWorkspaceRoot(cwd)
				if !ok {
					return fmt.Errorf("wt: not inside a zenify workspace (no .zenify/manifest.json above cwd)")
				}
				var allRows []wt.Row
				w := cmd.OutOrStdout()
				skipw := w
				if asJSON {
					skipw = cmd.ErrOrStderr()
				}
				for _, repo := range wt.WorkspaceRepos(ws) {
					name := filepath.Base(repo)
					cfg, err := wt.Load(repo)
					if err != nil {
						_, _ = fmt.Fprintf(skipw, "== %s == (skipped: %v)\n", name, err)
						continue
					}
					rows, err := wt.List(gitx.ExecRunner(), repo, cfg)
					if err != nil {
						_, _ = fmt.Fprintf(skipw, "== %s == (skipped: %v)\n", name, err)
						continue
					}
					if len(rows) == 0 {
						continue
					}
					for i := range rows {
						rows[i].Repo = name
					}
					if asJSON {
						allRows = append(allRows, rows...)
						continue
					}
					_, _ = fmt.Fprintf(w, "== %s (%s) ==\n", cfg.Abbrev, repo)
					printRows(w, rows)
				}
				if asJSON {
					if allRows == nil {
						allRows = []wt.Row{}
					}
					enc := json.NewEncoder(w)
					enc.SetIndent("", "  ")
					return enc.Encode(allRows)
				}
				return nil
			}
			root, err := repoRoot()
			if err != nil {
				return err
			}
			cfg, err := wt.Load(root)
			if err != nil {
				return err
			}
			rows, err := wt.List(gitx.ExecRunner(), root, cfg)
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if asJSON {
				enc := json.NewEncoder(w)
				enc.SetIndent("", "  ")
				if rows == nil {
					rows = []wt.Row{}
				}
				return enc.Encode(rows)
			}
			if len(rows) == 0 {
				_, _ = fmt.Fprintln(w, "wt: no tasks in this repo")
				return nil
			}
			printRows(w, rows)
			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	c.Flags().BoolVar(&asJSON, "json", false, "emit the rows as a JSON array (editor-agnostic)")
	c.Flags().BoolVar(&all, "all", false, "list every wt-managed repo in the workspace, grouped by repo")
	return c
}

// printRows renders the human-readable worktree table shared by `wt ls` and
// `wt ls --all`.
func printRows(w io.Writer, rows []wt.Row) {
	_, _ = fmt.Fprintf(w, "%-14s %-28s %-6s %-8s %-7s %-8s %-5s %s\n", "SLUG", "BRANCH", "PORT", "DEPS", "MERGED", "RUNNING", "STALE", "PATH")
	for _, r := range rows {
		_, _ = fmt.Fprintf(w, "%-14s %-28s %-6s %-8s %-7s %-8s %-5s %s\n", r.Slug, r.Branch, r.Port, r.Deps, r.Merged, r.Running, staleWord(r.Stale), r.Path)
	}
}

func staleWord(b bool) string {
	if b {
		return "yes"
	}
	return ""
}

func newWtUrlCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "url <slug>",
		Short: "In http://localhost:<port> của một slug", //znf:allow-lang
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := repoRoot()
			if err != nil {
				return err
			}
			cfg, err := wt.Load(root)
			if err != nil {
				return err
			}
			u, err := wt.URLFor(gitx.ExecRunner(), root, cfg, args[0])
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), u) // exact stdout contract, like `wt path`
			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
}

func newWtRmCmd() *cobra.Command {
	var force bool
	c := &cobra.Command{
		Use:   "rm <slug>",
		Short: "Gỡ một worktree (từ chối worktree dirty/detached/chưa merge nếu không có --force)", //znf:allow-lang
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := repoRoot()
			if err != nil {
				return err
			}
			host, _ := os.Hostname()
			return wt.RunRm(wt.RmOptions{
				RepoRoot: root,
				Slug:     args[0],
				Force:    force,
				Host:     host,
				Pid:      os.Getpid(),
				Now:      time.Now().Unix(),
				Runner:   gitx.ExecRunner(),
				Stderr:   cmd.ErrOrStderr(),
			})
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	c.Flags().BoolVarP(&force, "force", "f", false, "remove even if dirty, detached, or unmerged")
	return c
}

func newWtPromoteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "promote <slug>",
		Short: "Chuyển node_modules symlink của worktree thành bản copy CoW riêng", //znf:allow-lang
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := repoRoot()
			if err != nil {
				return err
			}
			return wt.RunPromote(wt.PromoteOptions{
				RepoRoot: root,
				Slug:     args[0],
				Runner:   gitx.ExecRunner(),
				Stdout:   cmd.OutOrStdout(),
				Stderr:   cmd.ErrOrStderr(),
			})
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
}

func newWtSweepCmd() *cobra.Command {
	var dry, fetch, all bool
	c := &cobra.Command{
		Use:   "sweep",
		Short: "Dọn mọi worktree đã merge và sạch trong repo này (hoặc cả workspace với --all)", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if all {
				cwd, _ := os.Getwd()
				ws, ok := wt.FindWorkspaceRoot(cwd)
				if !ok {
					return fmt.Errorf("wt: not inside a zenify workspace (no .zenify/manifest.json above cwd)")
				}
				if fetch {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "wt: --all always fetches; --fetch is redundant here")
				}
				host, _ := os.Hostname()
				return wt.RunSweepAll(wt.SweepAllOptions{
					WorkspaceRoot: ws, Host: host, DryRun: dry, Pid: os.Getpid(), Now: time.Now().Unix(),
					Runner: gitx.ExecRunner(), FetchRunner: gitx.TimeoutRunner(5 * time.Second),
					Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(),
				})
			}
			root, err := repoRoot()
			if err != nil {
				return err
			}
			host, _ := os.Hostname()
			return wt.RunSweep(wt.SweepOptions{
				RepoRoot: root, Host: host, DryRun: dry, Fetch: fetch,
				Pid: os.Getpid(), Now: time.Now().Unix(),
				Runner: gitx.ExecRunner(), Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(),
			})
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	c.Flags().BoolVarP(&dry, "dry-run", "n", false, "report what would be removed without touching anything")
	c.Flags().BoolVarP(&fetch, "fetch", "f", false, "fetch origin first so merge state is current")
	c.Flags().BoolVar(&all, "all", false, "sweep every wt-managed repo in the workspace (fetches each, 5s timeout, fail-open per repo)")
	return c
}

// mainCheckoutOf returns the main checkout root given any worktree toplevel wt.
// git-common-dir points at the shared .git; its parent is the main checkout. A
// relative result (".git") means wt already IS the main checkout.
func mainCheckoutOf(wtTop string) (string, error) {
	b, err := gitx.ExecRunner().Run(wtTop, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	cd := strings.TrimSpace(string(b))
	if !filepath.IsAbs(cd) {
		cd = filepath.Join(wtTop, cd) // ".git" → <wtTop>/.git
	}
	return filepath.Dir(cd), nil
}

func newWtWireCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "wire",
		Short: "Trỏ file env của worktree này sang các peer service đang được sửa", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			wtTop, err := repoRoot() // from inside a worktree = the worktree toplevel
			if err != nil {
				return err
			}
			mainRoot, err := mainCheckoutOf(wtTop)
			if err != nil {
				return err
			}
			return wt.RunWire(wt.WireOptions{
				RepoRoot:     mainRoot,
				WorktreePath: wtTop,
				DryRun:       dryRun,
				Runner:       gitx.ExecRunner(),
				Stdout:       cmd.OutOrStdout(),
				Stderr:       cmd.ErrOrStderr(),
			})
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "show what would change without writing")
	return cmd
}
