package cli

import (
	"encoding/json"
	"fmt"
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
		Short: "Git worktree + dev-env manager (read-only surface in this build)",
	}
	cmd.AddCommand(newWtPathCmd(), newWtConfigCmd(), newWtNewCmd(), newWtLsCmd(), newWtUrlCmd(), newWtRmCmd(), newWtSweepCmd())
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
		Short: "Print the absolute worktree path for a slug",
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
			fmt.Fprintln(cmd.OutOrStdout(), p)
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
		Short: "Show resolved worktree.json (or --port <key> for an allocated port)",
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
				fmt.Fprintln(cmd.OutOrStdout(), p)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(),
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
		Short: "Create a worktree: branch + port + seeded env + deps",
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
	var asJSON bool
	c := &cobra.Command{
		Use:   "ls",
		Short: "List worktrees in this repo (git ⋈ state), with running/merged status",
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
				fmt.Fprintln(w, "wt: no tasks in this repo")
				return nil
			}
			fmt.Fprintf(w, "%-14s %-28s %-6s %-8s %-7s %-8s %s\n", "SLUG", "BRANCH", "PORT", "DEPS", "MERGED", "RUNNING", "PATH")
			for _, r := range rows {
				fmt.Fprintf(w, "%-14s %-28s %-6s %-8s %-7s %-8s %s\n", r.Slug, r.Branch, r.Port, r.Deps, r.Merged, r.Running, r.Path)
			}
			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	c.Flags().BoolVar(&asJSON, "json", false, "emit the rows as a JSON array (editor-agnostic)")
	return c
}

func newWtUrlCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "url <slug>",
		Short: "Print http://localhost:<port> for a slug",
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
			fmt.Fprintln(cmd.OutOrStdout(), u) // exact stdout contract, like `wt path`
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
		Short: "Remove a worktree (refuses a dirty/detached/unmerged one without --force)",
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

func newWtSweepCmd() *cobra.Command {
	var dry, fetch bool
	c := &cobra.Command{
		Use:   "sweep",
		Short: "Tear down every merged, clean worktree in this repo",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
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
	return c
}
