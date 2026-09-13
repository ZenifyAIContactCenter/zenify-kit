package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/docsview"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ghx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/lock"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/playwright"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/reconcile"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// buildPlan runs the read-only reconciler core: auth → list → scan → classify.
// Injected runners make it unit-testable without gh/git. sources is the Where
// step's answer (nil when it did not run) — merged with the legacy flat-layout
// guess (flatSources), an explicit source winning over the guess.
func buildPlan(m *manifest.Manifest, gh ghx.Runner, git gitx.Runner, workspace string, sources map[string]reconcile.Source) ([]reconcile.RepoPlan, ghx.Auth, error) {
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
	merged := flatSources(m, git, workspace)
	for k, v := range sources {
		merged[k] = v // an explicit Where answer wins over the flat-layout guess
	}
	plans := reconcile.Build(m, access, scans, merged)
	return orderRelocateFirst(plans), auth, nil
}

// orderRelocateFirst moves RELOCATE plans ahead of everything else so a
// destination is never taken by a clone before the move lands (FR-4.4).
// Stable: relative order inside each half is preserved.
func orderRelocateFirst(plans []reconcile.RepoPlan) []reconcile.RepoPlan {
	out := make([]reconcile.RepoPlan, 0, len(plans))
	for _, p := range plans {
		if p.State == reconcile.Relocate {
			out = append(out, p)
		}
	}
	for _, p := range plans {
		if p.State != reconcile.Relocate {
			out = append(out, p)
		}
	}
	return out
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
	_, _ = fmt.Fprintf(w, "Account: %s\n\n", auth.Account)
	_, _ = fmt.Fprintf(w, "%-22s %-16s %s\n", "REPO", "STATE", "REASON")
	for _, p := range plans {
		_, _ = fmt.Fprintf(w, "%-22s %-16s %s\n", p.Name, p.State, p.Reason)
	}
	_, _ = fmt.Fprintln(w, "\n(dry-run — nothing was changed; run with --apply or in a terminal to apply)")
}

// minVersionFloor is the binary version that introduced the apply path. A
// binary older than this refuses to mutate (FR-004). A "dev" build is never
// blocked (see version.MeetsMin). Written v-prefixed; goreleaser injects
// {{.Version}} WITHOUT the prefix ("0.17.2") and MeetsMin normalises both. Set to the
// current public floor so the gate is real (a pre-0.3.0 binary is blocked) yet
// can never self-block: any release carrying apply is >= v0.3.0.
const minVersionFloor = "v0.3.0"

// docsRemote is the knowledge-store repo `zenify up --apply` clones on a
// brand-new machine when the store is absent. The on-disk store is named
// "docs" (M6a; see defaultDocsRepo in docs.go), but the GitHub repo itself
// is still named zenify-knowledge — that split is intentional, not a
// mismatch to "fix".
const docsRemote = "git@github.com:ZenifyAIContactCenter/zenify-knowledge.git"

// ensureDocsStore clones the docs knowledge store to its resolved path
// (resolveDocsStore) when absent, then reconciles the workspace view
// (docsview.EnsureView) against it. Onboarding convenience: FAIL-OPEN. A
// clone failure or a view failure only warns — it must never abort `up
// --apply`, which is why this returns nothing.
func ensureDocsStore(w io.Writer, errW io.Writer, git gitx.Runner, workspace string) {
	store := resolveDocsStore(workspace, os.Getenv, os.UserHomeDir, os.Stat, os.ReadDir)
	if fi, err := os.Stat(filepath.Join(store, ".git")); err != nil || !fi.IsDir() {
		// gitx.Runner always runs `git -C <dir> ...`, and `git -C` fails
		// immediately if <dir> does not exist yet. On a genuinely fresh
		// machine (no prior ~/.zenify at all) filepath.Dir(store) is
		// ~/.zenify, which nothing else creates — so the parent must be
		// created before the clone can run. Failure here is itself
		// fail-open: git.Run below will just fail (and warn) the same way
		// it would for any other clone error.
		if err := os.MkdirAll(filepath.Dir(store), 0o750); err != nil {
			_, _ = fmt.Fprintf(errW, "warning: docs store parent dir: %v (onboarding otherwise succeeded)\n", err)
		}
		if _, err := git.Run(filepath.Dir(store), "clone", docsRemote, store); err != nil {
			_, _ = fmt.Fprintf(errW, "warning: docs store clone: %v (onboarding otherwise succeeded)\n", err)
		}
	}
	viewDir := filepath.Join(workspace, defaultDocsRepo)
	if viewDir == store {
		return // not migrated yet — store IS the workspace docs dir, nothing to link
	}
	for _, n := range docsview.EnsureView(docsview.OSFS{}, store, viewDir) {
		_, _ = fmt.Fprintln(w, n)
	}
}

// runApply executes the actionable plans under the full b2a safety sequence:
// version gate → workspace lock → pre-mutation snapshot → apply → persist the
// ownership manifest. The lock is released on return.
func runApply(w io.Writer, errW io.Writer, plans []reconcile.RepoPlan, m *manifest.Manifest, workspace string, gh ghx.Runner, git gitx.Runner) error {
	if err := version.GuardMutation(version.Current(), minVersionFloor); err != nil {
		return exitcode.New(exitcode.Fail, err)
	}

	zenifyDir := filepath.Join(workspace, ".zenify")
	if err := os.MkdirAll(zenifyDir, 0o750); err != nil {
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
	defer func() { _ = h.Release() }()

	// Load (or start) the ownership manifest, then write a pre-mutation snapshot
	// of the files this run touches (FR-022 capture half; a future `zenify
	// backups restore`, FR-052, is the consumer). This whole-run snapshot stays a
	// manual-restore-only backup: nothing here auto-restores from it on failure.
	// FR-9a has since wired per-repo auto-restore-on-failure (stage → verify →
	// restore inside apply.Apply), but it operates on a SEPARATE per-repo
	// "txn-<name>" snapshot (see apply.go), not on the whole-run snapshot taken
	// here — so item #2 below (scope restore to the failed repo's paths) is DONE
	// there, not a TODO. Every action here is create-if-absent (settings
	// skeleton, clone) or append-if-absent (exclude), so a partial run is safe to
	// re-run and there is nothing destructive to roll back.
	//
	// RELOCATE moves are logged to <snapshot>/relocate.json (writeRelocateLog);
	// the whole-run snapshot itself only holds files.
	//
	// The snapshot id is unique per run — the unix second plus the pid — so even
	// two runs within one second never overwrite an earlier capture.
	manifestPath := filepath.Join(zenifyDir, "manifest.json")
	owned, err := managed.Load(manifestPath)
	if err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	snapshotID := fmt.Sprintf("apply-%d-%d", applyNow(), os.Getpid())
	if _, err := managed.Snapshot(snapshotID, snapshotTargets(plans, workspace), filepath.Join(zenifyDir, "snapshots")); err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	writeRelocateLog(filepath.Join(zenifyDir, "snapshots", snapshotID), plans, errW)

	repoByName := map[string]manifest.Repo{}
	for _, r := range m.Repos {
		repoByName[r.Name] = r
	}
	results, err := apply.Apply(plans, apply.Options{
		Workspace: workspace, Org: m.Org, Owned: owned, RepoByName: repoByName, SecretKeys: m.SecretKeys,
		SnapshotRoot: filepath.Join(zenifyDir, "snapshots"),
		ManifestPath: manifestPath,
		Now:          applyNow,
	}, gh, git)
	if err != nil {
		return exitcode.New(exitcode.Fail, err)
	}

	var failed int
	for _, r := range results {
		if r.Err != nil {
			failed++
			if r.Action != "" {
				_, _ = fmt.Fprintf(w, "%-22s %-16s %s — ERROR: %v\n", r.Repo, r.State, r.Action, r.Err)
			} else {
				_, _ = fmt.Fprintf(w, "%-22s %-16s ERROR: %v\n", r.Repo, r.State, r.Err)
			}
			continue
		}
		_, _ = fmt.Fprintf(w, "%-22s %-16s %s\n", r.Repo, r.State, r.Action)
	}

	if err := owned.Save(manifestPath); err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	if err := writeWorkspacePointer(os.Getenv, os.UserHomeDir, workspace); err != nil {
		_, _ = fmt.Fprintln(errW, "warning: could not record workspace pointer:", err)
	}

	// Provision the Playwright MCP + browsers when onboarding a frontend repo
	// (FR-014). NON-FATAL: a failure warns but does not fail onboarding or the
	// failed-count return — mirrors the gh-scope warning.
	if hasFrontendRepo(m) {
		po := playwright.Options{
			Runner: func(name string, args []string) error { return exec.Command(name, args...).Run() }, //nolint:gosec // G204 -- fixed trusted binary, args are internally-computed subcommands, not attacker-controlled shell input
			Getenv: os.Getenv,
			GOOS:   runtime.GOOS,
			Stdout: w,
		}
		if err := playwright.Bootstrap(po); err != nil {
			_, _ = fmt.Fprintf(errW, "warning: playwright bootstrap: %v (onboarding otherwise succeeded)\n", err)
		}
	}

	// Onboarding convenience (Task 5): clone the docs knowledge store on a
	// brand-new machine when it's absent, then reconcile the workspace view.
	// FAIL-OPEN — never affects `failed` or the return below.
	ensureDocsStore(w, errW, git, workspace)

	// Hooks + model pin + store config distribution, all fail-open (W0 FR-01.4);
	// never affects `failed`.
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		ensureWorkspace(workspace, home, w, errW)
	}

	if failed > 0 {
		return exitcode.New(exitcode.Fail, fmt.Errorf("apply: %d repo(s) failed", failed))
	}
	return nil
}

// hasFrontendRepo reports whether the manifest onboards any frontend repo,
// gating Playwright provisioning (FR-014) to FE workspaces.
func hasFrontendRepo(m *manifest.Manifest) bool {
	for _, r := range m.Repos {
		for _, t := range r.Tags {
			if t == "frontend" {
				return true
			}
		}
	}
	return false
}

// snapshotTargets lists the files an apply run touches, so Snapshot can capture
// their pre-mutation state for recovery. It lists settings.local.json and the
// exclude file per actionable repo; Snapshot skips any that do not yet exist,
// so a freshly created skeleton or a fresh clone contributes nothing to capture.
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
	if home, err := os.UserHomeDir(); err == nil {
		files = append(files, filepath.Join(home, ".claude", "settings.json"))
	}
	return files
}

// writeRelocateLog records every planned move next to the run's snapshot, so
// a hand rollback knows what was moved where (FR-4.4). Fail-open.
func writeRelocateLog(snapDir string, plans []reconcile.RepoPlan, errW io.Writer) {
	type mv struct{ From, To string }
	var moves []mv
	for _, p := range plans {
		if p.State == reconcile.Relocate {
			moves = append(moves, mv{From: p.From, To: p.Path})
		}
	}
	if len(moves) == 0 {
		return
	}
	b, _ := json.MarshalIndent(moves, "", "  ")
	if err := os.WriteFile(filepath.Join(snapDir, "relocate.json"), b, 0o600); err != nil {
		_, _ = fmt.Fprintln(errW, "warning: relocate log:", err)
	}
}

// applyNow returns the current unix time for the lock's diagnostic sidecar.
// Isolated so the value is injected in one place (tests do not call runApply's
// clock directly; the sidecar time is not asserted).
func applyNow() int64 { return time.Now().Unix() }

// dryRunApplyConflict rejects `--apply --dry-run` (i.e. --dry-run=true) together:
// --apply mutates and --dry-run (FR-050) is the non-mutating preview, so
// combining them is a contradiction. dryRunChanged is cmd.Flags().Changed(
// "dry-run") — true only when the user set --dry-run explicitly; dryRun is its
// value. The conflict needs all three: an explicit --dry-run set to true,
// alongside --apply. So the default (dry-run true, not set, no --apply) and the
// coherent `--apply --dry-run=false` (explicitly asking to mutate) both pass.
func dryRunApplyConflict(apply, dryRunChanged, dryRun bool) error {
	if apply && dryRunChanged && dryRun {
		return fmt.Errorf("cannot combine --apply with --dry-run: --apply mutates, --dry-run only previews")
	}
	return nil
}

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
		Short: "Onboard the workspace: interactive wizard in a terminal, dry-run plan otherwise (use --apply to execute headless)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := dryRunApplyConflict(applyFlag, cmd.Flags().Changed("dry-run"), dryRun); err != nil {
				return exitcode.New(exitcode.BadArgs, err)
			}
			cwd, _ := os.Getwd()
			isTTY := term.IsTerminal(int(os.Stdout.Fd()))
			wantWizard := decideMode(isTTY, applyFlag, cmd.Flags().Changed("dry-run"), dryRun, jsonOut, nonInteractive) == modeWizard
			var sources map[string]reconcile.Source
			ws, _, ok := resolveWorkspace(cwd, workspace, os.Getenv, os.UserHomeDir)
			if !ok {
				if !wantWizard {
					return exitcode.New(exitcode.BadArgs, errors.New("chưa có workspace: dùng --workspace <dir> hoặc chạy zenify up có terminal")) //znf:allow-lang
				}
				// Task 8 fills this in: ws, sources = the Where step's answer.
				return exitcode.New(exitcode.BadArgs, errors.New("chưa có workspace: dùng --workspace <dir>")) //znf:allow-lang
			}
			workspace = ws
			if overlayPath == "" {
				overlayPath = filepath.Join(workspace, ".zenify-overlay.yaml")
			}
			w := cmd.OutOrStdout()
			// A preview invocation (anything short of --apply) must show the
			// HOOKS / DOCS-STORE synthetic rows even when the manifest fails
			// to load or buildPlan errors below — both rows are derived from
			// home dir + workspace only (manifest/gh-independent), so there
			// is no reason to gate them behind a successful repo-plan build
			// (SC-10 dry-run parity).
			isPreview := !applyFlag
			m, _, err := loadKitManifest(manifestPath, overlayPath)
			if err != nil {
				if isPreview {
					printPlanFooterRows(w, workspace)
				}
				return exitcode.New(exitcode.Fail, err)
			}
			plans, auth, err := buildPlan(m, ghx.ExecRunner(), gitx.ExecRunner(), workspace, sources)
			if err != nil {
				if isPreview {
					printPlanFooterRows(w, workspace)
				}
				return exitcode.New(exitcode.Fail, err)
			}
			if !auth.LoggedIn {
				if isPreview {
					printPlanFooterRows(w, workspace)
				}
				return exitcode.New(exitcode.Fail,
					fmt.Errorf("not logged in to GitHub — run `gh auth login` (need scopes read:org, repo)"))
			}
			if !auth.HasScopes("read:org", "repo") {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(),
					"warning: gh token missing read:org or repo scope; discovery may be incomplete")
			}
			switch decideMode(isTTY, applyFlag, cmd.Flags().Changed("dry-run"), dryRun, jsonOut, nonInteractive) {
			case modeWizard:
				return runWizard(w, m, workspace, sources)
			case modeApply:
				return runApply(w, cmd.ErrOrStderr(), plans, m, workspace, ghx.ExecRunner(), gitx.ExecRunner())
			default: // modeDryRun
				if jsonOut {
					return renderPlanJSON(w, plans, auth)
				}
				renderPlanTable(w, plans, auth)
				printPlanFooterRows(w, workspace)
				return nil
			}
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the plan as a JSON envelope")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "never prompt — forces the headless dry-run/apply path instead of the interactive wizard")
	cmd.Flags().BoolVar(&dryRun, "dry-run", true, "preview the plan without making changes")
	cmd.Flags().StringVar(&workspace, "workspace", "", "workspace root (default: the workspace found from cwd or ~/.zenify/workspace; the wizard asks when there is none)")
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "path to repos.yaml (default: manifest/repos.yaml under cwd when present, else the copy embedded in the binary)")
	cmd.Flags().StringVar(&overlayPath, "overlay", "", "path to personal overlay (default <workspace>/.zenify-overlay.yaml)")
	cmd.Flags().BoolVar(&applyFlag, "apply", false, "apply changes without the interactive wizard (required for non-interactive/CI runs)")
	return cmd
}
