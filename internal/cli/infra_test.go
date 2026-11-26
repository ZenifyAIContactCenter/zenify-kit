package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/infra"
)

// infraRunner records commands; gh auth token answers a fixed token.
type infraRunner struct{ calls []string }

func (r *infraRunner) Run(_ context.Context, _ io.Reader, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, strings.Join(append([]string{name}, args...), " "))
	if name == "gh" && len(args) > 0 && args[0] == "auth" {
		return []byte("tok-secret\n"), nil
	}
	return nil, nil
}

func (r *infraRunner) RunEnv(ctx context.Context, _ []string, name string, args ...string) ([]byte, error) {
	return r.Run(ctx, nil, name, args...)
}

func (r *infraRunner) Start(context.Context, string, ...string) (func() error, error) {
	return func() error { return nil }, nil
}

// testWorkspace is the neutral fixture infra.json shared with internal/infra.
func testWorkspace(t *testing.T) infra.Workspace {
	t.Helper()
	w, err := infra.LoadWorkspace("../infra/testdata/infra.json")
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// fakeInfra fills every step with a recorder; k3s/argo report alreadyThere.
func fakeInfra(t *testing.T, alreadyThere bool) (infraDeps, *[]string, *infraRunner) {
	t.Helper()
	home := t.TempDir()
	steps := &[]string{}
	rec := func(s string) { *steps = append(*steps, s) }
	r := &infraRunner{}
	d := infraDeps{
		home:      func() (string, error) { return home, nil },
		workspace: func() (string, bool, error) { return "/ws", true, nil },
		runner:    r,
		builder:   r,
		preflight: func(string) []infra.Problem { rec("preflight"); return nil },
		spec:      func(string) (infra.Workspace, error) { return testWorkspace(t), nil },
		lock: func(string) (func() error, error) {
			rec("lock")
			return func() error { rec("unlock"); return nil }, nil
		},
		applyEnvCA: func(_ context.Context, _ infra.Runner, slug string, pem []byte) error {
			rec("env-ca " + slug + " " + string(pem))
			return nil
		},
		installK3s: func(context.Context, infra.Runner) (bool, error) {
			rec("k3s")
			return !alreadyThere, nil
		},
		installArgo: func(context.Context, infra.Runner) (bool, error) {
			rec("argocd")
			return !alreadyThere, nil
		},
		setRepoCred: func(_ context.Context, _ infra.Runner, repoURL, tok string) error {
			if tok != "tok-secret" || repoURL != "https://github.com/example-team/charts.git" {
				t.Errorf("token = %q, repo = %q", tok, repoURL)
			}
			rec("repo-credential")
			return nil
		},
		applyPlatform: func(context.Context, infra.Runner, string, string, string) error { rec("platform"); return nil },
		waitApp: func(_ context.Context, _ infra.Runner, name string, _ time.Duration) error {
			rec("wait " + name)
			return nil
		},
		fetchCA:    func(context.Context, infra.Runner) ([]byte, error) { rec("fetch-ca"); return []byte("pem"), nil },
		installCA:  func(context.Context, infra.Runner, string, []byte) (bool, error) { rec("ca"); return false, nil },
		splitDNS:   func(context.Context, infra.Runner, string) (bool, error) { rec("dns"); return false, nil },
		restartDNS: func(context.Context, infra.Runner) error { rec("dns-restart"); return nil },
		ensureBuildctl: func(context.Context, infra.Runner, string) (string, error) {
			rec("buildctl")
			return "/bin/buildctl", nil
		},
		deployedTags: func(context.Context, infra.Runner, infra.Workspace, string) (map[string]string, error) {
			return nil, nil
		},
		scanTargets: func(_ context.Context, _ infra.Runner, w infra.Workspace, _, _, _ string, _ map[string]string) ([]infra.BuildTarget, error) {
			var ts []infra.BuildTarget
			for _, b := range w.Builds {
				ts = append(ts, infra.BuildTarget{Repo: b.Repo, Image: b.Image})
			}
			ts = append(ts, infra.BuildTarget{Repo: "scan-tool", Image: "scan-tool"}) // scanned, not in builds[]
			return ts, nil
		},
		build: func(_ context.Context, _ infra.BuildRunner, _ string, tg infra.BuildTarget, _ map[string]string) (infra.BuildResult, error) {
			rec("build " + tg.Repo)
			return infra.BuildResult{Image: tg.Repo, Tag: "sha-" + tg.Repo}, nil
		},
		apply: func(_ context.Context, _ infra.Runner, m []byte) (bool, error) {
			if bytes.Contains(m, []byte("enabled: false")) {
				rec("apply phase 1")
			} else {
				rec("apply phase 2")
			}
			return true, nil
		},
		saveState: func(string, *infra.State) error { rec("save-state"); return nil },
		livePlatform: func(context.Context, infra.Runner) (infra.PlatformTarget, bool, error) {
			return infra.PlatformTarget{}, false, nil
		},
		k3sVersion:  func(context.Context, infra.Runner) (string, error) { return infra.K3sVersion, nil },
		argoVersion: func(context.Context, infra.Runner) (string, error) { return infra.ArgoCDVersion, nil },
		envApps: func(context.Context, infra.Runner) ([]string, error) {
			return []string{"env-base"}, nil
		},
		deleteApp: func(_ context.Context, _ infra.Runner, name string, _ bool) error {
			rec("delete-app " + name)
			return nil
		},
		deleteNamespace: func(_ context.Context, _ infra.Runner, ns string) error {
			rec("delete-ns " + ns)
			return nil
		},
		removeTrust:  func(context.Context, infra.Runner, string) error { rec("remove-trust"); return nil },
		uninstallK3s: func(context.Context, infra.Runner) error { rec("uninstall-k3s"); return nil },
		removeState:  func(string) error { rec("remove-state"); return nil },
	}
	d.appExists = func(_ context.Context, _ infra.Runner, name string) (bool, error) {
		return name == infra.PlatformApp, nil
	}
	return d, steps, r
}

func runInfra(d infraDeps, args ...string) (string, string, error) {
	cmd := newInfraCmdWith(d)
	var out, errW bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errW)
	cmd.SetArgs(args)
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	err := cmd.Execute()
	return out.String(), errW.String(), err
}

func indexOf(steps []string, s string) int {
	for i, v := range steps {
		if v == s {
			return i
		}
	}
	return -1
}

// A host that fails preflight must stay untouched (SC-3): no k3s install.
func TestInfraUp_PreflightProblemsStopBeforeAnyChange(t *testing.T) {
	d, steps, _ := fakeInfra(t, false)
	d.preflight = func(string) []infra.Problem {
		return []infra.Problem{{Check: "build-secrets", Detail: "NPM_TOKEN is not set and gh auth token failed", Fix: "export NPM_TOKEN or run: gh auth login"}}
	}
	_, errOut, err := runInfra(d, "up", "--local")
	if exitcode.Code(err) == 0 {
		t.Fatal("want non-zero exit")
	}
	if !strings.Contains(errOut, "build-secrets") || !strings.Contains(errOut, "NPM_TOKEN") {
		t.Errorf("problem not printed: %q", errOut)
	}
	if len(*steps) != 0 {
		t.Errorf("steps ran after a failed preflight: %v", *steps)
	}
}

// failBuilds makes the builds of the listed repos fail with a multi-line
// error whose last line is the cause.
func failBuilds(d *infraDeps, repos ...string) {
	ok := d.build
	d.build = func(ctx context.Context, br infra.BuildRunner, ws string, t infra.BuildTarget, refs map[string]string) (infra.BuildResult, error) {
		if slices.Contains(repos, t.Repo) {
			return infra.BuildResult{}, errors.New("build " + t.Repo + ": buildctl failed\nstep 4/9\nnpm ERR! 401 unauthorized")
		}
		res, err := ok(ctx, br, ws, t, refs)
		res.Built = true
		return res, err
	}
}

// hasRow reports whether a table line has image as its first and status as
// its second field, whatever the column spacing.
func hasRow(out, image, status string) bool {
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(strings.TrimPrefix(strings.TrimSpace(l), "│"))
		if len(f) >= 2 && f[0] == image && f[1] == status {
			return true
		}
	}
	return false
}

// A failed image does not stop up (FR-13.3): the next image is still built,
// the table says FAIL with the cause, the app that needs it is left out of
// phase 2 (FR-13.4), the rest deploys and state is saved, and the exit is
// non-zero naming the app and the image (FR-13.6, SC-29).
func TestInfraUp_FailedImageContinuesAndSkipsItsApp(t *testing.T) {
	d, steps, _ := fakeInfra(t, false)
	failBuilds(&d, "shop-gateway") // the middle one of shop-api, shop-gateway, shop-web
	var phase2 []byte
	d.apply = func(_ context.Context, _ infra.Runner, m []byte) (bool, error) {
		if bytes.Contains(m, []byte("tag: sha-shop-api")) {
			phase2 = m
		}
		*steps = append(*steps, "apply")
		return true, nil
	}
	out, _, err := runInfra(d, "up", "--local")
	if exitcode.Code(err) == 0 || err == nil {
		t.Fatal("want a non-zero exit")
	}
	for _, want := range []string{"shop-gateway", "image(s) failed", "app(s) skipped"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
	if indexOf(*steps, "build shop-web") < 0 {
		t.Errorf("the image after the failed one was not built: %v", *steps)
	}
	if indexOf(*steps, "save-state") < 0 {
		t.Errorf("state was not saved: %v", *steps)
	}
	for _, want := range []string{"✗", "npm ERR! 401 unauthorized",
		"skipped shop-gateway: image shop-gateway failed", "built, no deploy config: scan-tool"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	for img, status := range map[string]string{"shop-gateway": "FAIL", "shop-web": "OK"} {
		if !hasRow(out, img, status) {
			t.Errorf("table has no %s row with %s:\n%s", img, status, out)
		}
	}
	if strings.Contains(out, "no deploy config: team/base-ui") {
		t.Errorf("a declared builds[] base image got the no-deploy line:\n%s", out)
	}
	if strings.Contains(out, "step 4/9") {
		t.Errorf("the screen shows more than the last error line:\n%s", out)
	}
	if phase2 == nil {
		t.Fatal("phase 2 was not applied")
	}
	var app struct {
		Spec struct {
			Source struct {
				Helm struct {
					Values map[string]any `yaml:"valuesObject"`
				} `yaml:"helm"`
			} `yaml:"source"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(phase2, &app); err != nil {
		t.Fatal(err)
	}
	vals := map[string]map[string]any{}
	for k, v := range app.Spec.Source.Helm.Values {
		if m, ok := v.(map[string]any); ok {
			vals[k] = m
		}
	}
	if vals["shop-gateway"]["enabled"] != false || vals["shop-gateway"]["values"] != nil {
		t.Errorf("shop-gateway must be disabled without values: %v", vals["shop-gateway"])
	}
	if vals["shop-api"]["enabled"] == false || vals["shop-web"]["enabled"] == false {
		t.Errorf("apps with an image must stay enabled: api %v, web %v", vals["shop-api"], vals["shop-web"])
	}
	if indexOf(*steps, "wait shop-gateway-base-local") >= 0 || indexOf(*steps, "wait shop-web-base-local") < 0 {
		t.Errorf("waits must skip the left-out app only: %v", *steps)
	}
}

// Without the seed image there is no data: stop before phase 1, exit non-zero.
func TestInfraUp_SeedImageFailureStopsBeforePhase1(t *testing.T) {
	d, steps, _ := fakeInfra(t, false)
	failBuilds(&d, "demo-seed")
	d.apply = func(context.Context, infra.Runner, []byte) (bool, error) {
		t.Error("apply ran although the seed image failed")
		return false, nil
	}
	out, _, err := runInfra(d, "up", "--local")
	if exitcode.Code(err) == 0 || err == nil || !strings.Contains(err.Error(), "demo-seed") {
		t.Fatalf("err = %v, want a failure naming the seed image", err)
	}
	if indexOf(*steps, "build scan-tool") >= 0 {
		t.Errorf("builds went on after the seed image failed: %v", *steps)
	}
	if !hasRow(out, "demo-seed", "FAIL") {
		t.Errorf("table lacks the seed FAIL row:\n%s", out)
	}
	if indexOf(*steps, "save-state") >= 0 {
		t.Errorf("state saved after a seed failure: %v", *steps)
	}
}

// A rerun on an installed devbox reports k3s/argo unchanged and carries on
// through build, both env phases (seed before apps) and state.
func TestInfraUp_RerunUnchangedContinuesInOrder(t *testing.T) {
	d, steps, r := fakeInfra(t, true)
	out, _, err := runInfra(d, "up", "--local", "--ref", "shop-web=origin/feat/x")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"✓ 2/18 k3s   unchanged", "✓ 4/18 argocd   unchanged", "https://web.zenify.test", "admin@demo.example"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "tok-secret") {
		t.Error("the GitHub token was printed")
	}
	order := []string{"preflight", "k3s", "argocd", "repo-credential", "platform", "wait local-platform", "ca", "dns", "dns-restart",
		"buildctl", "build ui-base", "build shop-web", "env-ca base pem", "apply phase 1", "wait app-config-base-local",
		"apply phase 2", "wait shop-web-base-local", "save-state", "unlock"}
	last := -1
	for _, s := range order {
		i := indexOf(*steps, s)
		if i <= last {
			t.Fatalf("step %q out of order or missing: %v", s, *steps)
		}
		last = i
	}
	for _, c := range r.calls {
		if strings.HasPrefix(c, "kubectl") && !strings.Contains(c, "--kubeconfig "+infra.K3sKubeconfig) {
			t.Errorf("kubectl without the k3s kubeconfig: %s", c)
		}
	}
}

// Re-applying phase 1 on a deployed env would prune be/hub/web.
func TestInfraUp_DeployedEnvSkipsPhase1(t *testing.T) {
	d, steps, _ := fakeInfra(t, true)
	d.appExists = func(context.Context, infra.Runner, string) (bool, error) { return true, nil }
	if _, _, err := runInfra(d, "up", "--local"); err != nil {
		t.Fatal(err)
	}
	if indexOf(*steps, "apply phase 1") >= 0 || indexOf(*steps, "apply phase 2") < 0 {
		t.Errorf("steps = %v", *steps)
	}
}

// down without --apply is a preview: it must not touch the host or cluster.
func TestInfraDown_PreviewRunsNothing(t *testing.T) {
	d := defaultInfraDeps()
	r := &infraRunner{}
	d.runner, d.builder = r, r
	d.home = func() (string, error) { return t.TempDir(), nil }
	out, _, err := runInfra(d, "down", "--local", "--purge")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 0 {
		t.Errorf("preview ran commands: %v", r.calls)
	}
	if !strings.Contains(out, "PREVIEW") || !strings.Contains(out, "k3s-uninstall.sh") {
		t.Errorf("preview output: %s", out)
	}
}

// status --json is read by scripts and agents; it must be valid JSON with the
// documented keys.
func TestInfraStatus_JSONParses(t *testing.T) {
	d, _, _ := fakeInfra(t, true)
	d.status = func(_ context.Context, _ infra.Runner, w infra.Workspace, _, domain string) (infra.Report, error) {
		rep := infra.Report{K3s: infra.K3sVersion, ArgoCD: infra.ArgoCDVersion, Platform: infra.AppState{Sync: "Synced", Health: "Healthy"},
			Envs: []infra.EnvStatus{{Slug: "base", Sync: "Synced", Health: "Healthy", URLs: infra.EnvURLs(w, "base", domain)}}}
		rep.DNS.Resolves, rep.CA.Trusted = true, true
		return rep, nil
	}
	out, _, err := runInfra(d, "status", "--local", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	for _, k := range []string{"k3s", "argocd", "platform", "envs", "dns", "ca"} {
		if _, ok := got[k]; !ok {
			t.Errorf("missing key %s", k)
		}
	}
	if !strings.Contains(out, "https://web.zenify.test") {
		t.Errorf("env url not on the default domain zenify.test: %s", out)
	}
}

// Every subcommand refuses to run without --local (exit 2): no remote target exists yet.
func TestInfra_RequiresLocal(t *testing.T) {
	d, _, _ := fakeInfra(t, true)
	for _, sub := range []string{"up", "status", "show", "down", "config"} {
		_, _, err := runInfra(d, sub)
		if exitcode.Code(err) != exitcode.BadArgs || !strings.Contains(err.Error(), "only --local is supported") {
			t.Errorf("%s: err = %v", sub, err)
		}
	}
}

// maxEnvs 0 in a hand-edited config.json would allow no env: name the file.
func TestInfraUp_MaxEnvsZeroInConfigIsError(t *testing.T) {
	d, steps, _ := fakeInfra(t, true)
	home, _ := d.home()
	if err := os.MkdirAll(filepath.Join(home, ".zenify", "infra"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(infra.ConfigPath(home), []byte(`{"maxEnvs":0}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := runInfra(d, "up", "--local")
	if err == nil || !strings.Contains(err.Error(), infra.ConfigPath(home)) || !strings.Contains(err.Error(), "maxEnvs") {
		t.Fatalf("err = %v", err)
	}
	if len(*steps) != 0 {
		t.Errorf("steps ran: %v", *steps)
	}
}

// config rejects a domain outside .test and --max-envs below 1; a valid one is saved.
func TestInfraConfig_ValidatesAndSaves(t *testing.T) {
	d, _, _ := fakeInfra(t, true)
	for _, args := range [][]string{{"--domain", "devbox2.com"}, {"--domain", "Bad_Name.test"}, {"--max-envs", "0"}} {
		_, _, err := runInfra(d, append([]string{"config", "--local"}, args...)...)
		if exitcode.Code(err) != exitcode.BadArgs {
			t.Errorf("%v: err = %v", args, err)
		}
	}
	if _, _, err := runInfra(d, "config", "--local", "--domain", "dev1.test", "--max-envs", "2"); err != nil {
		t.Fatal(err)
	}
	home, _ := d.home()
	cfg, err := infra.LoadConfig(home)
	if err != nil || cfg.Domain != "dev1.test" || cfg.MaxEnvs != 2 {
		t.Fatalf("cfg = %+v err=%v", cfg, err)
	}
}

var deployed = infra.PlatformTarget{Domain: "devbox2.test", ChartRef: "feat/local"}

func withLive(d infraDeps) infraDeps {
	d.livePlatform = func(context.Context, infra.Runner) (infra.PlatformTarget, bool, error) { return deployed, true, nil }
	return d
}

// With no config and no state, up keeps the deployed platform's domain and
// chart ref instead of retargeting it to zenify.test at main.
func TestInfraUp_FollowsDeployedPlatform(t *testing.T) {
	d, _, _ := fakeInfra(t, true)
	d = withLive(d)
	var gotDomain, gotRef string
	d.applyPlatform = func(_ context.Context, _ infra.Runner, _, domain, ref string) error {
		gotDomain, gotRef = domain, ref
		return nil
	}
	out, _, err := runInfra(d, "up", "--local")
	if err != nil {
		t.Fatal(err)
	}
	if gotDomain != "devbox2.test" || gotRef != "feat/local" || !strings.Contains(out, "https://web.devbox2.test") {
		t.Fatalf("platform applied with %s@%s\n%s", gotDomain, gotRef, out)
	}
}

// A state domain that differs from the deployed platform is not a choice the
// user made now: stop before touching anything and say how to resolve it.
func TestInfraUp_DriftStopsBeforeAnyChange(t *testing.T) {
	d, steps, _ := fakeInfra(t, true)
	d = withLive(d)
	home, _ := d.home()
	if err := infra.SaveState(home, &infra.State{Domain: "old.test"}); err != nil {
		t.Fatal(err)
	}
	_, _, err := runInfra(d, "up", "--local")
	if exitcode.Code(err) == 0 || !strings.Contains(err.Error(), `"old.test"`) || !strings.Contains(err.Error(), `"devbox2.test"`) ||
		!strings.Contains(err.Error(), "zenify infra config --local --domain devbox2.test") ||
		!strings.Contains(err.Error(), "zenify infra up --local --chart-ref feat/local") ||
		strings.Contains(err.Error(), "config --local --domain devbox2.test (and pass --chart-ref") {
		t.Fatalf("err = %v", err)
	}
	// k3s recovery may run first (ruling: it must, to retry a failed restart);
	// nothing may reach Argo before the drift check
	if indexOf(*steps, "argocd") >= 0 || indexOf(*steps, "repo-credential") >= 0 || indexOf(*steps, "platform") >= 0 {
		t.Errorf("steps ran past the drift check: %v", *steps)
	}
}

// A domain set in config or a --chart-ref flag is a change on purpose.
func TestInfraUp_ExplicitValuesRetarget(t *testing.T) {
	d, _, _ := fakeInfra(t, true)
	d = withLive(d)
	home, _ := d.home()
	if err := infra.SaveConfig(home, infra.Config{Domain: "new.test", MaxEnvs: 3}); err != nil {
		t.Fatal(err)
	}
	var got string
	d.applyPlatform = func(_ context.Context, _ infra.Runner, _, domain, ref string) error {
		got = domain + "@" + ref
		return nil
	}
	if _, _, err := runInfra(d, "up", "--local", "--chart-ref", "main"); err != nil {
		t.Fatal(err)
	}
	if got != "new.test@main" {
		t.Fatalf("platform applied with %s", got)
	}
}

// status shows the deployed domain, not the github-login default.
func TestInfraStatus_UsesDeployedDomain(t *testing.T) {
	d, _, _ := fakeInfra(t, true)
	d = withLive(d)
	var got string
	d.status = func(_ context.Context, _ infra.Runner, _ infra.Workspace, _, domain string) (infra.Report, error) {
		got = domain
		return infra.Report{Envs: []infra.EnvStatus{}}, nil
	}
	if _, _, err := runInfra(d, "status", "--local", "--json"); err != nil {
		t.Fatal(err)
	}
	if got != "devbox2.test" {
		t.Fatalf("status domain = %s", got)
	}
}

// A typo in --ref would otherwise build every repo from its baseRef silently.
func TestInfraUp_UnknownRefRepoIsBadArgs(t *testing.T) {
	d, steps, _ := fakeInfra(t, true)
	_, _, err := runInfra(d, "up", "--local", "--ref", "shop-wbe=origin/x")
	if exitcode.Code(err) != exitcode.BadArgs || !strings.Contains(err.Error(), "shop-web") {
		t.Fatalf("err = %v", err)
	}
	if len(*steps) != 0 {
		t.Errorf("steps ran: %v", *steps)
	}
}

// A rerun keeps when the devbox was first set up.
func TestInfraUp_KeepsCreatedAt(t *testing.T) {
	d, _, _ := fakeInfra(t, true)
	home, _ := d.home()
	first := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	if err := infra.SaveState(home, &infra.State{Domain: "devbox2.test", CreatedAt: first}); err != nil {
		t.Fatal(err)
	}
	var saved *infra.State
	d.saveState = func(_ string, s *infra.State) error { saved = s; return nil }
	if _, _, err := runInfra(d, "up", "--local"); err != nil {
		t.Fatal(err)
	}
	if saved == nil || !saved.CreatedAt.Equal(first) {
		t.Fatalf("saved = %+v", saved)
	}
}

// down --apply removes envs before the platform (their finalizers need Argo)
// and, without --purge, leaves k3s, trust and state alone.
func TestInfraDown_ApplyEnvsThenPlatformOnly(t *testing.T) {
	d, steps, _ := fakeInfra(t, true)
	if _, _, err := runInfra(d, "down", "--local", "--apply"); err != nil {
		t.Fatal(err)
	}
	want := "lock,delete-app env-base,delete-ns env-base,delete-app local-platform,delete-ns zenify-system,unlock"
	if got := strings.Join(*steps, ","); got != want {
		t.Fatalf("steps = %s, want %s", got, want)
	}
}

// --purge order: envs, platform, trust/DNS, k3s (then its firewall), state.
func TestInfraDown_PurgeOrder(t *testing.T) {
	d, steps, _ := fakeInfra(t, true)
	if _, _, err := runInfra(d, "down", "--local", "--purge", "--apply"); err != nil {
		t.Fatal(err)
	}
	want := "lock,delete-app env-base,delete-ns env-base,delete-app local-platform,delete-ns zenify-system,remove-trust,uninstall-k3s,remove-state,unlock"
	if got := strings.Join(*steps, ","); got != want {
		t.Fatalf("steps = %s, want %s", got, want)
	}
}

// A failed step stops the rest and is named, so state is never removed while
// k3s is still installed.
func TestInfraDown_FailureStopsLaterSteps(t *testing.T) {
	d, steps, _ := fakeInfra(t, true)
	d.uninstallK3s = func(context.Context, infra.Runner) error { return errors.New("k3s-uninstall.sh: exit status 1") }
	_, _, err := runInfra(d, "down", "--local", "--purge", "--apply")
	if exitcode.Code(err) == 0 || !strings.HasPrefix(err.Error(), "k3s: ") {
		t.Fatalf("err = %v", err)
	}
	if indexOf(*steps, "remove-state") >= 0 {
		t.Errorf("state removed after a failed uninstall: %v", *steps)
	}
	d2, steps2, _ := fakeInfra(t, true)
	d2.deleteApp = func(context.Context, infra.Runner, string, bool) error { return errors.New("timed out") }
	_, _, err = runInfra(d2, "down", "--local", "--purge", "--apply")
	if err == nil || !strings.HasPrefix(err.Error(), "envs: ") || strings.Join(*steps2, ",") != "lock,unlock" {
		t.Fatalf("err = %v steps = %v", err, *steps2)
	}
}

// Nothing installed: say so instead of printing nothing.
func TestInfraDown_NothingInstalled(t *testing.T) {
	d, steps, _ := fakeInfra(t, true)
	d.k3sVersion = func(context.Context, infra.Runner) (string, error) { return "", nil }
	out, _, err := runInfra(d, "down", "--local", "--apply")
	if err != nil || !strings.Contains(out, "nothing to remove") || strings.Join(*steps, ",") != "lock,unlock" {
		t.Fatalf("err=%v out=%q steps=%v", err, out, *steps)
	}
}

// Ruling B: a k3s restart that failed in an earlier run leaves the API down;
// up must run the k3s recovery before it reads the deployed platform through
// that API, then resolve the target before anything reaches Argo.
func TestInfraUp_RecoversK3sBeforeReadingPlatform(t *testing.T) {
	d, steps, _ := fakeInfra(t, true)
	apiUp := false
	d.installK3s = func(context.Context, infra.Runner) (bool, error) {
		*steps = append(*steps, "k3s")
		apiUp = true // the restart-marker retry restarted k3s
		return true, nil
	}
	d.livePlatform = func(context.Context, infra.Runner) (infra.PlatformTarget, bool, error) {
		if !apiUp {
			return infra.PlatformTarget{}, false, errors.New("connection refused: k3s API down")
		}
		*steps = append(*steps, "read-platform")
		return deployed, true, nil
	}
	if _, _, err := runInfra(d, "up", "--local"); err != nil {
		t.Fatal(err)
	}
	k, p, a := indexOf(*steps, "k3s"), indexOf(*steps, "read-platform"), indexOf(*steps, "argocd")
	if k < 0 || p < k || a < p {
		t.Fatalf("order = %v, want k3s < read-platform < argocd", *steps)
	}
}

// A phase 2 that stopped half-way (some apps created, the probe app not)
// must not fall back to phase 1, which would prune the created apps.
func TestInfraUp_PartialPhase2SkipsPhase1(t *testing.T) {
	d, steps, _ := fakeInfra(t, true)
	d.appExists = func(_ context.Context, _ infra.Runner, name string) (bool, error) {
		return name == "shop-web-base-local", nil // the last phase-2 app only
	}
	if _, _, err := runInfra(d, "up", "--local"); err != nil {
		t.Fatal(err)
	}
	if indexOf(*steps, "apply phase 1") >= 0 || indexOf(*steps, "apply phase 2") < 0 {
		t.Fatalf("steps = %v", *steps)
	}
}

// The local CA reaches the env namespace before phase 1 (the env charts mount
// ConfigMap local-ca), with the PEM fetched from the platform.
func TestInfraUp_EnvCABeforePhase1(t *testing.T) {
	d, steps, _ := fakeInfra(t, true)
	if _, _, err := runInfra(d, "up", "--local"); err != nil {
		t.Fatal(err)
	}
	c, p1 := indexOf(*steps, "env-ca base pem"), indexOf(*steps, "apply phase 1")
	if c < 0 || p1 < c || indexOf(*steps, "fetch-ca") > c {
		t.Fatalf("steps = %v", *steps)
	}
	d2, steps2, _ := fakeInfra(t, true)
	d2.applyEnvCA = func(context.Context, infra.Runner, string, []byte) error { return errors.New("forbidden") }
	if _, _, err := runInfra(d2, "up", "--local"); err == nil || !strings.HasPrefix(err.Error(), "env-ca: ") || indexOf(*steps2, "apply phase 1") >= 0 {
		t.Fatalf("err = %v steps = %v", err, *steps2)
	}
}

// Workspace truth comes from infra.json only: without it, up/status/show stop
// naming the file before touching anything.
func TestInfra_MissingWorkspaceFileStops(t *testing.T) {
	for _, sub := range []string{"up", "status", "show"} {
		d, steps, _ := fakeInfra(t, true)
		d.spec = func(string) (infra.Workspace, error) {
			return infra.Workspace{}, errors.New("infra: /store/.config/infra.json not found: create it with helmRepoURL, builds, envApps and seed")
		}
		_, _, err := runInfra(d, sub, "--local")
		if exitcode.Code(err) == 0 || !strings.Contains(err.Error(), "infra.json not found") {
			t.Errorf("%s: err = %v", sub, err)
		}
		if len(*steps) != 0 {
			t.Errorf("%s: steps ran: %v", sub, *steps)
		}
	}
}

// Two up/down runs at once race on temp worktrees and root files: a held lock
// stops the second before any change.
func TestInfraUp_LockHeldStops(t *testing.T) {
	d, steps, _ := fakeInfra(t, true)
	d.lock = func(string) (func() error, error) {
		return nil, errors.New("another zenify infra up/down is running (lock /h/.zenify/infra/infra.lock)")
	}
	_, _, err := runInfra(d, "up", "--local")
	if exitcode.Code(err) == 0 || !strings.Contains(err.Error(), "infra.lock") || indexOf(*steps, "k3s") >= 0 {
		t.Fatalf("err = %v steps = %v", err, *steps)
	}
}

// show passes --slug through, prints no password in either format, and still
// shows domain, CA and accounts when no env exists yet.
func TestInfraShow_TextAndJSON(t *testing.T) {
	d, _, _ := fakeInfra(t, true)
	d = withLive(d)
	var gotDomain, gotSlug string
	d.show = func(_ context.Context, _ infra.Runner, w infra.Workspace, _, domain, slug string) (infra.ShowReport, error) {
		gotDomain, gotSlug = domain, slug
		return infra.ShowReport{Domain: domain, CA: "/h/.zenify/infra/ca.crt", Envs: []infra.ShowEnv{}, Accounts: w.Accounts(), LoginHelp: w.Seed.PasswordHint}, nil
	}
	out, _, err := runInfra(d, "show", "--local", "--slug", "base")
	if err != nil {
		t.Fatal(err)
	}
	if gotDomain != "devbox2.test" || gotSlug != "base" {
		t.Errorf("show called with %s/%s", gotDomain, gotSlug)
	}
	for _, want := range []string{"domain    devbox2.test", "/h/.zenify/infra/ca.crt", "envs      none", "admin@demo.example", "agent-01@demo.example", "tenant domain demotenant", "password: see the demo-seed README"} {
		if !strings.Contains(out, want) {
			t.Errorf("text missing %q:\n%s", want, out)
		}
	}
	js, _, err := runInfra(d, "show", "--local", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(js), &m); err != nil || strings.Contains(strings.ToLower(js), "password") { // fake has no ArgoCD row
		t.Fatalf("json err=%v or carries a password key:\n%s", err, js)
	}
	// cluster rows: the ArgoCD password is a command to run, OpenBao is left
	// out when no Service exists
	d.show = func(context.Context, infra.Runner, infra.Workspace, string, string, string) (infra.ShowReport, error) {
		return infra.ShowReport{Domain: "devbox2.test", Cluster: infra.ShowCluster{
			Kubeconfig: "/etc/rancher/k3s/k3s.yaml", Export: "export KUBECONFIG=/etc/rancher/k3s/k3s.yaml", KubeconfigNote: "mode 0640",
			Context: "default", Nodes: []infra.ShowNode{{Name: "devbox", Version: "v1"}},
			Registry: infra.ShowRegistry{Alias: "reg.alias:5000", Address: "10.0.0.1:5000"},
			ArgoCD:   &infra.ShowArgoCD{URL: "https://argocd-server.argocd.svc", PasswordCommand: "kubectl -n argocd get secret x"},
		}}, nil
	}
	out, _, err = runInfra(d, "show", "--local")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"export KUBECONFIG=/etc/rancher/k3s/k3s.yaml", "context     default", "node        devbox v1",
		"reg.alias:5000 (pods pull), 10.0.0.1:5000 (host)", "argocd      https://argocd-server.argocd.svc", "admin password: kubectl -n argocd get secret x"} {
		if !strings.Contains(out, want) {
			t.Errorf("cluster text missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "openbao") {
		t.Errorf("openbao row without a Service:\n%s", out)
	}
	d.show = func(context.Context, infra.Runner, infra.Workspace, string, string, string) (infra.ShowReport, error) {
		return infra.ShowReport{}, errors.New(`show: env "nope" not found (envs: base)`)
	}
	if _, _, err := runInfra(d, "show", "--local", "--slug", "nope"); exitcode.Code(err) == 0 || !strings.Contains(err.Error(), "envs: base") {
		t.Fatalf("err = %v", err)
	}
}

// An unreadable cwd must stop up with the cause, not look like "no workspace".
func TestInfraUp_WorkspaceErrorSurfaces(t *testing.T) {
	d, steps, _ := fakeInfra(t, true)
	d.workspace = func() (string, bool, error) { return "", false, errors.New("current directory: getwd: no such file") }
	_, _, err := runInfra(d, "up", "--local")
	if exitcode.Code(err) != exitcode.Fail || !strings.Contains(err.Error(), "workspace: current directory") {
		t.Fatalf("err = %v", err)
	}
	if len(*steps) != 0 {
		t.Errorf("steps ran: %v", *steps)
	}
}

// A rerun whose rebuild fails must not prune an app that already runs: it
// keeps its deployed tag, the run still ends non-zero (FR-13.6).
func TestInfraUp_FailedImageKeepsDeployedApp(t *testing.T) {
	d, steps, _ := fakeInfra(t, true)
	failBuilds(&d, "shop-gateway")
	d.deployedTags = func(context.Context, infra.Runner, infra.Workspace, string) (map[string]string, error) {
		return map[string]string{"shop-gateway": "sha-deployed", "shop-api": "sha-old-api"}, nil
	}
	var phase2 []byte
	d.apply = func(_ context.Context, _ infra.Runner, m []byte) (bool, error) {
		if bytes.Contains(m, []byte("sha-shop-api")) {
			phase2 = m
		}
		return true, nil
	}
	out, _, err := runInfra(d, "up", "--local")
	if exitcode.Code(err) == 0 || err == nil || !strings.Contains(err.Error(), "shop-gateway") {
		t.Fatalf("err = %v, want non-zero naming shop-gateway", err)
	}
	if !strings.Contains(out, "kept shop-gateway at sha-deployed: image shop-gateway failed") || strings.Contains(out, "skipped shop-gateway") {
		t.Errorf("want a kept line, no skipped line:\n%s", out)
	}
	if !strings.Contains(string(phase2), "sha-deployed") || strings.Contains(string(phase2), "sha-old-api") {
		t.Errorf("phase 2 must keep shop-gateway at sha-deployed and give shop-api its new tag:\n%s", phase2)
	}
	if strings.Contains(string(phase2), "shop-gateway:\n    name: shop-gateway-base-local\n    namespace: env-base\n    enabled: false") {
		t.Error("the deployed app was disabled")
	}
	if indexOf(*steps, "wait shop-gateway-base-local") < 0 || indexOf(*steps, "save-state") < 0 {
		t.Errorf("kept app must still be waited on and state saved: %v", *steps)
	}
}

// withFullOnly puts shop-gateway in profile full only.
func withFullOnly(t *testing.T, d *infraDeps) {
	t.Helper()
	d.spec = func(string) (infra.Workspace, error) {
		w := testWorkspace(t)
		for i := range w.EnvApps {
			if w.EnvApps[i].Key == "shop-gateway" {
				w.EnvApps[i].Profiles = []string{infra.ProfileFull}
			}
		}
		return w, nil
	}
}

// FR-13.5: any profile other than core or full is a bad argument (exit 2).
func TestInfraUp_BadProfileIsBadArgs(t *testing.T) {
	d, steps, _ := fakeInfra(t, true)
	_, _, err := runInfra(d, "up", "--local", "--profile", "bogus")
	if exitcode.Code(err) != exitcode.BadArgs || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("err = %v", err)
	}
	if len(*steps) != 0 {
		t.Errorf("steps ran: %v", *steps)
	}
}

// The profile decides which apps are rendered, waited for and saved to state;
// a later up without the flag reuses the saved one, and none saved means core.
func TestInfraUp_ProfileFiltersAppsAndIsSaved(t *testing.T) {
	run := func(saved string, args ...string) (rendered string, steps []string, st *infra.State) {
		d, sp, _ := fakeInfra(t, true)
		withFullOnly(t, &d)
		home, _ := d.home()
		if saved != "" {
			if err := infra.SaveState(home, &infra.State{Domain: "devbox2.test", Profile: saved}); err != nil {
				t.Fatal(err)
			}
		}
		var phase2 []byte
		d.apply = func(_ context.Context, _ infra.Runner, m []byte) (bool, error) {
			if bytes.Contains(m, []byte("tag: sha-shop-api")) {
				phase2 = m
			}
			return true, nil
		}
		d.saveState = func(_ string, s *infra.State) error { st = s; return nil }
		if _, _, err := runInfra(d, append([]string{"up", "--local"}, args...)...); err != nil {
			t.Fatal(err)
		}
		return string(phase2), *sp, st
	}
	cases := []struct {
		name, saved string
		args        []string
		wantProfile string
		wantGateway bool
	}{
		{"default is core", "", nil, "core", false},
		{"explicit full", "", []string{"--profile", "full"}, "full", true},
		{"saved full reused", "full", nil, "full", true},
		{"flag beats saved", "full", []string{"--profile", "core"}, "core", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, steps, st := run(c.saved, c.args...)
			if st == nil || st.Profile != c.wantProfile {
				t.Fatalf("saved state = %+v, want profile %s", st, c.wantProfile)
			}
			var app struct {
				Spec struct {
					Source struct {
						Helm struct {
							Values map[string]any `yaml:"valuesObject"`
						} `yaml:"helm"`
					} `yaml:"source"`
				} `yaml:"spec"`
			}
			if err := yaml.Unmarshal([]byte(m), &app); err != nil {
				t.Fatal(err)
			}
			gw, ok := app.Spec.Source.Helm.Values["shop-gateway"].(map[string]any)
			if !ok {
				t.Fatalf("shop-gateway missing from the values (the chart map would deploy it):\n%s", m)
			}
			if on := gw["enabled"] != false; on != c.wantGateway {
				t.Errorf("shop-gateway enabled = %v, want %v: %v", on, c.wantGateway, gw)
			}
			if got := indexOf(steps, "wait shop-gateway-base-local") >= 0; got != c.wantGateway {
				t.Errorf("waits for shop-gateway = %v, want %v: %v", got, c.wantGateway, steps)
			}
		})
	}
}

// status and show list only the saved profile's apps (no state = core).
func TestInfraStatusShow_UseSavedProfile(t *testing.T) {
	for _, c := range []struct {
		saved string
		want  bool
	}{{"", false}, {"core", false}, {"full", true}} {
		d, _, _ := fakeInfra(t, true)
		withFullOnly(t, &d)
		home, _ := d.home()
		if c.saved != "" {
			if err := infra.SaveState(home, &infra.State{Profile: c.saved}); err != nil {
				t.Fatal(err)
			}
		}
		var gotS, gotW infra.Workspace
		d.status = func(_ context.Context, _ infra.Runner, w infra.Workspace, _, _ string) (infra.Report, error) {
			gotS = w
			return infra.Report{}, nil
		}
		d.show = func(_ context.Context, _ infra.Runner, w infra.Workspace, _, _, _ string) (infra.ShowReport, error) {
			gotW = w
			return infra.ShowReport{}, nil
		}
		if _, _, err := runInfra(d, "status", "--local"); err != nil {
			t.Fatal(err)
		}
		if _, _, err := runInfra(d, "show", "--local"); err != nil {
			t.Fatal(err)
		}
		for name, w := range map[string]infra.Workspace{"status": gotS, "show": gotW} {
			has := false
			for _, a := range w.EnvApps {
				has = has || a.Key == "shop-gateway"
			}
			if has != c.want {
				t.Errorf("saved %q: %s sees shop-gateway = %v, want %v", c.saved, name, has, c.want)
			}
		}
	}
}

// An active app whose repo is neither a build nor scanned stops up before any
// image is built; the same app outside the active profile is not checked.
func TestInfraUp_AppRepoNeedsBuildOrScan(t *testing.T) {
	orphan := func(t *testing.T, d *infraDeps, profiles []string) {
		t.Helper()
		d.spec = func(string) (infra.Workspace, error) {
			w := testWorkspace(t)
			w.EnvApps = append(w.EnvApps, infra.WorkspaceApp{Key: "orphan", Phase: 2, Repo: "no-such-repo", Svc: "orph", Profiles: profiles})
			return w, nil
		}
	}
	d, steps, _ := fakeInfra(t, true)
	orphan(t, &d, nil)
	_, _, err := runInfra(d, "up", "--local")
	want := "envApps[orphan].repo no-such-repo: no Dockerfile at its base ref and not in builds[]"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("err = %v, want %q", err, want)
	}
	for _, s := range *steps {
		if strings.HasPrefix(s, "build ") {
			t.Errorf("an image was built before the check: %v", *steps)
		}
	}
	d, _, _ = fakeInfra(t, true)
	orphan(t, &d, []string{infra.ProfileFull})
	if _, _, err := runInfra(d, "up", "--local", "--profile", "core"); err != nil {
		t.Fatalf("out-of-profile app must not be checked: %v", err)
	}
}

// The reason on a failed build line is the real cause (the npm 401), not
// buildkit's last line that restates the whole RUN command; capped at 200.
func TestBuildReason_PrefersTheRealError(t *testing.T) {
	log := "buildctl: exit status 1: #9 RUN npm ci\n" +
		"#9 0.9 npm error code E401\n" +
		"#9 0.9 npm error 401 Unauthorized - GET https://npm.pkg.github.com/@acme%2fcore\n" +
		"#9 ERROR: process \"/bin/sh -c export NPM_TOKEN=[redacted] && npm ci\" did not complete successfully\n" +
		"error: failed to solve: process \"/bin/sh -c export NPM_TOKEN=[redacted] && npm ci\" did not complete successfully"
	want := "#9 ERROR: process \"/bin/sh -c export NPM_TOKEN=[redacted] && npm ci\" did not complete successfully"
	if got := buildReason(errors.New(log)); got != want {
		t.Errorf("reason = %q, want the last ERROR: line", got)
	}
	noErr := strings.Join(strings.Split(log, "\n")[:3], "\n")
	if got := buildReason(errors.New(noErr)); !strings.Contains(got, "npm error 401 Unauthorized") {
		t.Errorf("reason = %q, want the 401 line", got)
	}
	if got := buildReason(errors.New("a\nplain last line")); got != "plain last line" {
		t.Errorf("reason = %q, want the last line when nothing matches", got)
	}
	long := "npm error " + strings.Repeat("x", 400)
	if got := buildReason(errors.New(long)); len([]rune(got)) != 200 || !strings.HasSuffix(got, "...") {
		t.Errorf("reason len %d, want 200 ending in ...", len([]rune(got)))
	}
}
