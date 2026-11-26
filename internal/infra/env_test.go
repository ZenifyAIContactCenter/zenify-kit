package infra

import (
	"context"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// testWS is the neutral fixture workspace (testdata/infra.json).
func testWS(t *testing.T) Workspace {
	t.Helper()
	w, err := LoadWorkspace("testdata/infra.json")
	if err != nil {
		t.Fatal(err)
	}
	return w
}

var testImages = map[string]string{
	"shop-api": "sha-api0001", "shop-gateway": "sha-gw00001",
	"shop-web": "sha-web0001-v12345678", "demo-seed": "sha-seed001",
}

type renderedApp struct {
	Metadata struct {
		Name       string   `yaml:"name"`
		Namespace  string   `yaml:"namespace"`
		Finalizers []string `yaml:"finalizers"`
	} `yaml:"metadata"`
	Spec struct {
		Source struct {
			RepoURL        string `yaml:"repoURL"`
			Path           string `yaml:"path"`
			TargetRevision string `yaml:"targetRevision"`
			Helm           struct {
				ValueFiles   []string       `yaml:"valueFiles"`
				ValuesObject map[string]any `yaml:"valuesObject"`
			} `yaml:"helm"`
		} `yaml:"source"`
	} `yaml:"spec"`
}

func render(t *testing.T, phase int) renderedApp {
	t.Helper()
	b, err := RenderEnvApp(testWS(t), "base", "zenify.test", "main", testImages, phase, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var a renderedApp
	if err := yaml.Unmarshal(b, &a); err != nil {
		t.Fatalf("rendered app is not YAML: %v\n%s", err, b)
	}
	return a
}

func dig(m any, keys ...string) any {
	for _, k := range keys {
		mm, ok := m.(map[string]any)
		if !ok {
			return nil
		}
		m = mm[k]
	}
	return m
}

func enabledApps(t *testing.T, vals map[string]any) []string {
	var on []string
	for _, a := range testWS(t).EnvApps {
		if dig(vals, a.Key, "enabled") != false {
			on = append(on, a.Key)
		}
	}
	return on
}

// Phase 1 must not start the apps: the seed runs --if-empty and refuses once
// an app has created its collections.
func TestRenderEnvApp_Phase1EnablesOnlyDatastoresAndSeed(t *testing.T) {
	a := render(t, 1)
	got := strings.Join(enabledApps(t, a.Spec.Source.Helm.ValuesObject), ",")
	if got != "mongo,mysql,redis,app-config" {
		t.Fatalf("phase 1 enabled = %s", got)
	}
	if tag := dig(a.Spec.Source.Helm.ValuesObject, "app-config", "values", "seed", "tag"); tag != "sha-seed001" {
		t.Fatalf("seed.tag = %v", tag)
	}
}

// Phase 2 deploys every child app of env base with the hosts and values keys
// infra.json names; a wrong name or host breaks the URLs the kit prints.
func TestRenderEnvApp_Phase2AppsNamesAndHosts(t *testing.T) {
	a := render(t, 2)
	vals := a.Spec.Source.Helm.ValuesObject
	if a.Metadata.Name != "env-base" || a.Spec.Source.Path != "charts/app-of-apps" || a.Spec.Source.TargetRevision != "main" ||
		a.Spec.Source.RepoURL != "https://github.com/example-team/charts.git" {
		t.Fatalf("root app = %s path %s rev %s repo %s", a.Metadata.Name, a.Spec.Source.Path, a.Spec.Source.TargetRevision, a.Spec.Source.RepoURL)
	}
	if len(a.Metadata.Finalizers) != 1 || a.Metadata.Finalizers[0] != "resources-finalizer.argocd.argoproj.io" {
		t.Fatalf("finalizers = %v", a.Metadata.Finalizers)
	}
	if vals["env"] != "local" || dig(vals, "global", "targetRevision") != "main" {
		t.Fatalf("env/global = %v/%v", vals["env"], vals["global"])
	}
	on := enabledApps(t, vals)
	if len(on) != 7 {
		t.Fatalf("phase 2 enabled %d apps: %v", len(on), on)
	}
	for _, app := range on {
		if dig(vals, app, "name") != app+"-base" || dig(vals, app, "namespace") != "env-base" {
			t.Errorf("%s name/namespace = %v/%v", app, dig(vals, app, "name"), dig(vals, app, "namespace"))
		}
	}
	host := func(app string, path ...string) any {
		hs, _ := dig(vals, append([]string{app, "values", "apps"}, path...)...).([]any)
		if len(hs) != 1 {
			return nil
		}
		return hs[0]
	}
	if h := dig(host("shop-web", "shop-web", "ingress", "hosts"), "host"); h != "web.zenify.test" {
		t.Errorf("web host = %v", h)
	}
	if h := dig(host("shop-api", "api", "ingress", "hosts"), "host"); h != "be.zenify.test" {
		t.Errorf("be host = %v", h)
	}
	if h := dig(host("shop-gateway", "hub", "ingress", "hosts"), "host"); h != "hub.zenify.test" {
		t.Errorf("hub host = %v", h)
	}
	auth := host("shop-gateway", "auth", "ingress", "hosts")
	paths, _ := dig(auth, "paths").([]any)
	if dig(auth, "host") != "hub.zenify.test" || len(paths) != 1 || dig(paths[0], "path") != "/auth" {
		t.Errorf("auth host = %v", auth)
	}
	if tag := dig(vals, "shop-web", "values", "image", "tag"); tag != "sha-web0001-v12345678" {
		t.Errorf("web image.tag = %v", tag)
	}
	if dig(vals, "mongo", "values") != nil {
		t.Error("a datastore must take no kit values")
	}
}

// A missing image tag would deploy an app with no image: refuse to render.
func TestRenderEnvApp_RejectsMissingTagAndBadInput(t *testing.T) {
	w := testWS(t)
	if _, err := RenderEnvApp(w, "base", "zenify.test", "main", map[string]string{"demo-seed": "x"}, 2, nil, nil); err == nil {
		t.Fatal("phase 2 without app tags must fail")
	}
	if _, err := RenderEnvApp(w, "Bad_Slug", "zenify.test", "main", testImages, 2, nil, nil); err == nil {
		t.Fatal("invalid slug must fail")
	}
	if _, err := RenderEnvApp(w, "base", strings.Repeat("a", 64)+".test", "main", testImages, 2, nil, nil); err == nil {
		t.Fatal("a 64-byte label must fail")
	}
}

// Contract with the env charts: before phase 1, namespace env-<slug> holds
// ConfigMap local-ca key ca.crt with the local CA, applied server-side through
// the k3s kubeconfig so pods can trust the env's HTTPS hosts.
func TestApplyEnvCA(t *testing.T) {
	raw := []byte(testPEM)
	r := &fakeRunner{}
	if err := ApplyEnvCA(context.Background(), r, "base", raw); err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 1 {
		t.Fatalf("calls = %v", r.calls)
	}
	c := r.calls[0]
	if !strings.HasPrefix(c.line(), "kubectl --kubeconfig "+K3sKubeconfig+" apply --server-side --field-manager=zenify -f -") {
		t.Fatalf("apply = %s", c.line())
	}
	dec := yaml.NewDecoder(strings.NewReader(c.stdin))
	var ns, cm map[string]any
	if err := dec.Decode(&ns); err != nil {
		t.Fatal(err)
	}
	if err := dec.Decode(&cm); err != nil {
		t.Fatal(err)
	}
	if ns["kind"] != "Namespace" || dig(ns, "metadata", "name") != "env-base" {
		t.Fatalf("namespace doc = %v", ns)
	}
	if cm["kind"] != "ConfigMap" || dig(cm, "metadata", "name") != "local-ca" || dig(cm, "metadata", "namespace") != "env-base" {
		t.Fatalf("configmap doc = %v", cm)
	}
	crt, _ := dig(cm, "data", "ca.crt").(string)
	if !samePEM([]byte(crt), raw) {
		t.Fatalf("ca.crt = %q", crt)
	}
	if err := ApplyEnvCA(context.Background(), &fakeRunner{}, "base", []byte("not a cert")); err == nil {
		t.Fatal("a non-CA PEM must be refused")
	}
}

// apply reports unchanged only when kubectl left every object as it was, so
// `up` prints the truth on a rerun.
func TestApply_ChangedFromKubectlOutput(t *testing.T) {
	for out, want := range map[string]bool{
		"application.argoproj.io/env-base unchanged\n":  false,
		"application.argoproj.io/env-base configured\n": true,
		"application.argoproj.io/env-base created\n":    true,
	} {
		r := &fakeRunner{respond: func(call) ([]byte, error) { return []byte(out), nil }}
		got, err := Apply(context.Background(), r, []byte("x"))
		if err != nil || got != want {
			t.Errorf("%q: changed=%v err=%v, want %v", out, got, err, want)
		}
		if !strings.HasPrefix(r.calls[0].line(), "kubectl --kubeconfig "+K3sKubeconfig+" apply") {
			t.Errorf("apply must target the k3s kubeconfig: %s", r.calls[0].line())
		}
	}
}

// An app whose image failed is disabled in phase 2 with no values (SC-29),
// and has no child Application to wait for; one that keeps its deployed tag
// stays enabled with it. A repo that is no build target is no skip.
func TestRenderEnvApp_SkipAndKeep(t *testing.T) {
	w := testWS(t)
	images := map[string]string{"demo-seed": "x", "shop-api": "a", "shop-web": "c"} // shop-gateway failed
	if got := SkippedApps(w, map[string]bool{"shop-gateway": true}); len(got) != 1 || got[0] != "shop-gateway" {
		t.Fatalf("SkippedApps = %v, want [shop-gateway]", got)
	}
	if got := SkippedApps(w, map[string]bool{}); len(got) != 0 {
		t.Errorf("SkippedApps without a failed build = %v, want none", got)
	}
	render2 := func(skip map[string]bool, kept map[string]string) map[string]any {
		b, err := RenderEnvApp(w, "base", "zenify.test", "main", images, 2, skip, kept)
		if err != nil {
			t.Fatal(err)
		}
		var a renderedApp
		if err := yaml.Unmarshal(b, &a); err != nil {
			t.Fatal(err)
		}
		return a.Spec.Source.Helm.ValuesObject
	}
	vals := render2(map[string]bool{"shop-gateway": true}, nil)
	if dig(vals, "shop-gateway", "enabled") != false || dig(vals, "shop-gateway", "values") != nil {
		t.Errorf("shop-gateway = %v, want disabled without values", vals["shop-gateway"])
	}
	if dig(vals, "shop-api", "values", "image", "tag") != "a" {
		t.Errorf("shop-api lost its tag: %v", vals["shop-api"])
	}
	vals = render2(nil, map[string]string{"shop-gateway": "sha-old"})
	if dig(vals, "shop-gateway", "enabled") == false || dig(vals, "shop-gateway", "values", "image", "tag") != "sha-old" {
		t.Errorf("kept shop-gateway = %v, want enabled at sha-old", vals["shop-gateway"])
	}
	if _, err := RenderEnvApp(w, "base", "zenify.test", "main", images, 2, nil, nil); err == nil {
		t.Error("a phase-2 app without a tag, skip or keep must stay a config error")
	}
	for _, n := range ChildApps(w, "base", 2, map[string]bool{"shop-gateway": true}) {
		if n == ChildAppName("shop-gateway", "base") {
			t.Error("ChildApps lists a skipped app")
		}
	}
}

// FR-13.5: an app outside the active profile is not rendered at all, like an
// app that does not exist; an app without profiles is in both.
func TestForProfile_FiltersAppsAndRender(t *testing.T) {
	w := testWS(t)
	for i := range w.EnvApps {
		if w.EnvApps[i].Key == "shop-gateway" {
			w.EnvApps[i].Profiles = []string{ProfileFull}
		}
	}
	has := func(w Workspace, key string) bool {
		for _, a := range w.EnvApps {
			if a.Key == key {
				return true
			}
		}
		return false
	}
	core, full := w.ForProfile(ProfileCore), w.ForProfile(ProfileFull)
	if has(core, "shop-gateway") || !has(full, "shop-gateway") {
		t.Fatalf("shop-gateway: core=%v full=%v, want false/true", has(core, "shop-gateway"), has(full, "shop-gateway"))
	}
	for _, k := range []string{"mongo", "app-config", "shop-api", "shop-web"} { // no profiles = both, seed and datastores included
		if !has(core, k) || !has(full, k) {
			t.Errorf("%s must be in both profiles", k)
		}
	}
	if len(w.EnvApps) != 7 {
		t.Errorf("ForProfile mutated the source: %d apps", len(w.EnvApps))
	}
	b, err := RenderEnvApp(core, "base", "zenify.test", "main", testImages, 2, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "shop-gateway") {
		t.Errorf("core render still names shop-gateway:\n%s", b)
	}
	b, err = RenderEnvApp(full, "base", "zenify.test", "main", testImages, 2, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "shop-gateway") {
		t.Errorf("full render lacks shop-gateway:\n%s", b)
	}
	for _, n := range ChildApps(core, "base", 2, nil) {
		if n == ChildAppName("shop-gateway", "base") {
			t.Error("core ChildApps lists shop-gateway")
		}
	}
	if got := SkippedApps(core, map[string]bool{"shop-gateway": true}); len(got) != 0 {
		t.Errorf("SkippedApps(core) = %v, want none: the app is not in the profile", got)
	}
}
