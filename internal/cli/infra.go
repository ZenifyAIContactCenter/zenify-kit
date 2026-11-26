package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/infra"
)

var (
	platformTimeout = 15 * time.Minute
	envTimeout      = 20 * time.Minute
)

// infraDeps are the host boundaries `zenify infra` touches; tests replace them.
type infraDeps struct {
	home      func() (string, error)
	workspace func() (string, bool, error)
	runner    infra.Runner
	builder   infra.BuildRunner
	preflight func(ws, profile string) []infra.Problem
	// spec loads <knowledge store>/.config/infra.json for the workspace ws.
	spec func(ws string) (infra.Workspace, error)
	lock func(home string) (func() error, error)

	installK3s     func(ctx context.Context, r infra.Runner) (bool, error)
	installArgo    func(ctx context.Context, r infra.Runner) (bool, error)
	setRepoCred    func(ctx context.Context, r infra.Runner, repoURL, token string) error
	applyPlatform  func(ctx context.Context, r infra.Runner, repoURL, domain, chartRef string) error
	waitApp        func(ctx context.Context, r infra.Runner, name string, timeout time.Duration) error
	fetchCA        func(ctx context.Context, r infra.Runner) ([]byte, error)
	installCA      func(ctx context.Context, r infra.Runner, home string, pem []byte) (bool, error)
	splitDNS       func(ctx context.Context, r infra.Runner, domain string) (bool, error)
	restartDNS     func(ctx context.Context, r infra.Runner) error
	ensureBuildctl func(ctx context.Context, r infra.Runner, home string) (string, error)
	scanTargets    func(ctx context.Context, r infra.Runner, w infra.Workspace, ws, domain, slug string, refs map[string]string) ([]infra.BuildTarget, error)
	build          func(ctx context.Context, br infra.BuildRunner, ws string, t infra.BuildTarget, refs map[string]string) (infra.BuildResult, error)
	apply          func(ctx context.Context, r infra.Runner, manifest []byte) (bool, error)
	appExists      func(ctx context.Context, r infra.Runner, name string) (bool, error)
	refreshApps    func(ctx context.Context, r infra.Runner, names []string) error
	syncedRevision func(ctx context.Context, r infra.Runner, name string) (string, bool, error)
	syncApp        func(ctx context.Context, r infra.Runner, name string, timeout time.Duration) (string, error)
	deployedTags   func(ctx context.Context, r infra.Runner, w infra.Workspace, slug string) (map[string]string, error)
	applyEnvCA     func(ctx context.Context, r infra.Runner, slug string, pem []byte) error
	saveState      func(home string, s *infra.State) error
	livePlatform   func(ctx context.Context, r infra.Runner) (infra.PlatformTarget, bool, error)

	status          func(ctx context.Context, r infra.Runner, w infra.Workspace, home, domain string) (infra.Report, error)
	show            func(ctx context.Context, r infra.Runner, w infra.Workspace, home, domain, slug string) (infra.ShowReport, error)
	removeTrust     func(ctx context.Context, r infra.Runner, home string) error
	uninstallK3s    func(ctx context.Context, r infra.Runner) error
	removeState     func(home string) error
	deleteApp       func(ctx context.Context, r infra.Runner, name string, cascade bool) error
	deleteNamespace func(ctx context.Context, r infra.Runner, ns string) error
	k3sVersion      func(ctx context.Context, r infra.Runner) (string, error)
	argoVersion     func(ctx context.Context, r infra.Runner) (string, error)
	envApps         func(ctx context.Context, r infra.Runner) ([]string, error)
}

func defaultInfraDeps() infraDeps {
	return infraDeps{
		home: os.UserHomeDir,
		workspace: func() (string, bool, error) {
			cwd, err := os.Getwd()
			if err != nil {
				return "", false, fmt.Errorf("current directory: %w", err)
			}
			ws, _, ok := resolveWorkspace(cwd, "", os.Getenv, os.UserHomeDir)
			return ws, ok, nil
		},
		runner:  infra.ExecRunner(),
		builder: infra.ExecBuildRunner(),
		preflight: func(_, profile string) []infra.Problem {
			return infra.Preflight(infra.RealHostProbe(), infra.DefaultRequirements, infra.K3sVersion, profile)
		},
		spec: func(ws string) (infra.Workspace, error) {
			store := resolveDocsStore(ws, os.Getenv, os.UserHomeDir, os.Stat, os.ReadDir)
			return infra.LoadWorkspace(filepath.Join(store, defaultConfigSub, infra.WorkspaceFile))
		},
		lock:            infra.LockHost,
		applyEnvCA:      infra.ApplyEnvCA,
		deployedTags:    infra.DeployedTags,
		installK3s:      infra.InstallK3s,
		installArgo:     infra.InstallArgo,
		setRepoCred:     infra.SetRepoCredential,
		applyPlatform:   infra.ApplyPlatform,
		waitApp:         infra.WaitApp,
		fetchCA:         infra.FetchCA,
		installCA:       infra.InstallCA,
		splitDNS:        infra.ConfigureSplitDNS,
		restartDNS:      infra.RestartSplitDNS,
		ensureBuildctl:  infra.EnsureBuildctl,
		scanTargets:     infra.ScanTargets,
		build:           infra.BuildFromBase,
		apply:           infra.Apply,
		appExists:       infra.AppExists,
		refreshApps:     infra.RefreshApps,
		syncedRevision:  infra.SyncedRevision,
		syncApp:         infra.SyncApp,
		saveState:       infra.SaveState,
		status:          infra.Status,
		show:            infra.Show,
		removeTrust:     infra.RemoveTrustAndDNS,
		uninstallK3s:    infra.UninstallK3s,
		removeState:     infra.RemoveState,
		deleteApp:       infra.DeleteApp,
		deleteNamespace: infra.DeleteNamespace,
		livePlatform:    infra.LivePlatform,
		k3sVersion:      infra.K3sInstalledVersion,
		argoVersion:     infra.ArgoInstalledVersion,
		envApps:         infra.EnvApps,
	}
}

var errOnlyLocal = errors.New("only --local is supported")

func newInfraCmd() *cobra.Command { return newInfraCmdWith(defaultInfraDeps()) }

func newInfraCmdWith(d infraDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "infra",
		Short: "Devbox k3s cục bộ: cài k3s, ArgoCD, platform và env base", //znf:allow-lang
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(d.upCmd(), d.statusCmd(), d.showCmd(), d.downCmd(), d.configCmd())
	return cmd
}

func requireLocal(local bool) error {
	if !local {
		return exitcode.New(exitcode.BadArgs, errOnlyLocal)
	}
	return nil
}

// failStep names the step that failed and exits 1.
func failStep(step string, err error) error {
	return exitcode.New(exitcode.Fail, fmt.Errorf("%s: %w", step, err))
}

func changedWord(changed bool) string {
	if changed {
		return "changed"
	}
	return "unchanged"
}

// loadConfig reads config.json and rejects values that would break up.
func (d infraDeps) loadConfig() (string, infra.Config, error) {
	home, err := d.home()
	if err != nil {
		return "", infra.Config{}, fmt.Errorf("home dir: %w", err)
	}
	cfg, err := infra.LoadConfig(home)
	if err != nil {
		return "", infra.Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return "", infra.Config{}, fmt.Errorf("%s: %w", infra.ConfigPath(home), err)
	}
	return home, cfg, nil
}

// workspaceSpec loads infra.json for the resolved workspace (the store is
// found from it, or from the default store when there is no workspace).
func (d infraDeps) workspaceSpec() (infra.Workspace, error) {
	ws, ok, err := d.workspace()
	if err != nil {
		return infra.Workspace{}, err
	}
	if !ok {
		ws = ""
	}
	return d.spec(ws)
}

// infraTarget is the domain and helm chart ref a command works on, with where
// each came from; an explicit source (flag or config) is a change on purpose.
type infraTarget struct {
	domain, domainFrom string
	chartRef, refFrom  string
	live               infra.PlatformTarget
	hasLive            bool
	state              *infra.State
}

func explicitSource(from string) bool { return from == "flag" || from == "config" }

// resolveTarget picks the domain (config, state, deployed platform,
// zenify.test) and chart ref (flag, config, deployed platform, main).
func (d infraDeps) resolveTarget(ctx context.Context, home string, cfg infra.Config, flagRef string) (infraTarget, error) {
	var t infraTarget
	var err error
	if t.state, err = infra.LoadState(home); err != nil {
		return t, err
	}
	if t.live, t.hasLive, err = d.livePlatform(ctx, d.runner); err != nil {
		return t, fmt.Errorf("read deployed platform: %w", err)
	}
	switch {
	case cfg.Domain != "":
		t.domain, t.domainFrom = cfg.Domain, "config"
	case t.state != nil && t.state.Domain != "":
		t.domain, t.domainFrom = t.state.Domain, "state"
	case t.hasLive && t.live.Domain != "":
		t.domain, t.domainFrom = t.live.Domain, "deployed platform"
	default:
		t.domain, t.domainFrom = infra.DefaultDomain, "default"
	}
	switch {
	case flagRef != "":
		t.chartRef, t.refFrom = flagRef, "flag"
	case cfg.ChartRef != "":
		t.chartRef, t.refFrom = cfg.ChartRef, "config"
	case t.hasLive && t.live.ChartRef != "":
		t.chartRef, t.refFrom = t.live.ChartRef, "deployed platform"
	default:
		t.chartRef, t.refFrom = infra.DefaultChartRef, "default"
	}
	return t, nil
}

// drift refuses to retarget a deployed platform unless the new value was asked for.
func (t infraTarget) drift() error {
	if !t.hasLive {
		return nil
	}
	var diffs []string
	if t.domain != t.live.Domain && !explicitSource(t.domainFrom) {
		diffs = append(diffs, fmt.Sprintf("domain %q (from %s) but the deployed platform runs %q", t.domain, t.domainFrom, t.live.Domain))
	}
	if t.chartRef != t.live.ChartRef && !explicitSource(t.refFrom) {
		diffs = append(diffs, fmt.Sprintf("chart ref %q (from %s) but the deployed platform runs %q", t.chartRef, t.refFrom, t.live.ChartRef))
	}
	if len(diffs) == 0 {
		return nil
	}
	return fmt.Errorf("%s; to keep the deployed platform run: zenify infra config --local --domain %s and zenify infra up --local --chart-ref %s; to change it on purpose run zenify infra config --local --domain <x>.test or zenify infra up --local --chart-ref <ref>",
		strings.Join(diffs, "; "), t.live.Domain, t.live.ChartRef)
}

func parseRefs(in []string) (map[string]string, error) {
	refs := map[string]string{}
	for _, s := range in {
		repo, ref, ok := strings.Cut(s, "=")
		if !ok || repo == "" || ref == "" {
			return nil, fmt.Errorf("--ref %q: want <repo>=<ref>", s)
		}
		refs[repo] = ref
	}
	return refs, nil
}

func (d infraDeps) upCmd() *cobra.Command {
	var (
		local, verbose bool
		chartRef       string
		profile        string
		refFlags       []string
	)
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Cài hoặc tiếp tục cài devbox cục bộ tới env base", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireLocal(local); err != nil {
				return err
			}
			refs, err := parseRefs(refFlags)
			if err != nil {
				return exitcode.New(exitcode.BadArgs, err)
			}
			if cmd.Flags().Changed("profile") && !infra.ValidProfile(profile) {
				return exitcode.New(exitcode.BadArgs, fmt.Errorf("--profile %q: want %s or %s", profile, infra.ProfileCore, infra.ProfileFull))
			}
			return d.runUp(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), chartRef, profile, refs, verbose)
		},
	}
	cmd.Flags().BoolVar(&local, "local", false, "target the local k3s devbox (required)")
	cmd.Flags().StringVar(&chartRef, "chart-ref", "", "helm repo ref for the platform and env charts (default: config, else the deployed platform's, else main)")
	cmd.Flags().StringVar(&profile, "profile", "", "app set to deploy: core or full (default: the profile saved by the last up, else core)")
	cmd.Flags().StringArrayVar(&refFlags, "ref", nil, "build <repo>=<ref> instead of the repo's baseRef (repeatable)")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "print every build log line (always saved to the run log)")
	return cmd
}

func (d infraDeps) runUp(ctx context.Context, out, errW io.Writer, chartRef, profile string, refs map[string]string, verbose bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// Ctrl-C cancels the running step, so the view can stop cleanly and say
	// where the run stopped
	ctx, stopSignals := signal.NotifyContext(ctx, os.Interrupt)
	defer stopSignals()
	home, cfg, err := d.loadConfig()
	if err != nil {
		return failStep("config", err)
	}
	ws, ok, err := d.workspace()
	if err != nil {
		return failStep("workspace", err)
	}
	if !ok {
		return failStep("workspace", errors.New("no zenify workspace found (run zenify up first)"))
	}
	w, err := d.spec(ws)
	if err != nil {
		return failStep("workspace", err)
	}
	// the preflight RAM floor depends on the profile: flag, else saved state, else core
	st, err := infra.LoadState(home)
	if err != nil {
		return failStep("state", err)
	}
	if profile == "" {
		if profile, err = savedProfile(st); err != nil {
			return err
		}
	}
	ctx = infra.WithGhTokenCache(ctx)
	logFile, logPath, err := openRunLog(home, "up")
	if err != nil {
		return failStep("log", err)
	}
	defer func() {
		if cerr := logFile.Close(); cerr != nil {
			fmt.Fprintf(errW, "warning: close run log: %v\n", cerr)
		}
	}()
	steps := []string{"preflight", "k3s", "target", "argocd", "repo-credential", "platform", "ca", "dns", "buildctl"}
	for _, b := range w.Builds {
		steps = append(steps, "build "+b.Image)
	}
	steps = append(steps, "env-ca", "env-phase-1", "env-phase-2", "state")
	v := newStepView(out, "zenify infra up", "zenify infra up --local", steps, verbose, logFile, logPath)

	var unlock func() error
	defer func() {
		if unlock == nil {
			return
		}
		if uerr := unlock(); uerr != nil {
			fmt.Fprintf(errW, "warning: release infra lock: %v\n", uerr)
		}
	}()
	if err := v.run(ctx, "preflight", func(context.Context) (stepOutcome, error) {
		if ps := d.preflight(ws, profile); len(ps) > 0 {
			for _, p := range ps {
				fmt.Fprintf(errW, "problem %s: %s\n  fix: %s\n", p.Check, p.Detail, p.Fix)
			}
			return stepOutcome{}, fmt.Errorf("%d problem(s), nothing was changed", len(ps))
		}
		var err error
		if unlock, err = d.lock(home); err != nil {
			return stepOutcome{}, fmt.Errorf("lock: %w", err)
		}
		return stepOutcome{detail: "ok"}, nil
	}); err != nil {
		return err
	}
	r := d.runner

	// k3s first: a restart that failed in an earlier run left the API down,
	// and only InstallK3s's restart-marker retry brings it back
	if err := v.run(ctx, "k3s", func(ctx context.Context) (stepOutcome, error) {
		changed, err := d.installK3s(ctx, r)
		return stepOutcome{detail: changedWord(changed)}, err
	}); err != nil {
		return err
	}
	// the target and the drift check come before anything is applied to Argo
	var tg infraTarget
	if err := v.run(ctx, "target", func(ctx context.Context) (stepOutcome, error) {
		var err error
		if tg, err = d.resolveTarget(ctx, home, cfg, chartRef); err != nil {
			return stepOutcome{}, err
		}
		if err := tg.drift(); err != nil {
			return stepOutcome{}, err
		}
		return stepOutcome{detail: fmt.Sprintf("domain %s (%s), chart ref %s (%s)", tg.domain, tg.domainFrom, tg.chartRef, tg.refFrom)}, nil
	}); err != nil {
		return err
	}
	domain, chartRef := tg.domain, tg.chartRef
	w = w.ForProfile(profile)
	if err := v.run(ctx, "argocd", func(ctx context.Context) (stepOutcome, error) {
		changed, err := d.installArgo(ctx, r)
		return stepOutcome{detail: changedWord(changed)}, err
	}); err != nil {
		return err
	}
	if err := v.run(ctx, "repo-credential", func(ctx context.Context) (stepOutcome, error) {
		tok, err := r.Run(ctx, nil, "gh", "auth", "token")
		if err != nil {
			return stepOutcome{}, fmt.Errorf("gh auth token: %w", err)
		}
		return stepOutcome{detail: "applied"}, d.setRepoCred(ctx, r, w.HelmRepoURL, strings.TrimSpace(string(tok)))
	}); err != nil {
		return err
	}
	if err := v.run(ctx, "platform", func(ctx context.Context) (stepOutcome, error) {
		if err := d.applyPlatform(ctx, r, w.HelmRepoURL, domain, chartRef); err != nil {
			return stepOutcome{}, err
		}
		if err := d.waitApp(ctx, r, infra.PlatformApp, platformTimeout); err != nil {
			return stepOutcome{}, err
		}
		return stepOutcome{detail: fmt.Sprintf("%s Synced/Healthy at %s", infra.PlatformApp, chartRef)}, nil
	}); err != nil {
		return err
	}
	var pem []byte
	if err := v.run(ctx, "ca", func(ctx context.Context) (stepOutcome, error) {
		var err error
		if pem, err = d.fetchCA(ctx, r); err != nil {
			return stepOutcome{}, err
		}
		changed, err := d.installCA(ctx, r, home, pem)
		return stepOutcome{detail: changedWord(changed)}, err
	}); err != nil {
		return err
	}
	if err := v.run(ctx, "dns", func(ctx context.Context) (stepOutcome, error) {
		changed, err := d.splitDNS(ctx, r, domain)
		if err != nil {
			return stepOutcome{}, err
		}
		// always restart: cni0 may have been recreated since the unit last ran
		return stepOutcome{detail: changedWord(changed) + " (restarted)"}, d.restartDNS(ctx, r)
	}); err != nil {
		return err
	}

	images, failed, failedRepos, err := d.buildImages(ctx, v, w, home, ws, domain, profile, refs)
	if err != nil {
		return err
	}
	slug := infra.BaseSlug
	// an app whose image failed: if it already runs, it keeps its deployed
	// tag (leaving it out would prune a healthy app); otherwise it is left out
	// of phase 2. Either way the run ends non-zero after the state is saved.
	broken := infra.SkippedApps(w, failedRepos)
	kept := map[string]string{}
	if len(broken) > 0 {
		deployed, err := d.deployedTags(ctx, d.runner, w, slug)
		if err != nil {
			return failStep("deployed tags", err)
		}
		for _, k := range broken {
			if t, ok := deployed[k]; ok {
				kept[k] = t
			}
		}
	}
	skip := map[string]bool{}
	var skipped, keptKeys []string
	for _, k := range broken {
		if _, ok := kept[k]; ok {
			keptKeys = append(keptKeys, k)
		} else {
			skip[k] = true
			skipped = append(skipped, k)
		}
	}
	v.printLines(skippedLines(w, skipped, keptKeys, kept))
	configRev, err := d.applyEnv(ctx, v, w, slug, domain, chartRef, images, skip, kept, pem, st.ConfigRev())
	if err != nil {
		return err
	}
	if err := v.run(ctx, "state", func(context.Context) (stepOutcome, error) {
		created := time.Now().UTC()
		if tg.state != nil && !tg.state.CreatedAt.IsZero() {
			created = tg.state.CreatedAt // a rerun keeps when the devbox was first set up
		}
		return stepOutcome{detail: "saved"}, d.saveState(home, &infra.State{Domain: domain, K3sVersion: infra.K3sVersion, ArgoCDVersion: infra.ArgoCDVersion, CreatedAt: created, Profile: profile, ConfigRevision: configRev})
	}); err != nil {
		return err
	}
	if len(failed) > 0 {
		v.close("finished with failures")
		return exitcode.New(exitcode.Fail, fmt.Errorf("%d image(s) failed to build (%s); %d app(s) skipped (%s); %d app(s) kept at their deployed image (%s); the rest is deployed, rerun `zenify infra up --local` to retry the failed images",
			len(failed), strings.Join(failed, ", "), len(skipped), strings.Join(skipped, ", "), len(keptKeys), strings.Join(keptKeys, ", ")))
	}
	v.close("ready")
	fmt.Fprintf(out, "\nready: %s\n", strings.Join(infra.EnvURLs(w, slug, domain), " "))
	pw := w.Seed.Password
	if pw == "" {
		pw = w.Seed.PasswordHint
	}
	fmt.Fprintf(out, "seed login: email %s, domain %s, password %s\n", w.Seed.Accounts[0].Email, w.Seed.Tenant, pw)
	return nil
}

// buildRow is one line of the build summary table.
type buildRow struct {
	image, status, reason string
	dur                   time.Duration
}

// maxReason caps the reason shown on a failed build's line.
const maxReason = 200

// oomHint follows a buildctl connection error: buildkitd was most likely
// OOM-killed by the build.
const oomHint = " (buildkitd restarted? check memory: kubectl -n zenify-system get pod -l app.kubernetes.io/name=buildkitd)"

// npmCodeRe matches an npm error line that carries a code or status
// (E401, 401, 404, ERESOLVE...), not the generic log-path line.
var npmCodeRe = regexp.MustCompile(`npm error (code )?(E[A-Z0-9]+|[0-9]{3})\b`)

// buildReason is the most useful line of a failed build's error text (already
// redacted): the last npm error line with a code, else the last line starting
// `error:` (buildctl's own), else the last line containing `ERROR:`, else the
// last line (buildkit ends with the whole RUN command). It is capped at 200
// characters; a connection loss to buildkitd gets oomHint appended.
func buildReason(err error) string {
	ls := strings.Split(strings.TrimSpace(err.Error()), "\n")
	last := func(match func(string) bool) string {
		for i := len(ls) - 1; i >= 0; i-- {
			if l := strings.TrimSpace(ls[i]); match(l) {
				return l
			}
		}
		return ""
	}
	pick := last(npmCodeRe.MatchString)
	if pick == "" {
		pick = last(func(l string) bool { return strings.HasPrefix(l, "error:") })
	}
	if pick == "" {
		pick = last(func(l string) bool { return strings.Contains(l, "ERROR:") })
	}
	if pick == "" {
		pick = strings.TrimSpace(ls[len(ls)-1])
	}
	lost := strings.Contains(pick, "Unavailable") || strings.HasSuffix(pick, "EOF")
	if r := []rune(pick); len(r) > maxReason {
		pick = string(r[:maxReason-3]) + "..."
	}
	if lost {
		pick += oomHint
	}
	return pick
}

// buildImages scans the workspace for images and builds them in order, one
// step each. A failed build marks its step and the run goes on. It returns
// repo → tag of the images that exist (built or tag already there) and the
// failed image names, after printing the summary table. The seed image
// failing is an error: without data the env is pointless.
func (d infraDeps) buildImages(ctx context.Context, v *stepView, w infra.Workspace, home, ws, domain, profile string, refs map[string]string) (map[string]string, []string, map[string]bool, error) {
	var ts []infra.BuildTarget
	if err := v.run(ctx, "buildctl", func(ctx context.Context) (stepOutcome, error) {
		if _, err := d.ensureBuildctl(ctx, d.runner, home); err != nil {
			return stepOutcome{}, err
		}
		var err error
		ts, err = d.scanTargets(ctx, d.runner, w, ws, domain, infra.BaseSlug, refs)
		if err == nil {
			err = checkAppRepos(w, ts)
		}
		return stepOutcome{detail: fmt.Sprintf("ready, %d image(s) to check", len(ts))}, err
	}); err != nil {
		return nil, nil, nil, err
	}
	// --ref is checked against builds[] and the scanned repos
	var valid []string
	for _, t := range ts {
		valid = append(valid, t.Repo)
	}
	for _, repo := range sortedRefRepos(refs) {
		if !slices.Contains(valid, repo) {
			return nil, nil, nil, exitcode.New(exitcode.BadArgs, fmt.Errorf("--ref: unknown repo %q (valid: %s)", repo, strings.Join(valid, ", ")))
		}
	}
	names := make([]string, len(ts))
	for i, t := range ts {
		names[i] = "build " + t.Image
	}
	v.addSteps("env-ca", names)
	images := map[string]string{}
	var rows []buildRow
	var failed []string
	failedRepos := map[string]bool{}
	seedImage := ""
	for i, t := range ts {
		start := time.Now()
		row := buildRow{image: t.Image}
		// the view prints a failure as a failed step and returns it; the
		// table and the final error report it, so the run goes on
		_ = v.run(ctx, names[i], func(ctx context.Context) (stepOutcome, error) {
			res, err := d.build(ctx, d.builder, ws, t, refs)
			if err != nil {
				v.logf("%s: %v", names[i], err)
				row.status, row.reason = "FAIL", buildReason(err)
				return stepOutcome{}, errors.New(row.reason)
			}
			images[t.Repo] = res.Tag
			if !res.Built {
				row.status, row.reason = "SKIP", "tag "+res.Tag+" exists"
				return stepOutcome{detail: res.Tag + " unchanged (tag exists)", skipped: true}, nil
			}
			row.status = "OK"
			return stepOutcome{detail: res.Tag + " built"}, nil
		})
		if ctx.Err() != nil {
			return nil, nil, nil, failStep(names[i], ctx.Err())
		}
		row.dur = time.Since(start).Round(time.Second)
		rows = append(rows, row)
		if row.status == "FAIL" {
			failed = append(failed, t.Image)
			failedRepos[t.Repo] = true
			if t.Repo == w.Seed.Repo {
				seedImage = t.Image
				break // fail fast: no data, no env, so build nothing else
			}
		}
	}
	v.printLines(summaryLines(rows))
	if seedImage != "" {
		return nil, nil, nil, failStep("build "+seedImage, errors.New("the seed image failed to build, nothing was deployed"))
	}
	v.printLines(noDeployLines(w, profile, rows, ts))
	return images, failed, failedRepos, nil
}

// checkAppRepos fails when an app of w (the active profile) names a repo that
// is neither a build nor a scanned target: nothing would build its image.
func checkAppRepos(w infra.Workspace, ts []infra.BuildTarget) error {
	built := map[string]bool{}
	for _, t := range ts {
		built[t.Repo] = true
	}
	for _, a := range w.EnvApps {
		if a.Repo != "" && !built[a.Repo] {
			return fmt.Errorf("envApps[%s].repo %s: no Dockerfile at its base ref and not in builds[]", a.Key, a.Repo)
		}
	}
	return nil
}

// summaryLines is the table `image · OK|FAIL|SKIP · duration · reason`.
func summaryLines(rows []buildRow) []string {
	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "image\tresult\ttime\treason")
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.image, r.status, r.dur, r.reason)
	}
	_ = tw.Flush() // a strings.Builder write cannot fail
	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
}

// noDeployLines names the scanned images (not the declared builds[] ones, which
// may be bases on purpose) that no envApps entry of the active profile uses: an
// image only an app outside the profile uses is "not in profile", any other is
// "no deploy config".
func noDeployLines(w infra.Workspace, profile string, rows []buildRow, ts []infra.BuildTarget) []string {
	declared := map[string]bool{}
	for _, b := range w.Builds {
		declared[b.Repo] = true
	}
	for _, a := range w.EnvApps {
		declared[a.Repo] = true
	}
	other := w.OutOfProfileRepos()
	var out []string
	for i, t := range ts {
		if declared[t.Repo] || i >= len(rows) {
			continue
		}
		if rows[i].status != "OK" && rows[i].status != "SKIP" {
			continue
		}
		switch {
		case other[t.Repo]:
			out = append(out, "image ready, not in profile "+profile+": "+t.Image)
		case rows[i].status == "OK":
			out = append(out, "built, no deploy config: "+t.Image)
		default:
			out = append(out, "image ready, no deploy config: "+t.Image)
		}
	}
	return out
}

// sortedRefRepos are the repos named by --ref, sorted.
func sortedRefRepos(refs map[string]string) []string {
	out := make([]string, 0, len(refs))
	for r := range refs {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// savedProfile is the profile saved in state (core when none); a value that is
// no profile is a bad argument naming state.json.
func savedProfile(st *infra.State) (string, error) {
	p := st.ActiveProfile()
	if !infra.ValidProfile(p) {
		return "", exitcode.New(exitcode.BadArgs, fmt.Errorf("state.json: profile %q is not %s or %s; fix or delete ~/.zenify/infra/state.json, or pass --profile", p, infra.ProfileCore, infra.ProfileFull))
	}
	return p, nil
}

// skippedLines is one line per app that is left out (`skipped`) or keeps its
// deployed image (`kept`) because its image failed.
func skippedLines(w infra.Workspace, skipped, keptKeys []string, tags map[string]string) []string {
	repo := map[string]string{}
	for _, a := range w.EnvApps {
		repo[a.Key] = a.Repo
	}
	var out []string
	for _, k := range keptKeys {
		out = append(out, fmt.Sprintf("kept %s at %s: image %s failed", k, tags[k], repo[k]))
	}
	for _, k := range skipped {
		out = append(out, fmt.Sprintf("skipped %s: image %s failed", k, repo[k]))
	}
	return out
}

// applyEnv publishes the local CA into env slug's namespace, then brings the
// env up in two phases: datastores + seed, then the apps. An env where any
// phase-2 app already exists skips phase 1: re-applying it would prune them.
func (d infraDeps) applyEnv(ctx context.Context, v *stepView, w infra.Workspace, slug, domain, chartRef string, images map[string]string, skip map[string]bool, kept map[string]string, caPEM []byte, prevRev string) (string, error) {
	r := d.runner
	root := infra.EnvAppName(slug)
	if err := v.run(ctx, "env-ca", func(ctx context.Context) (stepOutcome, error) {
		return stepOutcome{detail: infra.EnvNamespace(slug) + "/" + infra.EnvCAConfigMap + " applied"}, d.applyEnvCA(ctx, r, slug, caPEM)
	}); err != nil {
		return "", err
	}
	configRev := ""
	for phase := 1; phase <= 2; phase++ {
		if err := v.run(ctx, fmt.Sprintf("env-phase-%d", phase), func(ctx context.Context) (stepOutcome, error) {
			if phase == 1 {
				for _, name := range infra.PhaseTwoApps(w, slug) {
					ok, err := d.appExists(ctx, r, name)
					if err != nil {
						return stepOutcome{}, err
					}
					if ok {
						return stepOutcome{detail: "skipped (apps already deployed)", skipped: true}, nil
					}
				}
			}
			// ArgoCD may serve a cached helm commit; refresh before anything reads it
			refresh := append([]string{root}, infra.ChildApps(w, slug, 2, nil)...)
			if phase == 2 {
				if err := d.refreshApps(ctx, r, refresh); err != nil {
					return stepOutcome{}, err
				}
				// auto-sync runs no hooks when nothing in the diff changed: sync the
				// seed app (its PreSync hook adds new Secret keys) before the apps start,
				// unless it is already Synced/Healthy at the revision the last up synced
				if seed := infra.SeedAppName(w, slug); seed != "" {
					rev, synced, err := d.syncedRevision(ctx, r, seed)
					if err != nil {
						return stepOutcome{}, err
					}
					if synced && prevRev != "" && rev == prevRev {
						configRev = rev
					} else if configRev, err = d.syncApp(ctx, r, seed, envTimeout); err != nil {
						return stepOutcome{}, err
					}
				}
			}
			m, err := infra.RenderEnvAppRev(w, slug, domain, chartRef, configRev, images, phase, skip, kept)
			if err != nil {
				return stepOutcome{}, err
			}
			changed, err := d.apply(ctx, r, m)
			if err != nil {
				return stepOutcome{}, err
			}
			if phase == 1 {
				if err := d.refreshApps(ctx, r, refresh); err != nil {
					return stepOutcome{}, err
				}
			}
			for _, name := range append([]string{root}, infra.ChildApps(w, slug, phase, skip)...) {
				if err := d.waitApp(ctx, r, name, envTimeout); err != nil {
					return stepOutcome{}, err
				}
			}
			return stepOutcome{detail: changedWord(changed) + ", Synced/Healthy"}, nil
		}); err != nil {
			return "", err
		}
	}
	return configRev, nil
}

func (d infraDeps) statusCmd() *cobra.Command {
	var local, asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Trạng thái k3s, ArgoCD, platform, env, DNS và CA", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireLocal(local); err != nil {
				return err
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			home, cfg, err := d.loadConfig()
			if err != nil {
				return failStep("config", err)
			}
			tg, err := d.resolveTarget(ctx, home, cfg, "")
			if err != nil {
				return failStep("target", err)
			}
			w, err := d.workspaceSpec()
			if err != nil {
				return failStep("workspace", err)
			}
			profile, err := savedProfile(tg.state)
			if err != nil {
				return err
			}
			w = w.ForProfile(profile)
			rep, err := d.status(ctx, d.runner, w, home, tg.domain)
			if err != nil {
				return exitcode.New(exitcode.Fail, err) // already prefixed "status:"
			}
			out := cmd.OutOrStdout()
			if asJSON {
				b, err := json.MarshalIndent(rep, "", "  ")
				if err != nil {
					return failStep("status", err)
				}
				fmt.Fprintln(out, string(b))
				return nil
			}
			fmt.Fprintf(out, "k3s       %s\nargocd    %s\nplatform  %s/%s\n", rep.K3s, rep.ArgoCD, rep.Platform.Sync, rep.Platform.Health)
			for _, e := range rep.Envs {
				fmt.Fprintf(out, "env %-5s %s/%s %s\n", e.Slug, e.Sync, e.Health, strings.Join(e.URLs, " "))
			}
			fmt.Fprintf(out, "dns       %s resolves=%v\nca        trusted=%v\n", rep.DNS.Host, rep.DNS.Resolves, rep.CA.Trusted)
			return nil
		},
	}
	cmd.Flags().BoolVar(&local, "local", false, "target the local k3s devbox (required)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func (d infraDeps) showCmd() *cobra.Command {
	var (
		local, asJSON bool
		slug          string
	)
	cmd := &cobra.Command{
		Use:   "show",
		Short: "In URL, image tag của từng env, tài khoản seed và cách vào cluster", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireLocal(local); err != nil {
				return err
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			home, cfg, err := d.loadConfig()
			if err != nil {
				return failStep("config", err)
			}
			tg, err := d.resolveTarget(ctx, home, cfg, "")
			if err != nil {
				return failStep("target", err)
			}
			w, err := d.workspaceSpec()
			if err != nil {
				return failStep("workspace", err)
			}
			profile, err := savedProfile(tg.state)
			if err != nil {
				return err
			}
			w = w.ForProfile(profile)
			rep, err := d.show(ctx, d.runner, w, home, tg.domain, slug)
			if err != nil {
				return exitcode.New(exitcode.Fail, err) // already prefixed "show:"
			}
			out := cmd.OutOrStdout()
			if asJSON {
				b, err := json.MarshalIndent(rep, "", "  ")
				if err != nil {
					return failStep("show", err)
				}
				fmt.Fprintln(out, string(b))
				return nil
			}
			fmt.Fprintf(out, "domain    %s\nca        %s\n", rep.Domain, rep.CA)
			if len(rep.Envs) == 0 {
				fmt.Fprintln(out, "envs      none (run: zenify infra up --local)")
			}
			for _, e := range rep.Envs {
				fmt.Fprintf(out, "env %s    %s/%s\n", e.Slug, e.Sync, e.Health)
				for _, a := range e.Apps {
					tag := a.Tag
					if tag == "" {
						tag = "-"
					}
					fmt.Fprintf(out, "  %-4s %-40s %-20s %s\n", a.Svc, a.URL, a.Repo, tag)
				}
			}
			tenant := ""
			if len(rep.Accounts) > 0 {
				tenant = rep.Accounts[0].Tenant
			}
			if rep.LoginHelp != "" {
				fmt.Fprintf(out, "accounts  tenant domain %s, password: %s\n", tenant, rep.LoginHelp)
			} else {
				fmt.Fprintf(out, "accounts  tenant domain %s\n", tenant)
			}
			for _, a := range rep.Accounts {
				fmt.Fprintf(out, "  %-14s %-36s %s\n", a.Role, a.Email, a.Password)
			}
			printCluster(out, rep.Cluster)
			return nil
		},
	}
	cmd.Flags().BoolVar(&local, "local", false, "target the local k3s devbox (required)")
	cmd.Flags().StringVar(&slug, "slug", "", "show only this env")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

// printCluster prints how to reach the cluster; rows for parts that are not
// installed are left out.
func printCluster(out io.Writer, c infra.ShowCluster) {
	fmt.Fprintln(out, "cluster")
	fmt.Fprintf(out, "  kubeconfig  %s (%s)\n", c.Kubeconfig, c.KubeconfigNote)
	fmt.Fprintf(out, "              %s\n", c.Export)
	if c.Context != "" {
		fmt.Fprintf(out, "  context     %s\n", c.Context)
	}
	for _, n := range c.Nodes {
		fmt.Fprintf(out, "  node        %s %s\n", n.Name, n.Version)
	}
	fmt.Fprintf(out, "  registry    %s (pods pull), %s (host)\n", c.Registry.Alias, c.Registry.Address)
	fmt.Fprintf(out, "              push: %s\n", c.Registry.Push)
	fmt.Fprintf(out, "              pull: %s\n", c.Registry.Pull)
	fmt.Fprintf(out, "              list: %s\n", c.Registry.List)
	if a := c.ArgoCD; a != nil {
		fmt.Fprintf(out, "  argocd      %s (in-cluster)\n", a.URL)
		if a.IngressURL != "" {
			fmt.Fprintf(out, "              %s\n", a.IngressURL)
		}
		if a.Password != "" {
			fmt.Fprintf(out, "              login: admin / %s\n", a.Password)
		} else {
			fmt.Fprintf(out, "              admin password: %s (initial secret deleted)\n", a.PasswordCommand)
		}
	}
	if c.OpenBao != "" {
		fmt.Fprintf(out, "  openbao     %s\n", c.OpenBao)
	}
}

func (d infraDeps) downCmd() *cobra.Command {
	var local, purge, applyFlag bool
	cmd := &cobra.Command{
		Use:   "down",
		Short: "Gỡ env và platform (mặc định preview; --apply để thực thi)", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireLocal(local); err != nil {
				return err
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return d.runDown(ctx, cmd.OutOrStdout(), purge, applyFlag)
		},
	}
	cmd.Flags().BoolVar(&local, "local", false, "target the local k3s devbox (required)")
	cmd.Flags().BoolVar(&purge, "purge", false, "also remove the CA trust, split DNS, k3s and its firewall, and the state file")
	cmd.Flags().BoolVar(&applyFlag, "apply", false, "execute (default: preview only)")
	return cmd
}

func (d infraDeps) runDown(ctx context.Context, out io.Writer, purge, applyFlag bool) error {
	if !applyFlag {
		fmt.Fprintln(out, "zenify infra down: PREVIEW (nothing changes; pass --apply to execute)")
		fmt.Fprintln(out, "would delete every env-<slug> Application (waiting for its finalizer) and namespace env-<slug>")
		fmt.Fprintf(out, "would delete Application %s with its resources, and namespace %s\n", infra.PlatformApp, infra.PlatformNamespace)
		if purge {
			fmt.Fprintln(out, "would remove the local CA trust (system + NSS) and the split DNS unit")
			fmt.Fprintln(out, "would run k3s-uninstall.sh, then remove the k3s firewall unit")
			fmt.Fprintln(out, "would remove ~/.zenify/infra/state.json")
		}
		return nil
	}
	home, err := d.home()
	if err != nil {
		return failStep("home", err)
	}
	ctx, stopSignals := signal.NotifyContext(ctx, os.Interrupt)
	defer stopSignals()
	unlock, err := d.lock(home)
	if err != nil {
		return failStep("lock", err)
	}
	defer func() {
		if uerr := unlock(); uerr != nil {
			fmt.Fprintf(out, "warning: release infra lock: %v\n", uerr)
		}
	}()
	logFile, logPath, err := openRunLog(home, "down")
	if err != nil {
		return failStep("log", err)
	}
	defer func() {
		if cerr := logFile.Close(); cerr != nil {
			fmt.Fprintf(out, "warning: close run log: %v\n", cerr)
		}
	}()
	steps := []string{"detect", "envs", "platform"}
	if purge {
		steps = append(steps, "trust-dns", "k3s", "state")
	}
	v := newStepView(out, "zenify infra down", "zenify infra down --local --apply", steps, false, logFile, logPath)
	r := d.runner
	argo := ""
	if err := v.run(ctx, "detect", func(ctx context.Context) (stepOutcome, error) {
		k3s, err := d.k3sVersion(ctx, r)
		if err != nil {
			return stepOutcome{}, fmt.Errorf("k3s: %w", err)
		}
		if k3s == "" {
			return stepOutcome{detail: "k3s not installed"}, nil
		}
		if argo, err = d.argoVersion(ctx, r); err != nil {
			return stepOutcome{}, fmt.Errorf("argocd: %w", err)
		}
		if argo == "" {
			return stepOutcome{detail: "k3s " + k3s + ", ArgoCD not installed"}, nil
		}
		return stepOutcome{detail: "k3s " + k3s + ", ArgoCD " + argo}, nil
	}); err != nil {
		return err
	}
	if err := v.run(ctx, "envs", func(ctx context.Context) (stepOutcome, error) {
		if argo == "" {
			return stepOutcome{detail: "nothing to remove (k3s or ArgoCD not installed)", skipped: true}, nil
		}
		envs, err := d.envApps(ctx, r)
		if err != nil {
			return stepOutcome{}, err
		}
		for _, name := range envs {
			v.Sub("deleting " + name)
			if err := d.deleteApp(ctx, r, name, false); err != nil {
				return stepOutcome{}, err
			}
			if err := d.deleteNamespace(ctx, r, name); err != nil { // root name == namespace env-<slug>
				return stepOutcome{}, err
			}
		}
		if len(envs) == 0 {
			return stepOutcome{detail: "none", skipped: true}, nil
		}
		return stepOutcome{detail: "deleted " + strings.Join(envs, ", ")}, nil
	}); err != nil {
		return err
	}
	if err := v.run(ctx, "platform", func(ctx context.Context) (stepOutcome, error) {
		if argo == "" {
			return stepOutcome{detail: "nothing to remove (k3s or ArgoCD not installed)", skipped: true}, nil
		}
		exists, err := d.appExists(ctx, r, infra.PlatformApp)
		if err != nil {
			return stepOutcome{}, err
		}
		if exists {
			v.Sub("deleting application " + infra.PlatformApp)
			if err := d.deleteApp(ctx, r, infra.PlatformApp, true); err != nil {
				return stepOutcome{}, err
			}
		}
		v.Sub("deleting namespace " + infra.PlatformNamespace)
		if err := d.deleteNamespace(ctx, r, infra.PlatformNamespace); err != nil {
			return stepOutcome{}, err
		}
		return stepOutcome{detail: "deleted " + infra.PlatformApp}, nil
	}); err != nil {
		return err
	}
	if !purge {
		v.close("removed")
		return nil
	}
	if err := v.run(ctx, "trust-dns", func(ctx context.Context) (stepOutcome, error) {
		return stepOutcome{detail: "CA trust and split DNS removed"}, d.removeTrust(ctx, r, home)
	}); err != nil {
		return err
	}
	if err := v.run(ctx, "k3s", func(ctx context.Context) (stepOutcome, error) {
		return stepOutcome{detail: "k3s and its firewall removed"}, d.uninstallK3s(ctx, r)
	}); err != nil {
		return err
	}
	if err := v.run(ctx, "state", func(context.Context) (stepOutcome, error) {
		return stepOutcome{detail: "state removed"}, d.removeState(home)
	}); err != nil {
		return err
	}
	v.close("purged")
	return nil
}

func (d infraDeps) configCmd() *cobra.Command {
	var (
		local   bool
		domain  string
		maxEnvs int
	)
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Xem hoặc đặt config devbox cục bộ (domain, số env tối đa)", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireLocal(local); err != nil {
				return err
			}
			home, err := d.home()
			if err != nil {
				return failStep("config", err)
			}
			cfg, err := infra.LoadConfig(home)
			if err != nil {
				return failStep("config", err)
			}
			setDomain, setMax := cmd.Flags().Changed("domain"), cmd.Flags().Changed("max-envs")
			if setDomain {
				if err := infra.ValidateDomain(domain); err != nil {
					return exitcode.New(exitcode.BadArgs, err)
				}
				cfg.Domain = domain
			}
			if setMax {
				if maxEnvs < 1 {
					return exitcode.New(exitcode.BadArgs, fmt.Errorf("--max-envs must be at least 1, got %d", maxEnvs))
				}
				cfg.MaxEnvs = maxEnvs
			}
			if err := cfg.Validate(); err != nil {
				return failStep("config", fmt.Errorf("%s: %w", infra.ConfigPath(home), err))
			}
			if setDomain || setMax {
				if err := infra.SaveConfig(home, cfg); err != nil {
					return failStep("config", err)
				}
			}
			dom, ref := cfg.Domain, cfg.ChartRef
			if dom == "" {
				dom = "(unset: state, else the deployed platform, else " + infra.DefaultDomain + ")"
			}
			if ref == "" {
				ref = "(unset: the deployed platform, else main)"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "domain    %s\nmaxEnvs   %d\nchartRef  %s\n", dom, cfg.MaxEnvs, ref)
			return nil
		},
	}
	cmd.Flags().BoolVar(&local, "local", false, "target the local k3s devbox (required)")
	cmd.Flags().StringVar(&domain, "domain", "", "env domain, must end in .test")
	cmd.Flags().IntVar(&maxEnvs, "max-envs", 0, "maximum envs on this devbox (at least 1)")
	return cmd
}
