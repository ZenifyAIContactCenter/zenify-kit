package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	cmd.AddCommand(newWtPathCmd(), newWtConfigCmd())
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
