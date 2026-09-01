package cli

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ghx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/reconcile"
	"github.com/spf13/cobra"
)

// buildPlan runs the read-only reconciler core: auth → list → scan → classify.
// Injected runners make it unit-testable without gh/git.
func buildPlan(m *manifest.Manifest, gh ghx.Runner, git gitx.Runner, workspace string) ([]reconcile.RepoPlan, ghx.Auth, error) {
	auth, err := ghx.CheckAuth(gh)
	if err != nil {
		return nil, auth, err
	}
	if !auth.LoggedIn {
		return nil, auth, nil // let the caller emit the friendly gh-auth-login message
	}
	remote, err := ghx.ListRepos(gh, m.Org)
	if err != nil {
		return nil, auth, err
	}
	access := map[string]ghx.RemoteRepo{}
	for _, r := range remote {
		if r.HasAccess() {
			access[r.Name] = r
		}
	}
	scans := map[string]gitx.RepoState{}
	for _, r := range m.Repos {
		st, err := gitx.Scan(git, filepath.Join(workspace, r.Path))
		if err != nil {
			return nil, auth, err
		}
		scans[r.Name] = st
	}
	return reconcile.Build(m, access, scans), auth, nil
}

// planData is the JSON payload for `up --json`.
type planData struct {
	Account  string               `json:"account"`
	LoggedIn bool                 `json:"logged_in"`
	Repos    []reconcile.RepoPlan `json:"repos"`
}

func renderPlanJSON(w io.Writer, plans []reconcile.RepoPlan, auth ghx.Auth) error {
	return writeJSON(w, planData{Account: auth.Account, LoggedIn: auth.LoggedIn, Repos: plans})
}

func renderPlanTable(w io.Writer, plans []reconcile.RepoPlan, auth ghx.Auth) {
	fmt.Fprintf(w, "Account: %s\n\n", auth.Account)
	fmt.Fprintf(w, "%-22s %-16s %s\n", "REPO", "STATE", "REASON")
	for _, p := range plans {
		fmt.Fprintf(w, "%-22s %-16s %s\n", p.Name, p.State, p.Reason)
	}
	fmt.Fprintln(w, "\n(dry-run — apply lands in a later build; nothing was changed)")
}

func newUpCmd() *cobra.Command {
	var (
		jsonOut        bool
		nonInteractive bool
		dryRun         bool
		workspace      string
		manifestPath   string
		overlayPath    string
	)
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Discover repos and print the onboarding plan (read-only in this build)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if manifestPath == "" {
				// The manifest is versioned inside the kit checkout, not the
				// scanned workspace — default relative to cwd, not --workspace.
				manifestPath = filepath.Join("manifest", "repos.yaml")
			}
			if overlayPath == "" {
				overlayPath = filepath.Join(workspace, ".zenify-overlay.yaml")
			}
			m, err := manifest.LoadWithOverlay(manifestPath, overlayPath)
			if err != nil {
				return exitcode.New(exitcode.Fail, err)
			}
			plans, auth, err := buildPlan(m, ghx.ExecRunner(), gitx.ExecRunner(), workspace)
			if err != nil {
				return exitcode.New(exitcode.Fail, err)
			}
			if !auth.LoggedIn {
				return exitcode.New(exitcode.Fail,
					fmt.Errorf("not logged in to GitHub — run `gh auth login` (need scopes read:org, repo)"))
			}
			if !auth.HasScopes("read:org", "repo") {
				fmt.Fprintln(cmd.ErrOrStderr(),
					"warning: gh token missing read:org or repo scope; discovery may be incomplete")
			}
			if jsonOut {
				return renderPlanJSON(cmd.OutOrStdout(), plans, auth)
			}
			renderPlanTable(cmd.OutOrStdout(), plans, auth)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the plan as a JSON envelope (implies --non-interactive)")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "never prompt")
	cmd.Flags().BoolVar(&dryRun, "dry-run", true, "compute and print the plan without acting (always on in this build)")
	cmd.Flags().StringVar(&workspace, "workspace", ".", "workspace root directory")
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "path to repos.yaml (default manifest/repos.yaml relative to the kit checkout)")
	cmd.Flags().StringVar(&overlayPath, "overlay", "", "path to personal overlay (default <workspace>/.zenify-overlay.yaml)")
	return cmd
}
