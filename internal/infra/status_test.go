package infra

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

const appsJSON = `{"items":[
 {"metadata":{"name":"local-platform"},"status":{"sync":{"status":"Synced"},"health":{"status":"Healthy"}}},
 {"metadata":{"name":"env-base"},"spec":{"source":{"helm":{"valuesObject":{"local-env":{"values":{"domain":"old.test"}}}}}},"status":{"sync":{"status":"OutOfSync"},"health":{"status":"Progressing"}}},
 {"metadata":{"name":"local-env-base-local"},"status":{"sync":{"status":"Synced"},"health":{"status":"Healthy"}}}]}`

// status reads the platform and root env apps from the local cluster only, and
// DNS from a real resolvectl query of the env's web host.
func TestStatus_ReadsAppsAndRealDNS(t *testing.T) {
	r := &fakeRunner{respond: func(c call) ([]byte, error) {
		switch {
		case c.name == "k3s":
			return []byte("k3s version " + K3sVersion + " (x)\n"), nil
		case c.name == "kubectl" && strings.Contains(c.line(), "deploy argocd-server"):
			return []byte("quay.io/argoproj/argocd:" + ArgoCDVersion), nil
		case c.name == "kubectl" && strings.Contains(c.line(), "get applications"):
			return []byte(appsJSON), nil
		case c.name == "resolvectl":
			return nil, exitErr(1)
		}
		return nil, nil
	}}
	rep, err := Status(context.Background(), r, t.TempDir(), "quyentm.test")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Platform != (AppState{"Synced", "Healthy"}) || len(rep.Envs) != 1 || rep.Envs[0].Slug != "base" || rep.Envs[0].Health != "Progressing" {
		t.Fatalf("report = %+v", rep)
	}
	if rep.Envs[0].URLs[0] != "https://web.old.test" {
		t.Errorf("env url must use the env's own domain: %v", rep.Envs[0].URLs)
	}
	if rep.DNS.Host != "web.quyentm.test" || rep.DNS.Resolves {
		t.Fatalf("dns = %+v", rep.DNS)
	}
	if !r.ran("resolvectl query web.quyentm.test") {
		t.Error("dns must come from a real resolvectl query")
	}
	for _, c := range r.calls {
		if c.name == "kubectl" && !strings.HasPrefix(c.line(), "kubectl --kubeconfig "+K3sKubeconfig) {
			t.Errorf("kubectl without the k3s kubeconfig: %s", c.line())
		}
	}
}

// Without k3s there is no cluster to ask: report not installed, no kubectl.
func TestStatus_NoK3sSkipsCluster(t *testing.T) {
	r := &fakeRunner{respond: func(c call) ([]byte, error) {
		if c.name == "k3s" {
			return nil, exec.ErrNotFound
		}
		return nil, nil
	}}
	rep, err := Status(context.Background(), r, t.TempDir(), "quyentm.test")
	if err != nil {
		t.Fatal(err)
	}
	if rep.K3s != NotInstalled || rep.ArgoCD != NotInstalled || rep.Envs == nil || r.ran("kubectl") {
		t.Fatalf("report = %+v calls=%v", rep, r.calls)
	}
}

// Only an answer with the ingress IP means the env zone routes to the cluster;
// another resolver's *.test wildcard (127.0.0.1) must not pass.
func TestDNSResolves_RequiresIngressIP(t *testing.T) {
	for out, want := range map[string]bool{
		"web-base.quyentm.test: " + TraefikClusterIP + "   -- link: cni0\n": true,
		"web-base.quyentm.test: 127.0.0.1\n":                                false,
	} {
		r := &fakeRunner{respond: func(call) ([]byte, error) { return []byte(out), nil }}
		got, err := DNSResolves(context.Background(), r, "web-base.quyentm.test")
		if err != nil || got != want {
			t.Errorf("%q: got %v err %v, want %v", out, got, err, want)
		}
	}
}

// up compares against what is deployed; the read must come from the live
// local-platform Application and report "absent" without k3s.
func TestLivePlatform(t *testing.T) {
	r := &fakeRunner{respond: func(c call) ([]byte, error) {
		switch {
		case c.name == "k3s":
			return []byte("k3s version " + K3sVersion + " (x)\n"), nil
		case strings.Contains(c.line(), "deploy argocd-server"):
			return []byte("quay.io/argoproj/argocd:" + ArgoCDVersion), nil
		case strings.Contains(c.line(), "application local-platform --ignore-not-found"):
			return []byte(`{"spec":{"source":{"targetRevision":"feat/x","helm":{"valuesObject":{"domain":"quyentm.test"}}}}}`), nil
		}
		return nil, nil
	}}
	got, ok, err := LivePlatform(context.Background(), r)
	if err != nil || !ok || got != (PlatformTarget{"quyentm.test", "feat/x"}) {
		t.Fatalf("got %+v ok=%v err=%v", got, ok, err)
	}
	none := &fakeRunner{respond: func(c call) ([]byte, error) {
		if c.name == "k3s" {
			return nil, exec.ErrNotFound
		}
		return nil, nil
	}}
	if _, ok, err := LivePlatform(context.Background(), none); ok || err != nil || none.ran("kubectl") {
		t.Fatalf("ok=%v err=%v calls=%v", ok, err, none.calls)
	}
}
