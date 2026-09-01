package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ghx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/lock"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/reconcile"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
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

// minVersionFloor is the binary version that introduced the apply path. A
// binary older than this refuses to mutate (FR-004). A "dev" build is never
// blocked (see version.MeetsMin). Format is v-prefixed semver to match
// version.Current() (goreleaser injects {{.Version}} like "v0.3.0"). Set to the
// current public floor so the gate is real (a pre-0.3.0 binary is blocked) yet
// can never self-block: any release carrying apply is >= v0.3.0.
const minVersionFloor = "v0.3.0"

// runApply executes the actionable plans under the full b2a safety sequence:
// version gate → workspace lock → pre-mutation snapshot → apply → persist the
// ownership manifest. The lock is released on return.
func runApply(w io.Writer, plans []reconcile.RepoPlan, m *manifest.Manifest, workspace string, gh ghx.Runner, git gitx.Runner) error {
	if err := version.GuardMutation(version.Current(), minVersionFloor); err != nil {
		return exitcode.New(exitcode.Fail, err)
	}

	zenifyDir := filepath.Join(workspace, ".zenify")
	if err := os.MkdirAll(zenifyDir, 0o755); err != nil {
		return exitcode.New(exitcode.Fail, err)
	}

	host, _ := os.Hostname()
	h, err := lock.Acquire(zenifyDir, os.Getpid(), host, applyNow())
	if err != nil {
		if errors.Is(err, lock.ErrHeld) {
			return exitcode.New(exitcode.LockHeld, err)
		}
		return exitcode.New(exitcode.Fail, err)
	}
	defer h.Release()

	// Load (or start) the ownership manifest, and snapshot the files WIRE will
	// touch so a failed run can be rolled back (FR-022).
	manifestPath := filepath.Join(zenifyDir, "manifest.json")
	owned, err := managed.Load(manifestPath)
	if err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	if _, err := managed.Snapshot("apply", snapshotTargets(plans, workspace), filepath.Join(zenifyDir, "snapshots")); err != nil {
		return exitcode.New(exitcode.Fail, err)
	}

	repoByName := map[string]manifest.Repo{}
	for _, r := range m.Repos {
		repoByName[r.Name] = r
	}
	results, err := apply.Apply(plans, apply.Options{
		Workspace: workspace, Org: m.Org, Owned: owned, RepoByName: repoByName,
	}, gh, git)
	if err != nil {
		return exitcode.New(exitcode.Fail, err)
	}

	var failed int
	for _, r := range results {
		if r.Err != nil {
			failed++
			fmt.Fprintf(w, "%-22s %-16s ERROR: %v\n", r.Repo, r.State, r.Err)
			continue
		}
		fmt.Fprintf(w, "%-22s %-16s %s\n", r.Repo, r.State, r.Action)
	}

	if err := owned.Save(manifestPath); err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	if failed > 0 {
		return exitcode.New(exitcode.Fail, fmt.Errorf("apply: %d repo(s) failed", failed))
	}
	return nil
}

// snapshotTargets lists the files a WIRE/CLONE run may overwrite, so Snapshot
// can capture their pre-mutation state. It lists settings.local.json and the
// exclude file per actionable repo; Snapshot skips any that do not yet exist.
func snapshotTargets(plans []reconcile.RepoPlan, workspace string) []string {
	var files []string
	for _, p := range plans {
		switch p.State {
		case reconcile.Clone, reconcile.Wire, reconcile.Adopt:
			repoDir := filepath.Join(workspace, p.Path)
			files = append(files,
				filepath.Join(repoDir, ".claude", "settings.local.json"),
				filepath.Join(repoDir, ".git", "info", "exclude"),
			)
		}
	}
	return files
}

// applyNow returns the current unix time for the lock's diagnostic sidecar.
// Isolated so the value is injected in one place (tests do not call runApply's
// clock directly; the sidecar time is not asserted).
func applyNow() int64 { return time.Now().Unix() }

func newUpCmd() *cobra.Command {
	var (
		jsonOut        bool
		nonInteractive bool
		dryRun         bool
		workspace      string
		manifestPath   string
		overlayPath    string
		applyFlag      bool
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
			if applyFlag {
				return runApply(cmd.OutOrStdout(), plans, m, workspace, ghx.ExecRunner(), gitx.ExecRunner())
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
	cmd.Flags().BoolVar(&applyFlag, "apply", false, "execute the plan (clone/wire/adopt); without this, up is dry-run")
	return cmd
}
