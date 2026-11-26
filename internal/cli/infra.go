package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
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
	preflight func(ws string) []infra.Problem

	installK3s     func(ctx context.Context, r infra.Runner) (bool, error)
	installArgo    func(ctx context.Context, r infra.Runner) (bool, error)
	setRepoCred    func(ctx context.Context, r infra.Runner, token string) error
	applyPlatform  func(ctx context.Context, r infra.Runner, domain, chartRef string) error
	waitApp        func(ctx context.Context, r infra.Runner, name string, timeout time.Duration) error
	fetchCA        func(ctx context.Context, r infra.Runner) ([]byte, error)
	installCA      func(ctx context.Context, r infra.Runner, home string, pem []byte) (bool, error)
	splitDNS       func(ctx context.Context, r infra.Runner, domain string) (bool, error)
	restartDNS     func(ctx context.Context, r infra.Runner) error
	ensureBuildctl func(ctx context.Context, r infra.Runner, home string) (string, error)
	coreTargets    func(ws, domain, slug string) ([]infra.BuildTarget, error)
	build          func(ctx context.Context, br infra.BuildRunner, ws string, t infra.BuildTarget, refs map[string]string) (infra.BuildResult, error)
	apply          func(ctx context.Context, r infra.Runner, manifest []byte) (bool, error)
	appExists      func(ctx context.Context, r infra.Runner, name string) (bool, error)
	saveState      func(home string, s *infra.State) error
	livePlatform   func(ctx context.Context, r infra.Runner) (infra.PlatformTarget, bool, error)

	status          func(ctx context.Context, r infra.Runner, home, domain string) (infra.Report, error)
	show            func(ctx context.Context, r infra.Runner, home, domain, slug string) (infra.ShowReport, error)
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
		preflight: func(string) []infra.Problem {
			return infra.Preflight(infra.RealHostProbe(), infra.DefaultRequirements, infra.K3sVersion)
		},
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
		coreTargets:     infra.CoreTargets,
		build:           infra.BuildFromBase,
		apply:           infra.Apply,
		appExists:       infra.AppExists,
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
		local    bool
		chartRef string
		refFlags []string
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
			valid := infra.CoreRepos()
			for repo := range refs {
				if !slices.Contains(valid, repo) {
					return exitcode.New(exitcode.BadArgs, fmt.Errorf("--ref: unknown repo %q (valid: %s)", repo, strings.Join(valid, ", ")))
				}
			}
			return d.runUp(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), chartRef, refs)
		},
	}
	cmd.Flags().BoolVar(&local, "local", false, "target the local k3s devbox (required)")
	cmd.Flags().StringVar(&chartRef, "chart-ref", "", "helm repo ref for the platform and env charts (default: config, else the deployed platform's, else main)")
	cmd.Flags().StringArrayVar(&refFlags, "ref", nil, "build <repo>=<ref> instead of the repo's baseRef (repeatable)")
	return cmd
}

func (d infraDeps) runUp(ctx context.Context, out, errW io.Writer, chartRef string, refs map[string]string) error {
	if ctx == nil {
		ctx = context.Background()
	}
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
	if ps := d.preflight(ws); len(ps) > 0 {
		for _, p := range ps {
			fmt.Fprintf(errW, "problem %s: %s\n  fix: %s\n", p.Check, p.Detail, p.Fix)
		}
		return failStep("preflight", fmt.Errorf("%d problem(s), nothing was changed", len(ps)))
	}
	fmt.Fprintln(out, "preflight      ok")
	tg, err := d.resolveTarget(ctx, home, cfg, chartRef)
	if err != nil {
		return failStep("target", err)
	}
	if err := tg.drift(); err != nil {
		return failStep("target", err)
	}
	domain, chartRef := tg.domain, tg.chartRef
	fmt.Fprintf(out, "target         domain %s (%s), chart ref %s (%s)\n", domain, tg.domainFrom, chartRef, tg.refFrom)
	r := d.runner

	changed, err := d.installK3s(ctx, r)
	if err != nil {
		return failStep("k3s", err)
	}
	fmt.Fprintf(out, "k3s            %s\n", changedWord(changed))
	if changed, err = d.installArgo(ctx, r); err != nil {
		return failStep("argocd", err)
	}
	fmt.Fprintf(out, "argocd         %s\n", changedWord(changed))
	tok, err := r.Run(ctx, nil, "gh", "auth", "token")
	if err != nil {
		return failStep("repo-credential", fmt.Errorf("gh auth token: %w", err))
	}
	if err := d.setRepoCred(ctx, r, strings.TrimSpace(string(tok))); err != nil {
		return failStep("repo-credential", err)
	}
	fmt.Fprintln(out, "repo-credential applied")
	if err := d.applyPlatform(ctx, r, domain, chartRef); err != nil {
		return failStep("platform", err)
	}
	if err := d.waitApp(ctx, r, infra.PlatformApp, platformTimeout); err != nil {
		return failStep("platform", err)
	}
	fmt.Fprintf(out, "platform       %s Synced/Healthy at %s\n", infra.PlatformApp, chartRef)
	pem, err := d.fetchCA(ctx, r)
	if err != nil {
		return failStep("ca", err)
	}
	if changed, err = d.installCA(ctx, r, home, pem); err != nil {
		return failStep("ca", err)
	}
	fmt.Fprintf(out, "ca             %s\n", changedWord(changed))
	if changed, err = d.splitDNS(ctx, r, domain); err != nil {
		return failStep("dns", err)
	}
	// always restart: cni0 may have been recreated since the unit last ran
	if err := d.restartDNS(ctx, r); err != nil {
		return failStep("dns", err)
	}
	fmt.Fprintf(out, "dns            %s (restarted)\n", changedWord(changed))

	images, err := d.buildImages(ctx, out, home, ws, domain, refs)
	if err != nil {
		return failStep("build", err)
	}
	slug := infra.BaseSlug
	if err := d.applyEnv(ctx, out, slug, domain, chartRef, images); err != nil {
		return err
	}
	created := time.Now().UTC()
	if tg.state != nil && !tg.state.CreatedAt.IsZero() {
		created = tg.state.CreatedAt // a rerun keeps when the devbox was first set up
	}
	if err := d.saveState(home, &infra.State{Domain: domain, K3sVersion: infra.K3sVersion, ArgoCDVersion: infra.ArgoCDVersion, CreatedAt: created}); err != nil {
		return failStep("state", err)
	}
	fmt.Fprintf(out, "\nready: https://%s\n", infra.Host("web", slug, domain))
	fmt.Fprintf(out, "seed login: email %s, domain %s\n", infra.SeedAccounts[0].Email, infra.SeedTenant)
	return nil
}

// buildImages builds base-spa then the core images; it returns repo → tag.
func (d infraDeps) buildImages(ctx context.Context, out io.Writer, home, ws, domain string, refs map[string]string) (map[string]string, error) {
	if _, err := d.ensureBuildctl(ctx, d.runner, home); err != nil {
		return nil, err
	}
	ts, err := d.coreTargets(ws, domain, infra.BaseSlug)
	if err != nil {
		return nil, err
	}
	images := map[string]string{}
	for _, t := range ts {
		res, err := d.build(ctx, d.builder, ws, t, refs)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(out, "build          %s:%s %s (%s)\n", res.Image, res.Tag, changedWord(res.Built), res.Duration.Round(time.Second))
		images[t.Repo] = res.Tag
	}
	return images, nil
}

// applyEnv brings env slug up in two phases: datastores + seed, then the apps.
// An env whose apps already exist skips phase 1: re-applying it would prune them.
func (d infraDeps) applyEnv(ctx context.Context, out io.Writer, slug, domain, chartRef string, images map[string]string) error {
	r := d.runner
	root := infra.EnvAppName(slug)
	hasApps, err := d.appExists(ctx, r, infra.AppsProbe(slug))
	if err != nil {
		return failStep("env-phase-1", err)
	}
	for phase := 1; phase <= 2; phase++ {
		step := fmt.Sprintf("env-phase-%d", phase)
		if phase == 1 && hasApps {
			fmt.Fprintf(out, "%-14s skipped (apps already deployed)\n", step)
			continue
		}
		m, err := infra.RenderEnvApp(slug, domain, chartRef, images, infra.ProfileCore, phase)
		if err != nil {
			return failStep(step, err)
		}
		changed, err := d.apply(ctx, r, m)
		if err != nil {
			return failStep(step, err)
		}
		for _, name := range append([]string{root}, infra.ChildApps(slug, phase)...) {
			if err := d.waitApp(ctx, r, name, envTimeout); err != nil {
				return failStep(step, err)
			}
		}
		fmt.Fprintf(out, "%-14s %s, Synced/Healthy\n", step, changedWord(changed))
	}
	return nil
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
			rep, err := d.status(ctx, d.runner, home, tg.domain)
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

// seedPasswordHint is printed instead of any password.
const seedPasswordHint = "password: see zenify-data-seed README (preview)"

func (d infraDeps) showCmd() *cobra.Command {
	var (
		local, asJSON bool
		slug          string
	)
	cmd := &cobra.Command{
		Use:   "show",
		Short: "In URL, repo, image tag của từng env và tài khoản seed", //znf:allow-lang
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
			rep, err := d.show(ctx, d.runner, home, tg.domain, slug)
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
			fmt.Fprintf(out, "accounts  tenant domain %s, %s\n", infra.SeedTenant, seedPasswordHint)
			for _, a := range rep.Accounts {
				fmt.Fprintf(out, "  %-14s %s\n", a.Role, a.Email)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&local, "local", false, "target the local k3s devbox (required)")
	cmd.Flags().StringVar(&slug, "slug", "", "show only this env")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
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
	r := d.runner
	k3s, err := d.k3sVersion(ctx, r)
	if err != nil {
		return failStep("k3s", err)
	}
	argo := ""
	if k3s != "" {
		if argo, err = d.argoVersion(ctx, r); err != nil {
			return failStep("argocd", err)
		}
	}
	if argo != "" {
		envs, err := d.envApps(ctx, r)
		if err != nil {
			return failStep("envs", err)
		}
		for _, name := range envs {
			if err := d.deleteApp(ctx, r, name, false); err != nil {
				return failStep("envs", err)
			}
			if err := d.deleteNamespace(ctx, r, name); err != nil { // root name == namespace env-<slug>
				return failStep("envs", err)
			}
			fmt.Fprintf(out, "env            deleted %s\n", name)
		}
		exists, err := d.appExists(ctx, r, infra.PlatformApp)
		if err != nil {
			return failStep("platform", err)
		}
		if exists {
			if err := d.deleteApp(ctx, r, infra.PlatformApp, true); err != nil {
				return failStep("platform", err)
			}
		}
		if err := d.deleteNamespace(ctx, r, infra.PlatformNamespace); err != nil {
			return failStep("platform", err)
		}
		fmt.Fprintf(out, "platform       deleted %s\n", infra.PlatformApp)
	}
	if argo == "" {
		fmt.Fprintln(out, "cluster        nothing to remove (k3s or ArgoCD not installed)")
	}
	if !purge {
		return nil
	}
	if err := d.removeTrust(ctx, r, home); err != nil {
		return failStep("purge", err)
	}
	fmt.Fprintln(out, "purge          CA trust and split DNS removed")
	if err := d.uninstallK3s(ctx, r); err != nil {
		return failStep("purge", err)
	}
	fmt.Fprintln(out, "purge          k3s and its firewall removed")
	if err := d.removeState(home); err != nil {
		return failStep("purge", err)
	}
	fmt.Fprintln(out, "purge          state removed")
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
