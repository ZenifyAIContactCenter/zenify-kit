package infra

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImageTag(t *testing.T) {
	if got := ImageTag("abcdef1234", nil); got != "sha-abcdef1" {
		t.Fatalf("clean tag = %q", got)
	}
	a1, a2 := ImageTag("abcdef1234", []byte("diff a")), ImageTag("abcdef1234", []byte("diff a"))
	b := ImageTag("abcdef1234", []byte("diff b"))
	if a1 != a2 {
		t.Fatalf("same diff gave %q and %q: a rebuild of unchanged code would not be skipped", a1, a2)
	}
	if a1 == b {
		t.Fatalf("different diffs gave the same tag %q: a changed worktree would reuse a stale image", a1)
	}
	if !strings.HasPrefix(a1, "sha-abcdef1-dirty-") || len(a1) != len("sha-abcdef1-dirty-")+8 {
		t.Fatalf("dirty tag shape = %q", a1)
	}
}

// fakeWorkspace lays out <ws>/repos/<repo>/.claude/worktree.json for every core repo.
func fakeWorkspace(t *testing.T) string {
	t.Helper()
	ws := t.TempDir()
	for _, repo := range []string{"zenify-ci", "contact-center-be", "contact-center-hub", "contact-center-web", "zenify-data-seed"} {
		dir := filepath.Join(ws, "repos", repo, ".claude")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "worktree.json"), []byte(`{"baseRef":"origin/staging"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return ws
}

func TestCoreTargets(t *testing.T) {
	ts, err := CoreTargets(fakeWorkspace(t), "quyentm.test", "base")
	if err != nil {
		t.Fatal(err)
	}
	idx := map[string]int{}
	for i, x := range ts {
		idx[x.Repo] = i
		if strings.Contains(x.Image, "repo.zenify.vn") {
			t.Errorf("%s image %q pulls from the login-only registry", x.Repo, x.Image)
		}
		for k, v := range x.Args {
			if strings.Contains(v, "repo.zenify.vn") {
				t.Errorf("%s arg %s=%q points at the login-only registry", x.Repo, k, v)
			}
		}
	}
	if idx["zenify-ci"] > idx["contact-center-web"] {
		t.Fatal("base-spa must be built before web: web's runtime stage is FROM base-spa")
	}
	web := ts[idx["contact-center-web"]]
	want := map[string]string{"BUILD_MODE": "preview", "PREVIEW_SLUG": "base", "PREVIEW_DOMAIN": "quyentm.test", "REGISTRY": RegistryHost}
	for k, v := range want {
		if web.Args[k] != v {
			t.Errorf("web arg %s = %q, want %q (bundle would call the wrong hosts)", k, web.Args[k], v)
		}
	}
	spa := ts[idx["zenify-ci"]]
	if spa.Image != RegistryHost+"/zenify/base-spa" || spa.Tag != "1.30" {
		t.Fatalf("base-spa ref = %s:%s, web's FROM would not find it", spa.Image, spa.Tag)
	}
}

func TestCoreTargetsRejectsMissingInputs(t *testing.T) {
	ws := fakeWorkspace(t)
	if _, err := CoreTargets(ws, "", "base"); err == nil {
		t.Fatal("empty domain accepted: web would bake preview.zenify.vn hosts")
	}
	if _, err := CoreTargets(ws, "quyentm.test", ""); err == nil {
		t.Fatal("empty slug accepted: the web preview build fails late")
	}
	if err := os.RemoveAll(filepath.Join(ws, "repos", "zenify-data-seed")); err != nil {
		t.Fatal(err)
	}
	if _, err := CoreTargets(ws, "quyentm.test", "base"); err == nil || !strings.Contains(err.Error(), "zenify-data-seed") {
		t.Fatalf("missing repo error = %v, want it named", err)
	}
}

// fakeBuildRunner is a fakeRunner that also records RunEnv/Start.
type fakeBuildRunner struct {
	fakeRunner
	envs    [][]string // child env of each RunEnv call, aligned with calls
	started []call
	stopped int
}

func (f *fakeBuildRunner) RunEnv(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	f.envs = append(f.envs, env)
	return f.Run(ctx, nil, name, args...)
}

func (f *fakeBuildRunner) Start(_ context.Context, name string, args ...string) (func() error, error) {
	f.started = append(f.started, call{name: name, args: args})
	return func() error { f.stopped++; return nil }, nil
}

// buildEnv points the registry check at srv, fakes the port wait and plants an installed buildctl.
func buildEnv(t *testing.T, status int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
	t.Cleanup(srv.Close)
	oldURL, oldWait := registryURL, waitPort
	registryURL, waitPort = srv.URL, func(context.Context, string) error { return nil }
	t.Cleanup(func() { registryURL, waitPort = oldURL, oldWait })
	home := t.TempDir()
	t.Setenv("HOME", home)
	bin := filepath.Join(infraDir(home), "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "buildctl"), []byte("#!/bin/true\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func beTarget(t *testing.T) BuildTarget {
	t.Helper()
	ts, err := CoreTargets(fakeWorkspace(t), "quyentm.test", "base")
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range ts {
		if x.Repo == "contact-center-be" {
			x.Dir = t.TempDir()
			return x
		}
	}
	t.Fatal("no be target")
	return BuildTarget{}
}

func TestBuildKeepsTokenOffArgv(t *testing.T) {
	buildEnv(t, http.StatusNotFound)
	t.Setenv("NPM_TOKEN", "secret-xyz")
	r := &fakeBuildRunner{}
	built, err := Build(context.Background(), r, t.TempDir(), beTarget(t), "sha-abcdef1")
	if err != nil || !built {
		t.Fatalf("built=%v err=%v", built, err)
	}
	var bc *call
	for i, c := range r.calls {
		if strings.Contains(strings.Join(c.args, " "), "secret-xyz") || strings.Contains(c.name, "secret-xyz") {
			t.Fatalf("token leaked on argv: %s", c.line())
		}
		if filepath.Base(c.name) == "buildctl" {
			bc = &r.calls[i]
			if !strings.Contains(strings.Join(r.envs[0], "\n"), "NPM_TOKEN=secret-xyz") {
				t.Fatal("token not handed to buildctl through its env: npm install in the image would fail")
			}
		}
	}
	if bc == nil {
		t.Fatal("buildctl not called")
	}
	line := bc.line()
	for _, want := range []string{"--secret id=npm_token,env=NPM_TOKEN", "--opt target=api", "name=" + RegistryHost + "/contact-center-be/api:sha-abcdef1,push=true"} {
		if !strings.Contains(line, want) {
			t.Errorf("buildctl argv missing %q: %s", want, line)
		}
	}
	if len(r.started) != 1 || !strings.Contains(r.started[0].line(), "--kubeconfig "+K3sKubeconfig) || !strings.Contains(r.started[0].line(), "port-forward -n "+PlatformNamespace+" svc/buildkitd") {
		t.Fatalf("port-forward not through the local kubeconfig: %v", r.started)
	}
	if r.stopped != 1 {
		t.Fatal("port-forward left running")
	}
}

func TestBuildSkipsExistingTag(t *testing.T) {
	buildEnv(t, http.StatusOK)
	r := &fakeBuildRunner{}
	built, err := Build(context.Background(), r, t.TempDir(), beTarget(t), "sha-abcdef1")
	if err != nil || built {
		t.Fatalf("built=%v err=%v, want skipped", built, err)
	}
	if len(r.calls) != 0 || len(r.started) != 0 {
		t.Fatalf("existing tag still ran %v / %v", r.calls, r.started)
	}
}

func TestBuildHubNeedsTaskforceToken(t *testing.T) {
	buildEnv(t, http.StatusNotFound)
	t.Setenv("NPM_TOKEN", "x")
	t.Setenv("NPM_TASKFORCESH_TOKEN", "")
	ws := fakeWorkspace(t)
	ts, err := CoreTargets(ws, "quyentm.test", "base")
	if err != nil {
		t.Fatal(err)
	}
	var hub BuildTarget
	for _, x := range ts {
		if x.Repo == "contact-center-hub" {
			hub = x
		}
	}
	r := &fakeBuildRunner{}
	_, err = Build(context.Background(), r, ws, hub, "sha-abcdef1")
	if err == nil || !strings.Contains(err.Error(), "NPM_TASKFORCESH_TOKEN") {
		t.Fatalf("err = %v, want it to name NPM_TASKFORCESH_TOKEN", err)
	}
	if len(r.started) != 0 {
		t.Fatal("port-forward started although the build cannot run")
	}
}

func TestBuildErrorRedactsToken(t *testing.T) {
	buildEnv(t, http.StatusNotFound)
	t.Setenv("NPM_TOKEN", "secret-xyz")
	r := &fakeBuildRunner{}
	r.respond = func(c call) ([]byte, error) {
		if filepath.Base(c.name) == "buildctl" {
			return nil, errors.New("npm ERR! 401 token secret-xyz rejected")
		}
		return nil, nil
	}
	_, err := Build(context.Background(), r, t.TempDir(), beTarget(t), "sha-abcdef1")
	if err == nil || strings.Contains(err.Error(), "secret-xyz") {
		t.Fatalf("err = %v, want a failure without the token", err)
	}
	if r.stopped != 1 {
		t.Fatal("port-forward left running after a failed build")
	}
}

// gitFake answers the git calls of BaseSource; buildctl fails when failBuild.
func gitFake(failBuild bool) func(call) ([]byte, error) {
	return func(c call) ([]byte, error) {
		switch {
		case c.name == "git" && len(c.args) > 2 && c.args[2] == "rev-parse":
			return []byte("0123456789abcdef0123456789abcdef01234567\n"), nil
		case filepath.Base(c.name) == "buildctl" && failBuild:
			return nil, errors.New("build failed")
		}
		return nil, nil
	}
}

func TestBuildFromBaseRemovesTempWorktreeOnError(t *testing.T) {
	buildEnv(t, http.StatusNotFound)
	ws := fakeWorkspace(t)
	ts, err := CoreTargets(ws, "quyentm.test", "base")
	if err != nil {
		t.Fatal(err)
	}
	seed := ts[len(ts)-1]
	r := &fakeBuildRunner{fakeRunner: fakeRunner{respond: gitFake(true)}}
	_, err = BuildFromBase(context.Background(), r, ws, seed, map[string]string{"zenify-data-seed": "origin/feature"})
	if err == nil {
		t.Fatal("build error swallowed")
	}
	home, _ := os.UserHomeDir()
	tmp := filepath.Join(infraDir(home), "src", "zenify-data-seed-0123456")
	repo := filepath.Join(ws, "repos", "zenify-data-seed")
	if !r.ran("git -C " + repo + " worktree add --detach " + tmp + " 0123456789abcdef0123456789abcdef01234567") {
		t.Fatalf("temp worktree not created at %s: %v", tmp, r.calls)
	}
	if !r.ran("git -C " + repo + " rev-parse --verify origin/feature^{commit}") {
		t.Fatal("ref override ignored")
	}
	if !r.ran("git -C " + repo + " worktree remove --force " + tmp) {
		t.Fatal("temp worktree left behind after a failed build")
	}
	for _, c := range r.calls {
		if c.name == "git" && strings.Contains(c.line(), "worktree remove") && !strings.Contains(c.line(), tmp) {
			t.Fatalf("removed a worktree that is not the temp one: %s", c.line())
		}
	}
}

func TestBuildFromBaseUsesWorktreeJSONBaseRef(t *testing.T) {
	buildEnv(t, http.StatusOK)
	ws := fakeWorkspace(t)
	ts, err := CoreTargets(ws, "quyentm.test", "base")
	if err != nil {
		t.Fatal(err)
	}
	r := &fakeBuildRunner{fakeRunner: fakeRunner{respond: gitFake(false)}}
	res, err := BuildFromBase(context.Background(), r, ws, ts[len(ts)-1], nil)
	if err != nil || res.Built {
		t.Fatalf("res=%+v err=%v, want skipped", res, err)
	}
	repo := filepath.Join(ws, "repos", "zenify-data-seed")
	if !r.ran("git -C "+repo+" fetch origin") || !r.ran("git -C "+repo+" rev-parse --verify origin/staging^{commit}") {
		t.Fatalf("baseRef from worktree.json not used: %v", r.calls)
	}
	if res.Tag != "sha-0123456" {
		t.Fatalf("tag = %q", res.Tag)
	}
}

func webTarget(t *testing.T, ws, domain, slug string) BuildTarget {
	t.Helper()
	ts, err := CoreTargets(ws, domain, slug)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range ts {
		if x.Repo == "contact-center-web" {
			return x
		}
	}
	t.Fatal("no web target")
	return BuildTarget{}
}

func TestVariantTagSeparatesEnvs(t *testing.T) {
	ws := fakeWorkspace(t)
	base := ImageTag("abcdef1234", nil)
	a1 := VariantTag(base, webTarget(t, ws, "quyentm.test", "base"))
	a2 := VariantTag(base, webTarget(t, ws, "quyentm.test", "base"))
	other := VariantTag(base, webTarget(t, ws, "quyentm.test", "feat-x"))
	otherDomain := VariantTag(base, webTarget(t, ws, "other.test", "base"))
	if a1 != a2 {
		t.Fatalf("same env gave %q and %q: an unchanged web image would be rebuilt", a1, a2)
	}
	if a1 == other || a1 == otherDomain {
		t.Fatalf("another slug/domain reused tag %q: that env would serve a bundle calling the wrong hosts", a1)
	}
	if !strings.HasPrefix(a1, "sha-abcdef1-v") || len(a1) != len("sha-abcdef1-v")+8 {
		t.Fatalf("variant tag shape = %q", a1)
	}
	if got := VariantTag(base, beTarget(t)); got != base {
		t.Fatalf("be has no per-env args, tag = %q, want %q", got, base)
	}
}

func TestBaseSourceRejectsOptionLikeRef(t *testing.T) {
	r := &fakeRunner{}
	if _, _, _, err := BaseSource(context.Background(), r, fakeWorkspace(t), "zenify-data-seed", "--upload-pack=x"); err == nil {
		t.Fatal("a ref starting with '-' reached git as an option")
	}
	if len(r.calls) != 0 {
		t.Fatalf("git ran with an option-like ref: %v", r.calls)
	}
}
