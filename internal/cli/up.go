package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/apply"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/devenv"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/docsview"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ghx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/lock"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/mcpreg"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/playwright"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/reconcile"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/tui"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ui"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ui/applyview"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
	tea "github.com/charmbracelet/bubbletea"
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
	merged = dropInPlaceSources(m, workspace, merged)
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
	u := ui.New(w)
	u.KV([][2]string{{"Account", auth.Account}})
	u.Blank()
	rows := make([][]string, 0, len(plans))
	for _, p := range plans {
		rows = append(rows, []string{p.Name, string(p.State), p.Reason})
	}
	u.Table([]string{"REPO", "STATE", "REASON"}, rows)
	u.Blank()
	_, _ = fmt.Fprintln(w, "(dry-run — nothing was changed; run with --apply or in a terminal to apply)")
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
		sp := ui.NewSpinner(errW, "Cloning docs knowledge store")
		sp.Start()
		if _, err := git.Run(filepath.Dir(store), "clone", docsRemote, store); err != nil {
			sp.Fail(fmt.Sprintf("docs store clone: %v (onboarding otherwise succeeded)", err))
		} else {
			sp.Success("Cloned docs knowledge store")
		}
	}
	viewDir := filepath.Join(workspace, defaultDocsRepo)
	if viewDir != store { // not migrated yet when equal — store IS the workspace docs dir, nothing to link
		for _, n := range docsview.EnsureView(docsview.OSFS{}, store, viewDir) {
			_, _ = fmt.Fprintln(w, n)
		}
	}
	for _, n := range ensureLocalDirs(viewDir) {
		_, _ = fmt.Fprintln(w, n)
	}
}

// runApply executes the actionable plans under the full b2a safety sequence:
// version gate → workspace lock → pre-mutation snapshot → apply → persist the
// ownership manifest. The lock is released on return.
func runApply(w io.Writer, errW io.Writer, plans []reconcile.RepoPlan, m *manifest.Manifest, src, workspace string, gh ghx.Runner, git gitx.Runner) error {
	return runApplyWith(w, errW, plans, m, src, workspace, gh, git, false)
}

// runApplyWith is runApply with --non-interactive threaded through: when set, no step may
// launch an interactive child process even from a terminal.
func runApplyWith(w io.Writer, errW io.Writer, plans []reconcile.RepoPlan, m *manifest.Manifest, src, workspace string, gh ghx.Runner, git gitx.Runner, nonInteractive bool) error {
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
	writeRelocateLog(filepath.Join(zenifyDir, "snapshots", snapshotID), workspace, plans, errW)

	repoByName := map[string]manifest.Repo{}
	for _, r := range m.Repos {
		repoByName[r.Name] = r
	}

	// live drives the animated applyview only on a real styled TTY; off-TTY
	// (pipe/NO_COLOR/tests) keeps the static Step lines below, byte-clean.
	uApply := ui.New(w)
	live := uApply.Styled()

	var prog *tea.Program
	var progDone chan struct{}
	if live {
		model := applyview.New(len(plans))
		prog = tea.NewProgram(model, tea.WithOutput(w))
		progDone = make(chan struct{})
		go func() { _, _ = prog.Run(); close(progDone) }()
	}

	opts := apply.Options{
		Workspace: workspace, Org: m.Org, Owned: owned, RepoByName: repoByName,
		SnapshotRoot: filepath.Join(zenifyDir, "snapshots"),
		ManifestPath: manifestPath,
		Now:          applyNow,
	}
	if live {
		opts.OnProgress = func(done, total int, repo string, state reconcile.State, failed bool) {
			prog.Send(applyview.ProgressMsg{Done: done, Total: total, Repo: repo, State: state, Failed: failed})
		}
	}
	results, err := apply.Apply(plans, opts, gh, git)

	if live {
		prog.Send(applyview.DoneMsg{})
		<-progDone
	}
	if err != nil {
		return exitcode.New(exitcode.Fail, err)
	}

	var failed int
	if !live {
		for _, r := range results {
			label := fmt.Sprintf("%s (%s)", r.Repo, r.State)
			if r.Err != nil {
				failed++
				detail := fmt.Sprintf("ERROR: %v", r.Err)
				if r.Action != "" {
					detail = fmt.Sprintf("%s — ERROR: %v", r.Action, r.Err)
				}
				uApply.Step(ui.StatusFail, label, detail)
				continue
			}
			uApply.Step(ui.StatusOK, label, r.Action)
		}
	} else {
		for _, r := range results {
			if r.Err != nil {
				failed++
			}
		}
	}

	migrateEnv(w, errW, plans, m, src, workspace, owned)

	if err := owned.Save(manifestPath); err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	if err := writeWorkspacePointer(os.Getenv, os.UserHomeDir, workspace); err != nil {
		_, _ = fmt.Fprintln(errW, "warning: could not record workspace pointer:", err)
	}

	// Provision playwright-cli (frontend workspaces only), retire the kit's
	// Playwright MCP and its allow rules (every workspace). NON-FATAL: a failure
	// warns but does not fail onboarding or the failed-count return.
	if home, herr := os.UserHomeDir(); herr != nil || home == "" {
		_, _ = fmt.Fprintf(errW, "warning: playwright bootstrap: cannot resolve home: %v\n", herr)
	} else {
		playwrightProvision(w, errW, hasFrontendRepo(m), codexOnPath(), playwright.Options{
			Runner: func(name string, args []string) error { // stream: the chromium download is otherwise a silent multi-minute wait
				c := exec.Command(name, args...) //nolint:gosec // G204 -- fixed trusted binary, args are internally-computed subcommands, not attacker-controlled shell input
				c.Stdout, c.Stderr = w, errW
				return c.Run()
			},
			Output: func(name string, args []string) ([]byte, error) { // bounded like runClaudeBounded; stdout only, the parser reads it
				ctx, cancel := context.WithTimeout(context.Background(), claudeMCPTimeout)
				defer cancel()
				c := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204 -- fixed trusted binary, internally-computed args
				c.WaitDelay = 2 * time.Second
				return c.Output()
			},
			Home:   home,
			Getenv: os.Getenv,
			GOOS:   runtime.GOOS,
			Stdout: w,
		})
	}

	// Broker-backed Grafana/PostHog MCP servers for Claude (FR-9). NON-FATAL.
	// The broker comes from the trusted (embedded) manifest, never --manifest or
	// cwd: the registered headersHelper `zenify broker headers` resolves it the
	// same way, so the MCP URL and its token always name one broker.
	bm, _ := brokerManifest("", "")
	registerBrokerMCP(w, errW, bm, runClaudeBounded)

	// gh-stack extension for stacked PRs (FR-6.1). NON-FATAL.
	ensureGHStack(gh, w, errW)

	// Onboarding convenience (Task 5): clone the docs knowledge store on a
	// brand-new machine when it's absent, then reconcile the workspace view.
	// FAIL-OPEN — never affects `failed` or the return below.
	ensureDocsStore(w, errW, git, workspace)

	// Hooks + model pin + store config distribution, all fail-open (W0 FR-01.4);
	// never affects `failed`.
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		rep := ensureWorkspace(workspace, home, w, errW)
		renderCodexSummary(rep, w)
	}

	// tmux is required by `zenify hub`; hint only, never install from up.
	printTmuxHint(w, defaultTmuxEnv)

	// NPM_TOKEN for repos installing from GitHub Packages (FR-6). FAIL-OPEN.
	if dirs, err := repoDirsFor(workspace); err != nil {
		fmt.Fprintf(errW, "warning: npm token: %v\n", err)
	} else {
		interactive := npmTokenInteractive(nonInteractive, term.IsTerminal(int(os.Stdin.Fd())), term.IsTerminal(int(os.Stdout.Fd())))
		refresh := npmRefresh(os.Stdout)
		if err := ensureNPMToken(w, dirs, gh, interactive, refresh); err != nil {
			fmt.Fprintf(errW, "warning: %v\n", err)
		}
	}

	// wt / pm shims so skills and headless workers can call them. FAIL-OPEN.
	if home, err := os.UserHomeDir(); err != nil || home == "" {
		fmt.Fprintf(errW, "warning: shims: cannot resolve home: %v\n", err)
	} else {
		written, skipped, serr := apply.InstallShims(home, false)
		if serr != nil {
			fmt.Fprintf(errW, "warning: shims: %v\n", serr)
		}
		for _, p := range written {
			fmt.Fprintf(w, "shim: wrote %s\n", p)
		}
		for _, p := range skipped {
			fmt.Fprintf(errW, "warning: %s exists and is not managed by zenify; left unchanged\n", p)
		}
	}

	if failed > 0 {
		return exitcode.New(exitcode.Fail, fmt.Errorf("apply: %d repo(s) failed", failed))
	}
	return nil
}

// settingsEnvPaths lists the settings.local.json files whose "env" block may
// hold credentials: the workspace's, each repo's, and each repo worktree's copy
// (<repo>/.worktrees/*/.claude/settings.local.json). Worktree copies are
// best-effort: one that does not load is left out and returned in skipped, so
// it never blocks the rest; a broken workspace or repo file still aborts.
func settingsEnvPaths(workspace string, repoPaths []string) (paths, skipped []string) {
	paths = []string{filepath.Join(workspace, ".claude", "settings.local.json")}
	for _, rp := range repoPaths {
		paths = append(paths, filepath.Join(workspace, rp, ".claude", "settings.local.json"))
		wts, _ := filepath.Glob(filepath.Join(workspace, rp, ".worktrees", "*", ".claude", "settings.local.json"))
		for _, wt := range wts {
			if apply.CheckSettingsFile(wt) != nil {
				skipped = append(skipped, wt)
				continue
			}
			paths = append(paths, wt)
		}
	}
	return paths, skipped
}

// migrateEnv moves manifest keys out of the workspace, repo and repo worktree
// settings.local.json env blocks into <workspace>/.zenify/.env (FR-9), then
// denies agent reads of that file in ~/.claude/settings.json (FR-10). It runs under the apply lock and
// before the ownership manifest is saved, so re-recorded fingerprints persist.
// Fail-open; prints key names only, never a value.
func migrateEnv(w, errW io.Writer, plans []reconcile.RepoPlan, m *manifest.Manifest, src, workspace string, owned *managed.Manifest) {
	var repoPaths []string
	for _, p := range plans {
		repoPaths = append(repoPaths, p.Path)
	}
	paths, skipped := settingsEnvPaths(workspace, repoPaths)
	for _, p := range skipped {
		_, _ = fmt.Fprintf(w, "env: bỏ qua %s — file settings của worktree không đọc được, xử lý tay\n", p) //znf:allow-lang
	}
	if !migrateSettingsAndMerge(w, errW, paths, manifest.KeyNames(m.AllKeys()), devenv.Path(workspace), owned) {
		_, _ = fmt.Fprintln(errW, "env: bỏ qua dựng lại .zenify/.env do bước chuyển key lỗi") //znf:allow-lang
	} else if err := renderEnvStep(w, m, src, devenv.Path(workspace), owned, false); err != nil {
		_, _ = fmt.Fprintf(errW, "env: không dựng được .zenify/.env (%v)\n", err) //znf:allow-lang
	}
	home, herr := os.UserHomeDir()
	if herr != nil || home == "" {
		_, _ = fmt.Fprintf(errW, "permissions: không thêm được luật deny .zenify/.env (home: %v)\n", herr) //znf:allow-lang
		return
	}
	if n, err := apply.EnsureKitDeny(home, workspace, false); err != nil {
		_, _ = fmt.Fprintf(errW, "permissions: không thêm được luật deny .zenify/.env (%v)\n", err) //znf:allow-lang
	} else if n > 0 {
		rule, _ := apply.KitDenyRule(workspace)
		_, _ = fmt.Fprintf(w, "permissions: chặn agent đọc .zenify/.env (%s trong ~/.claude/settings.json)\n", rule) //znf:allow-lang
	}
}

// migrateSettingsAndMerge moves manifest and legacy MYSQL_* keys out of the
// settings files into envPath (FR-9), then joins the legacy keys there into
// MYSQL_URL (FR-12.3). Prints key names only, never a value.
// It reports false when the settings migration failed.
func migrateSettingsAndMerge(w, errW io.Writer, paths, manifestKeys []string, envPath string, owned *managed.Manifest) bool {
	res, err := apply.MigrateSettingsEnv(paths, apply.MigrationKeys(manifestKeys), envPath, owned)
	if err != nil {
		_, _ = fmt.Fprintf(errW, "env: không chuyển được key sang .zenify/.env (%v)\n", err) //znf:allow-lang
	}
	if len(res.Moved) > 0 {
		_, _ = fmt.Fprintf(w, "env: chuyển %d key (%s)\n", len(res.Moved), strings.Join(res.Moved, ", ")) //znf:allow-lang
	}
	if len(res.Removed) > 0 {
		_, _ = fmt.Fprintf(w, "env: xoá %d placeholder rỗng (%s)\n", len(res.Removed), strings.Join(res.Removed, ", ")) //znf:allow-lang
	}
	for _, k := range res.Conflicts {
		_, _ = fmt.Fprintf(w, "env: bỏ qua %s — hai giá trị khác nhau, xử lý tay\n", k) //znf:allow-lang
	}
	for _, k := range res.Skipped {
		_, _ = fmt.Fprintf(w, "env: bỏ qua %s — giá trị không phải chuỗi một dòng, xử lý tay\n", k) //znf:allow-lang
	}
	if err != nil {
		return false // which keys conflict is unknown; merging could drop a value settings still holds
	}
	mergeMySQLURLStep(w, errW, envPath, append(append([]string{}, res.Conflicts...), res.Skipped...))
	return true
}

// renderEnvStep rebuilds envPath as the grouped scaffold of the manifest keys.
// src is where m came from (loadKitManifest). A recorded key the manifest lacks
// is deleted only when envDeletionGate allows it; otherwise it stays in the
// file and in owned.EnvKeys. dryRun only prints what would change; otherwise
// it saves the file and records the kit-managed key names in owned.EnvKeys —
// new names and the EnvKeysVersion stamp only from the embedded manifest.
// Prints key names only, never a value.
func renderEnvStep(w io.Writer, m *manifest.Manifest, src, envPath string, owned *managed.Manifest, dryRun bool) error {
	old, err := devenv.Load(envPath)
	if err != nil {
		return err
	}
	current := manifest.KeyNames(m.AllKeys())
	prev := owned.EnvKeys
	allowed, reason := envDeletionGate(src, version.Current(), owned.EnvKeysVersion)
	var held []string
	if !allowed {
		skip := map[string]bool{}
		for _, k := range current {
			skip[k] = true
		}
		for _, k := range owned.EnvKeys {
			if _, ok := old.Get(k); ok && !skip[k] {
				skip[k] = true
				held = append(held, k)
			}
		}
		prev = nil
	}
	res := devenv.Render(old, m.PlacedKeys(), prev)
	if !dryRun {
		if err := res.File.Save(envPath); err != nil {
			return err
		}
		if src == "embedded" {
			owned.EnvKeys = append(current, held...)
			if bumpEnvKeysVersion(version.Current(), owned.EnvKeysVersion) {
				owned.EnvKeysVersion = version.Current()
			}
		} else {
			// only the embedded manifest may add a key: one recorded from
			// a checkout would be deleted by the next embedded run
			was := map[string]bool{}
			for _, k := range owned.EnvKeys {
				was[k] = true
			}
			var kept []string
			for _, k := range current {
				if was[k] {
					kept = append(kept, k)
				}
			}
			owned.EnvKeys = append(kept, held...)
		}
	}
	for _, l := range []struct {
		label string
		names []string
	}{{"thêm", res.Added}, {"xoá", res.Removed}} { //znf:allow-lang
		if len(l.names) > 0 {
			_, _ = fmt.Fprintf(w, "env: %s %s\n", l.label, strings.Join(l.names, ", "))
		}
	}
	if len(held) > 0 {
		_, _ = fmt.Fprintf(w, "env: giữ %s — %s\n", strings.Join(held, ", "), reason) //znf:allow-lang
	}
	if len(res.Undescribed) > 0 {
		_, _ = fmt.Fprintf(w, "env: custom thiếu mô tả %s\n", strings.Join(res.Undescribed, ", ")) //znf:allow-lang
	}
	if len(res.Shadowed) > 0 {
		_, _ = fmt.Fprintf(w, "env: giữ giá trị trùng của %s dưới dạng comment\n", strings.Join(res.Shadowed, ", ")) //znf:allow-lang
	}
	return nil
}

// envDeletionGate reports whether renderEnvStep may delete a recorded key the
// manifest lacks: only with the manifest embedded in this binary, and only
// when this binary is not older than the one that last recorded EnvKeys
// (recorded). When it may not, reason says why, for the keep line.
func envDeletionGate(src, current, recorded string) (bool, string) {
	if src != "embedded" {
		return false, fmt.Sprintf("manifest không phải bản nhúng (%s)", src) //znf:allow-lang
	}
	if !envVersionNotOlder(current, recorded) {
		return false, "binary cũ hơn bản đã ghi" //znf:allow-lang
	}
	return true, ""
}

// envVersionNotOlder reports current >= recorded. A dev build is comparable
// only to another dev build; an empty or unparsable version is unsure, and
// unsure is "older".
func envVersionNotOlder(current, recorded string) bool {
	if current == "dev" || recorded == "dev" {
		return current == recorded
	}
	ok, err := version.MeetsMin(current, recorded)
	return err == nil && ok
}

// bumpEnvKeysVersion reports whether a write by current re-stamps
// EnvKeysVersion: when nothing is recorded, when current is not older, or
// when the stamp is a dev or unparsable one and current is a release. A
// release stamp is never replaced by an older or a dev one.
func bumpEnvKeysVersion(current, recorded string) bool {
	if recorded == "" || envVersionNotOlder(current, recorded) {
		return true
	}
	if _, err := version.MeetsMin(current, "v0.0.0"); err != nil || current == "dev" {
		return false
	}
	_, err := version.MeetsMin("v0.0.0", recorded)
	return err != nil || recorded == "dev"
}

// mergeMySQLURLStep joins the legacy MYSQL_* keys of .zenify/.env into
// MYSQL_URL (FR-12.3). held names keys the settings migration left in a
// settings file; a legacy key among them blocks the merge, since deleting it
// from .zenify/.env would leave only the conflicting settings value. Prints
// key names only, never a value.
func mergeMySQLURLStep(w, errW io.Writer, envPath string, held []string) {
	for _, k := range held {
		for _, l := range apply.LegacyMySQLKeys {
			if k == l {
				_, _ = fmt.Fprintln(w, "env: bỏ qua MYSQL_URL — MYSQL_* còn xung đột trong settings, xử lý tay") //znf:allow-lang
				return
			}
		}
	}
	res, err := apply.MergeMySQLURL(envPath)
	switch {
	case err != nil:
		_, _ = fmt.Fprintf(errW, "env: không ghép được MYSQL_* thành MYSQL_URL (%v)\n", err) //znf:allow-lang
	case res.Merged:
		_, _ = fmt.Fprintln(w, "env: ghép MYSQL_HOST, MYSQL_PORT, MYSQL_USER, MYSQL_PASSWORD, MYSQL_DATABASE thành MYSQL_URL") //znf:allow-lang
	case res.Dropped:
		_, _ = fmt.Fprintln(w, "env: xoá MYSQL_* cũ, MYSQL_URL đã có cùng giá trị") //znf:allow-lang
	case res.Conflict:
		_, _ = fmt.Fprintln(w, "env: bỏ qua MYSQL_URL — khác giá trị ghép từ MYSQL_*, xử lý tay") //znf:allow-lang
	case res.Invalid:
		_, _ = fmt.Fprintln(w, "env: bỏ qua MYSQL_URL — MYSQL_* không ghép được thành URL hợp lệ, xử lý tay") //znf:allow-lang
	case len(res.Missing) > 0:
		_, _ = fmt.Fprintf(w, "env: chưa ghép MYSQL_URL — thiếu %s, xử lý tay\n", strings.Join(res.Missing, ", ")) //znf:allow-lang
	}
}

// renderCodexSummary prints the Codex block of `zenify up` (FR-7.2).
func renderCodexSummary(rep ensureReport, w io.Writer) {
	if !rep.CodexInstalled {
		fmt.Fprintln(w, "Codex: không cài, bỏ qua") //znf:allow-lang
		return
	}
	fmt.Fprintln(w, "Codex:")
	fmt.Fprintf(w, "  skills: symlink %s\n", rep.CodexLinkPath)
	fmt.Fprintf(w, "  hooks: %d entry\n", rep.CodexHooks.Total())
	if rep.CodexHooks.Added+rep.CodexHooks.Updated > 0 {
		fmt.Fprintln(w, "    hook mới/đổi chưa chạy cho tới khi trust: mở codex, gõ /hooks, chọn trust") //znf:allow-lang
	}
	fmt.Fprintf(w, "  agents: %d\n", rep.CodexAgents)
}

// playwrightProvision is the seam runApplyWith reaches provisionPlaywright
// through; TestMain swaps it so no test shells out to the real claude/codex.
var playwrightProvision = provisionPlaywright

// provisionPlaywright installs the pinned playwright-cli when the workspace has
// a frontend repo, then (for every workspace) removes the kit-owned Playwright
// MCP registration for Claude and, when codex is on PATH, Codex, and drops the
// retired kit allow rules. NON-FATAL: every failure only warns.
func provisionPlaywright(w, errW io.Writer, frontend, codex bool, po playwright.Options) {
	if frontend {
		if err := playwright.Install(po); err != nil {
			_, _ = fmt.Fprintf(errW, "warning: playwright bootstrap: %v (onboarding otherwise succeeded)\n", err)
		}
	}
	targets := []playwright.Target{playwright.TargetClaude}
	if codex {
		targets = append(targets, playwright.TargetCodex)
	}
	for _, t := range targets {
		o := po
		o.Target = t
		state, err := playwright.Deregister(o)
		switch {
		case err != nil:
			_, _ = fmt.Fprintf(errW, "warning: playwright MCP removal (%s): %v\n", t, err)
		case state == playwright.RegCustom:
			_, _ = fmt.Fprintf(w, "playwright: giữ đăng ký MCP \"playwright\" bạn tự cấu hình (%s); znf:ui-verifier giờ dùng zenify browser\n", t) //znf:allow-lang
		case state == playwright.RegDesired || state == playwright.RegLegacyDefault:
			_, _ = fmt.Fprintf(w, "playwright: đã gỡ MCP \"playwright\" của kit (%s)\n", t) //znf:allow-lang
		}
	}
	if _, err := apply.RemoveRetiredKitPermissions(po.Home, false); err != nil {
		_, _ = fmt.Fprintf(errW, "warning: playwright permissions: %v\n", err)
	}
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
// a hand rollback knows what was moved where (FR-4.4). To is the workspace-
// absolute destination (workspace joined with the manifest-relative Path),
// not the manifest-relative path alone, so the log is directly usable for a
// rollback without re-resolving it against the workspace root. Fail-open.
func writeRelocateLog(snapDir, workspace string, plans []reconcile.RepoPlan, errW io.Writer) {
	type mv struct{ From, To string }
	var moves []mv
	for _, p := range plans {
		if p.State == reconcile.Relocate {
			moves = append(moves, mv{From: p.From, To: filepath.Join(workspace, p.Path)})
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
		reviewer       string
	)
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Onboard workspace: wizard tương tác trong terminal, còn không thì in kế hoạch dry-run (--apply để chạy headless)", //znf:allow-lang
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := dryRunApplyConflict(applyFlag, cmd.Flags().Changed("dry-run"), dryRun); err != nil {
				return exitcode.New(exitcode.BadArgs, err)
			}
			if reviewer != "" {
				if reviewer != "agent" && reviewer != "codex" {
					return exitcode.New(exitcode.BadArgs, fmt.Errorf("--reviewer %q: want agent or codex", reviewer))
				}
				if home, herr := os.UserHomeDir(); herr == nil && home != "" {
					if err := apply.SetReviewerEnv(home, reviewer, !applyFlag); err != nil {
						fmt.Fprintln(cmd.ErrOrStderr(), "znf up: reviewer:", err)
					}
				}
			}
			cwd, _ := os.Getwd()
			isTTY := term.IsTerminal(int(os.Stdout.Fd()))
			wantWizard := decideMode(isTTY, applyFlag, cmd.Flags().Changed("dry-run"), dryRun, jsonOut, nonInteractive) == modeWizard
			var sources map[string]reconcile.Source
			ws, src, ok := resolveWorkspace(cwd, workspace, os.Getenv, os.UserHomeDir)
			var sourcesDir string
			if !ok && !wantWizard {
				return exitcode.New(exitcode.BadArgs, errors.New("chưa có workspace: dùng --workspace <dir> hoặc chạy zenify up có terminal")) //znf:allow-lang
			}
			if whereNeeded(wantWizard, src, ok) {
				home, _ := os.UserHomeDir()
				existing := ""
				if ok {
					existing = ws // pointer target (FR-3.2/SC-2)
				}
				res, werr := tui.RunWhere(tui.WhereConfig{
					Cwd:       cwd,
					Existing:  existing,
					OSDefault: osDefaultWorkspace(runtime.GOOS, home),
					Validate:  func(d string) error { return validateWorkspaceDir(expandHome(d, home), home, gitToplevel) },
				})
				if werr != nil {
					if tui.IsAborted(werr) {
						return exitcode.New(exitcode.Cancelled, werr)
					}
					return exitcode.New(exitcode.BadArgs, werr)
				}
				res.Workspace = expandHome(res.Workspace, home)
				res.SourcesDir = expandHome(res.SourcesDir, home)
				if err := os.MkdirAll(res.Workspace, 0o750); err != nil {
					return exitcode.New(exitcode.Fail, err)
				}
				ws = res.Workspace
				sourcesDir = res.SourcesDir
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
			m, mSrc, err := loadKitManifest(manifestPath, overlayPath)
			if err != nil {
				if isPreview {
					printPlanFooterRows(w, workspace)
				}
				return exitcode.New(exitcode.Fail, err)
			}
			if sourcesDir != "" {
				sources = scanSources(m, gitx.ExecRunner(), sourcesDir)
			}
			sp := ui.NewSpinner(cmd.ErrOrStderr(), "Đang dựng kế hoạch onboarding") //znf:allow-lang
			sp.Start()
			plans, auth, err := buildPlan(m, ghRunner, gitx.ExecRunner(), workspace, sources)
			if err != nil {
				sp.Fail("Dựng kế hoạch thất bại") //znf:allow-lang
				if isPreview {
					printPlanFooterRows(w, workspace)
				}
				return exitcode.New(exitcode.Fail, err)
			}
			sp.Success("Đã dựng kế hoạch onboarding") //znf:allow-lang
			// Logged out: the interactive wizard runs `gh auth login` itself
			// (tui.loginStep, before it rebuilds the plan), so only the headless
			// paths (dry-run / --apply / --json / --non-interactive) hard-fail here.
			if !auth.LoggedIn && !wantWizard {
				if isPreview {
					printPlanFooterRows(w, workspace)
				}
				return exitcode.New(exitcode.Fail,
					fmt.Errorf("not logged in to GitHub — run `gh auth login` (need scopes read:org, repo)"))
			}
			if auth.LoggedIn && !auth.HasScopes("read:org", "repo") {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(),
					"warning: gh token missing read:org or repo scope; discovery may be incomplete")
			}
			switch decideMode(isTTY, applyFlag, cmd.Flags().Changed("dry-run"), dryRun, jsonOut, nonInteractive) {
			case modeWizard:
				return runWizard(w, m, mSrc, workspace, sources, plans, auth.LoggedIn)
			case modeApply:
				uiOut(cmd).Header("zenify up --apply")
				return runApplyWith(w, cmd.ErrOrStderr(), plans, m, mSrc, workspace, ghRunner, gitx.ExecRunner(), nonInteractive)
			default: // modeDryRun
				if jsonOut {
					return renderPlanJSON(w, plans, auth)
				}
				uiOut(cmd).Header("zenify up")
				renderPlanTable(w, plans, auth)
				if owned, lerr := managed.Load(filepath.Join(workspace, ".zenify", "manifest.json")); lerr == nil {
					if rerr := renderEnvStep(w, m, mSrc, devenv.Path(workspace), owned, true); rerr != nil {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "env: không xem trước được .zenify/.env (%v)\n", rerr) //znf:allow-lang
					}
				}
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
	cmd.Flags().StringVar(&reviewer, "reviewer", "", "review engine written to ZNF_REVIEWER in ~/.claude/settings.json: agent or codex (written only with --apply)")
	cmd.Flags().BoolVar(&applyFlag, "apply", false, "apply changes without the interactive wizard (required for non-interactive/CI runs)")
	return cmd
}

// registerBrokerMCP registers the kit's Grafana/PostHog HTTP MCP servers with
// Claude when the manifest declares a broker and claude is on PATH; otherwise
// it does nothing, so `zenify up` keeps its pre-broker behaviour. Failures only warn.
func registerBrokerMCP(w, errW io.Writer, m *manifest.Manifest, run func(string, ...string) ([]byte, error)) {
	if m == nil || m.Broker == nil {
		return
	}
	if _, err := lookPath("claude"); err != nil {
		return
	}
	for _, name := range []string{"grafana", "posthog"} {
		st, err := mcpreg.Bootstrap(mcpreg.Options{
			Spec: mcpreg.Spec{Name: "zenify-" + name, URL: strings.TrimRight(m.Broker.URL, "/") + "/mcp/" + name},
			Run:  run,
		})
		switch {
		case err != nil:
			_, _ = fmt.Fprintf(errW, "warning: zenify-%s MCP: %v (onboarding otherwise succeeded)\n", name, err)
		case st == playwright.RegAbsent:
			_, _ = fmt.Fprintf(w, "zenify-%s: registered MCP server (user scope)\n", name)
		case st == playwright.RegLegacyDefault:
			_, _ = fmt.Fprintf(w, "zenify-%s: updated MCP server URL (user scope)\n", name)
		case st == playwright.RegCustom:
			_, _ = fmt.Fprintf(w, "zenify-%s: custom MCP registration kept\n", name)
		}
	}
}

// claudeMCPTimeout bounds each `claude mcp` call registerBrokerMCP makes.
var claudeMCPTimeout = 20 * time.Second

// runClaudeBounded runs one command with claudeMCPTimeout, returning its
// combined output; WaitDelay stops a child that keeps the pipes open.
func runClaudeBounded(n string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), claudeMCPTimeout)
	defer cancel()
	c := exec.CommandContext(ctx, n, args...) //nolint:gosec // G204 -- fixed trusted binary, internally-computed args
	c.WaitDelay = 2 * time.Second
	return c.CombinedOutput()
}
