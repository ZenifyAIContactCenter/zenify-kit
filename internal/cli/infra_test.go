package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/infra"
)

// infraRunner records commands; gh answers with a fixed login/token.
type infraRunner struct{ calls []string }

func (r *infraRunner) Run(_ context.Context, _ io.Reader, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, strings.Join(append([]string{name}, args...), " "))
	if name == "gh" && len(args) > 0 && args[0] == "auth" {
		return []byte("tok-secret\n"), nil
	}
	if name == "gh" {
		return []byte("QuyenTM\n"), nil
	}
	return nil, nil
}

func (r *infraRunner) RunEnv(ctx context.Context, _ []string, name string, args ...string) ([]byte, error) {
	return r.Run(ctx, nil, name, args...)
}

func (r *infraRunner) Start(context.Context, string, ...string) (func() error, error) {
	return func() error { return nil }, nil
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
		workspace: func() (string, bool) { return "/ws", true },
		runner:    r,
		builder:   r,
		preflight: func(string) []infra.Problem { rec("preflight"); return nil },
		installK3s: func(context.Context, infra.Runner) (bool, error) {
			rec("k3s")
			return !alreadyThere, nil
		},
		installArgo: func(context.Context, infra.Runner) (bool, error) {
			rec("argocd")
			return !alreadyThere, nil
		},
		setRepoCred: func(_ context.Context, _ infra.Runner, tok string) error {
			if tok != "tok-secret" {
				t.Errorf("token = %q", tok)
			}
			rec("repo-credential")
			return nil
		},
		applyPlatform: func(context.Context, infra.Runner, string, string) error { rec("platform"); return nil },
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
		coreTargets: func(_, _, _ string) ([]infra.BuildTarget, error) {
			return []infra.BuildTarget{{Repo: "zenify-ci"}, {Repo: "contact-center-be"}, {Repo: "contact-center-hub"},
				{Repo: "contact-center-web"}, {Repo: infra.SeedRepo}}, nil
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
		appExists: func(context.Context, infra.Runner, string) (bool, error) { return false, nil },
		saveState: func(string, *infra.State) error { rec("save-state"); return nil },
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
		return []infra.Problem{{Check: "build-secrets", Detail: "NPM_TASKFORCESH_TOKEN is not set", Fix: "zenify env set NPM_TASKFORCESH_TOKEN"}}
	}
	_, errOut, err := runInfra(d, "up", "--local")
	if exitcode.Code(err) == 0 {
		t.Fatal("want non-zero exit")
	}
	if !strings.Contains(errOut, "build-secrets") || !strings.Contains(errOut, "NPM_TASKFORCESH_TOKEN") {
		t.Errorf("problem not printed: %q", errOut)
	}
	if len(*steps) != 0 {
		t.Errorf("steps ran after a failed preflight: %v", *steps)
	}
}

// A build failure names the step and never records state: a later run must
// not believe the devbox is complete.
func TestInfraUp_BuildFailureNamesStepAndSkipsState(t *testing.T) {
	d, steps, _ := fakeInfra(t, false)
	d.build = func(context.Context, infra.BuildRunner, string, infra.BuildTarget, map[string]string) (infra.BuildResult, error) {
		return infra.BuildResult{}, errors.New("buildctl: exit status 1")
	}
	_, _, err := runInfra(d, "up", "--local")
	if exitcode.Code(err) == 0 || !strings.HasPrefix(err.Error(), "build: ") {
		t.Fatalf("err = %v, want a build step error", err)
	}
	if indexOf(*steps, "save-state") >= 0 || indexOf(*steps, "apply phase 1") >= 0 {
		t.Errorf("ran past the failed build: %v", *steps)
	}
}

// A rerun on an installed devbox reports k3s/argo unchanged and carries on
// through build, both env phases (seed before apps) and state.
func TestInfraUp_RerunUnchangedContinuesInOrder(t *testing.T) {
	d, steps, r := fakeInfra(t, true)
	out, _, err := runInfra(d, "up", "--local", "--ref", "contact-center-web=origin/feat/x")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"k3s            unchanged", "argocd         unchanged", "https://web-base.quyentm.test", "admin@zenifytest.vn"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "tok-secret") {
		t.Error("the GitHub token was printed")
	}
	order := []string{"preflight", "k3s", "argocd", "repo-credential", "platform", "wait local-platform", "ca", "dns", "dns-restart",
		"buildctl", "build zenify-ci", "build contact-center-web", "apply phase 1", "wait local-env-base-local",
		"apply phase 2", "wait contact-center-web-base-local", "save-state"}
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
	d.status = func(_ context.Context, _ infra.Runner, _, domain string) (infra.Report, error) {
		rep := infra.Report{K3s: infra.K3sVersion, ArgoCD: infra.ArgoCDVersion, Platform: infra.AppState{Sync: "Synced", Health: "Healthy"},
			Envs: []infra.EnvStatus{{Slug: "base", Sync: "Synced", Health: "Healthy", URLs: infra.EnvURLs("base", domain)}}}
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
	if !strings.Contains(out, "https://web-base.quyentm.test") {
		t.Errorf("env url not from the gh login domain: %s", out)
	}
}

// Every subcommand refuses to run without --local (exit 2): no remote target exists yet.
func TestInfra_RequiresLocal(t *testing.T) {
	d, _, _ := fakeInfra(t, true)
	for _, sub := range []string{"up", "status", "down", "config"} {
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
	for _, args := range [][]string{{"--domain", "quyentm.com"}, {"--domain", "Bad_Name.test"}, {"--max-envs", "0"}} {
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
