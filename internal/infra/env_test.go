package infra

import (
	"context"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var testImages = map[string]string{
	"contact-center-be": "sha-be00001", "contact-center-hub": "sha-hub0001",
	"contact-center-web": "sha-web0001-v12345678", SeedRepo: "sha-seed001",
}

type renderedApp struct {
	Metadata struct {
		Name       string   `yaml:"name"`
		Namespace  string   `yaml:"namespace"`
		Finalizers []string `yaml:"finalizers"`
	} `yaml:"metadata"`
	Spec struct {
		Source struct {
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
	b, err := RenderEnvApp("base", "zenify.test", "main", testImages, ProfileCore, phase)
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

func enabledApps(vals map[string]any) []string {
	var on []string
	for _, a := range envApps {
		if dig(vals, a.name, "enabled") != false {
			on = append(on, a.name)
		}
	}
	return on
}

// Phase 1 must not start be/hub/web: the seed runs --if-empty and refuses once
// an app has created its collections.
func TestRenderEnvApp_Phase1EnablesOnlyDatastoresAndSeed(t *testing.T) {
	a := render(t, 1)
	got := strings.Join(enabledApps(a.Spec.Source.Helm.ValuesObject), ",")
	if got != "mongo,mysql,redis,local-env" {
		t.Fatalf("phase 1 enabled = %s", got)
	}
	if tag := dig(a.Spec.Source.Helm.ValuesObject, "local-env", "values", "seed", "tag"); tag != "sha-seed001" {
		t.Fatalf("seed.tag = %v", tag)
	}
}

// Phase 2 deploys all seven child apps of env base with the hosts the
// helm envs/local files expect; a wrong name or host breaks the URL we print.
func TestRenderEnvApp_Phase2SevenAppsNamesAndHosts(t *testing.T) {
	a := render(t, 2)
	vals := a.Spec.Source.Helm.ValuesObject
	if a.Metadata.Name != "env-base" || a.Spec.Source.Path != "charts/app-of-apps" || a.Spec.Source.TargetRevision != "main" {
		t.Fatalf("root app = %s path %s rev %s", a.Metadata.Name, a.Spec.Source.Path, a.Spec.Source.TargetRevision)
	}
	if len(a.Metadata.Finalizers) != 1 || a.Metadata.Finalizers[0] != "resources-finalizer.argocd.argoproj.io" {
		t.Fatalf("finalizers = %v", a.Metadata.Finalizers)
	}
	if vals["env"] != "local" || dig(vals, "global", "targetRevision") != "main" {
		t.Fatalf("env/global = %v/%v", vals["env"], vals["global"])
	}
	on := enabledApps(vals)
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
	if h := dig(host("contact-center-web", "contact-center-web", "ingress", "hosts"), "host"); h != "web.zenify.test" {
		t.Errorf("web host = %v", h)
	}
	if h := dig(host("contact-center-be", "api", "ingress", "hosts"), "host"); h != "be.zenify.test" {
		t.Errorf("be host = %v", h)
	}
	if h := dig(host("contact-center-hub", "hub", "ingress", "hosts"), "host"); h != "hub.zenify.test" {
		t.Errorf("hub host = %v", h)
	}
	auth := host("contact-center-hub", "auth", "ingress", "hosts")
	paths, _ := dig(auth, "paths").([]any)
	if dig(auth, "host") != "hub.zenify.test" || len(paths) != 1 || dig(paths[0], "path") != "/auth" {
		t.Errorf("auth host = %v", auth)
	}
	if tag := dig(vals, "contact-center-web", "values", "image", "tag"); tag != "sha-web0001-v12345678" {
		t.Errorf("web image.tag = %v", tag)
	}
}

// A missing image tag would deploy an app with no image: refuse to render.
func TestRenderEnvApp_RejectsMissingTagAndBadInput(t *testing.T) {
	if _, err := RenderEnvApp("base", "quyentm.test", "main", map[string]string{SeedRepo: "x"}, ProfileCore, 2); err == nil {
		t.Fatal("phase 2 without app tags must fail")
	}
	if _, err := RenderEnvApp("base", "quyentm.test", "main", testImages, "full", 2); err == nil {
		t.Fatal("unknown profile must fail")
	}
	if _, err := RenderEnvApp("Bad_Slug", "quyentm.test", "main", testImages, ProfileCore, 2); err == nil {
		t.Fatal("invalid slug must fail")
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
