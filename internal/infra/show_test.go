package infra

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func child(name, tag string) string {
	return `{"metadata":{"name":"` + name + `"},"spec":{"source":{"helm":{"valuesObject":{"image":{"tag":"` + tag + `"}}}}},"status":{"sync":{"status":"Synced"},"health":{"status":"Healthy"}}}`
}

const showRoot = `{"metadata":{"name":"env-base"},"spec":{"source":{"helm":{"valuesObject":{"app-config":{"values":{"domain":"devbox2.test"}}}}}},"status":{"sync":{"status":"Synced"},"health":{"status":"Healthy"}}}`

func showRunner(items ...string) *fakeRunner {
	return &fakeRunner{respond: func(c call) ([]byte, error) {
		switch {
		case c.name == "k3s":
			return []byte("k3s version " + K3sVersion + " (x)\n"), nil
		case strings.Contains(c.line(), "deploy argocd-server"):
			return []byte("quay.io/argoproj/argocd:" + ArgoCDVersion), nil
		case strings.Contains(c.line(), "get applications"):
			return []byte(`{"items":[` + strings.Join(items, ",") + `]}`), nil
		}
		return nil, nil
	}}
}

// The tags shown must be the ones the env runs (child Application values), so
// a dev can tell which commit is deployed; show must never change the cluster.
func TestShow_URLsReposAndRunningTags(t *testing.T) {
	r := showRunner(showRoot,
		child("shop-api-base-local", "sha-be00001"),
		child("shop-gateway-base-local", "sha-hub0001"),
		child("shop-web-base-local", "sha-web0001-v1234abcd"),
		child("app-config-base-local", ""))
	rep, err := Show(context.Background(), r, testWS(t), "/home/u", "other.test", "")
	if err != nil {
		t.Fatal(err)
	}
	if rep.CA != "/home/u/.zenify/infra/ca.crt" || len(rep.Envs) != 1 || rep.Envs[0].Sync != "Synced" {
		t.Fatalf("report = %+v", rep)
	}
	want := []ShowApp{
		{"be", "https://be.devbox2.test", "shop-api", "sha-be00001"},
		{"hub", "https://hub.devbox2.test", "shop-gateway", "sha-hub0001"},
		{"web", "https://web.devbox2.test", "shop-web", "sha-web0001-v1234abcd"},
	}
	got := rep.Envs[0].Apps
	if len(got) != len(want) {
		t.Fatalf("apps = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("app %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	for _, c := range r.calls {
		if c.name == "kubectl" && (!strings.HasPrefix(c.line(), "kubectl --kubeconfig "+K3sKubeconfig) ||
			(!strings.Contains(c.line(), " get ") && c.line() != "kubectl --kubeconfig "+K3sKubeconfig+" config current-context")) {
			t.Errorf("show ran a non-read or non-local kubectl: %s", c.line())
		}
	}
}

// A mistyped --slug must fail and list the envs that exist.
func TestShow_UnknownSlugListsEnvs(t *testing.T) {
	_, err := Show(context.Background(), showRunner(showRoot), testWS(t), "/home/u", "devbox2.test", "nope")
	if err == nil || !strings.Contains(err.Error(), `"nope"`) || !strings.Contains(err.Error(), "envs: base") {
		t.Fatalf("err = %v", err)
	}
}

// The seed password must never leave the kit through show (JSON or struct).
func TestShow_JSONHasAccountsButNoPassword(t *testing.T) {
	rep, err := Show(context.Background(), showRunner(), testWS(t), "/home/u", "devbox2.test", "")
	if err != nil {
		t.Fatal(err)
	}
	noArgo := rep
	noArgo.Cluster.ArgoCD = nil // carries the command that reads Argo's password, checked below
	nb, err := json.Marshal(noArgo)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(nb)), "password") {
		t.Fatalf("password in show JSON: %s", nb)
	}
	b, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Domain   string `json:"domain"`
		CA       string `json:"ca"`
		Envs     []any  `json:"envs"`
		Accounts []struct {
			Role, Email, Tenant string
		} `json:"accounts"`
		LoginHelp string `json:"loginHelp"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Domain != "devbox2.test" || got.CA == "" || got.Envs == nil || len(got.Accounts) != 2 ||
		got.Accounts[0].Email != "admin@demo.example" || got.Accounts[1].Role != "agent" || got.Accounts[0].Tenant != "demotenant" ||
		got.LoginHelp != "see the demo-seed README" {
		t.Fatalf("show json = %s", b)
	}
}

// The running tag is what show exists for: a source that cannot be read must
// fail naming the app, never look like "not deployed yet" (-).
func TestShow_UnreadableTagIsAnError(t *testing.T) {
	bad := `{"metadata":{"name":"shop-web-base-local"},"spec":{"source":{"helm":"oops"}},"status":{}}`
	_, err := Show(context.Background(), showRunner(showRoot, bad), testWS(t), "/home/u", "devbox2.test", "")
	if err == nil || !strings.Contains(err.Error(), "shop-web-base-local") || !strings.Contains(err.Error(), "unreadable") {
		t.Fatalf("err = %v", err)
	}
	multi := `{"metadata":{"name":"shop-api-base-local"},"spec":{"sources":[{"helm":{}}]},"status":{}}`
	if _, err := Show(context.Background(), showRunner(showRoot, multi), testWS(t), "/home/u", "devbox2.test", ""); err == nil || !strings.Contains(err.Error(), "multi-source") {
		t.Fatalf("multi-source err = %v", err)
	}
}

// A child that exists without image.tag, or not at all yet, has no tag ("-").
func TestShow_MissingTagIsEmpty(t *testing.T) {
	noTag := `{"metadata":{"name":"shop-api-base-local"},"spec":{"source":{"helm":{"valuesObject":{}}}},"status":{}}`
	rep, err := Show(context.Background(), showRunner(showRoot, noTag), testWS(t), "/home/u", "devbox2.test", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range rep.Envs[0].Apps {
		if a.Tag != "" {
			t.Errorf("%s tag = %q, want empty", a.Svc, a.Tag)
		}
	}
}

// An env's URLs come from its own domain: an unreadable root source must fail
// naming the app in show and status, not silently use the default domain.
func TestEnvDomain_UnreadableRootIsAnError(t *testing.T) {
	badRoot := `{"metadata":{"name":"env-base"},"spec":{"source":{"helm":"oops"}},"status":{}}`
	if _, err := Show(context.Background(), showRunner(badRoot), testWS(t), "/home/u", "devbox2.test", ""); err == nil ||
		!strings.Contains(err.Error(), "env-base") || !strings.Contains(err.Error(), "env domain unreadable") {
		t.Fatalf("show err = %v", err)
	}
	if _, err := Status(context.Background(), showRunner(badRoot), testWS(t), t.TempDir(), "devbox2.test"); err == nil ||
		!strings.Contains(err.Error(), "env-base") || !strings.Contains(err.Error(), "env domain unreadable") {
		t.Fatalf("status err = %v", err)
	}
}

// A root without <seed app>.values.domain uses the resolved domain.
func TestEnvDomain_MissingFallsBack(t *testing.T) {
	root := `{"metadata":{"name":"env-base"},"spec":{"source":{"helm":{"valuesObject":{}}}},"status":{}}`
	rep, err := Show(context.Background(), showRunner(root), testWS(t), "/home/u", "fallback.test", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := rep.Envs[0].Apps[0].URL; got != "https://be.fallback.test" {
		t.Fatalf("url = %s", got)
	}
}

func clusterRunner(ingressHosts, openbao string) *fakeRunner {
	return clusterRunnerSecret(ingressHosts, openbao, base64.StdEncoding.EncodeToString([]byte("planted-admin-secret")))
}

func clusterRunnerSecret(ingressHosts, openbao, argoSecret string) *fakeRunner {
	base := showRunner()
	return &fakeRunner{respond: func(c call) ([]byte, error) {
		l := c.line()
		switch {
		case strings.HasSuffix(l, "config current-context"):
			return []byte("default\n"), nil
		case strings.Contains(l, "get nodes"):
			return []byte("devbox " + K3sVersion + "\n"), nil
		case strings.Contains(l, "get ingress"):
			return []byte(ingressHosts), nil
		case strings.Contains(l, "app.kubernetes.io/name=openbao"):
			return []byte(openbao), nil
		case strings.Contains(l, "argocd-initial-admin-secret"):
			return []byte(argoSecret), nil
		}
		return base.respond(c)
	}}
}

// A dev must be able to reach the cluster with their own kubectl, registry and
// ArgoCD (including its admin login) from show alone, and show must not invent
// rows for absent parts.
func TestShow_ClusterAccess(t *testing.T) {
	r := clusterRunner("", "")
	rep, err := Show(context.Background(), r, testWS(t), "/home/u", "devbox2.test", "")
	if err != nil {
		t.Fatal(err)
	}
	c := rep.Cluster
	if c.Kubeconfig != K3sKubeconfig || c.Export != "export KUBECONFIG="+K3sKubeconfig || !strings.Contains(c.KubeconfigNote, "group") {
		t.Fatalf("kubeconfig rows = %+v", c)
	}
	if c.Context != "default" || len(c.Nodes) != 1 || c.Nodes[0] != (ShowNode{"devbox", K3sVersion}) {
		t.Fatalf("context/nodes = %q %+v", c.Context, c.Nodes)
	}
	if c.Registry.Alias != RegistryHost || c.Registry.Address != RegistryClusterIP+":5000" {
		t.Fatalf("registry = %+v", c.Registry)
	}
	if c.ArgoCD == nil || c.ArgoCD.URL != "https://argocd-server.argocd.svc" || c.ArgoCD.PasswordCommand != argoPasswordCommand ||
		c.ArgoCD.Password != "planted-admin-secret" || c.ArgoCD.IngressURL != "" {
		t.Fatalf("argocd = %+v", c.ArgoCD)
	}
	if c.OpenBao != "" {
		t.Fatalf("openbao row without a Service: %q", c.OpenBao)
	}
	b, _ := json.Marshal(rep)
	if strings.Contains(string(b), `"openbao"`) || strings.Contains(string(b), "ingressUrl") {
		t.Fatalf("show json = %s", b)
	}

	rep, err = Show(context.Background(), clusterRunner("argocd.devbox2.test", "openbao openbao 8200\n"), testWS(t), "/home/u", "devbox2.test", "")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Cluster.ArgoCD.IngressURL != "https://argocd.devbox2.test" || rep.Cluster.OpenBao != "http://openbao.openbao.svc:8200" {
		t.Fatalf("ingress/openbao = %+v %q", rep.Cluster.ArgoCD, rep.Cluster.OpenBao)
	}
}

// Once the dev deleted argocd-initial-admin-secret (after changing the admin
// password), show must still succeed and fall back to the command.
func TestShow_ArgoSecretDeleted(t *testing.T) {
	rep, err := Show(context.Background(), clusterRunnerSecret("", "", ""), testWS(t), "/home/u", "devbox2.test", "")
	if err != nil {
		t.Fatal(err)
	}
	if a := rep.Cluster.ArgoCD; a == nil || a.Password != "" || a.PasswordCommand != argoPasswordCommand {
		t.Fatalf("argocd = %+v", rep.Cluster.ArgoCD)
	}
}

// With seed.password set, every account carries it so a dev logs in from show
// alone, and the README hint is dropped.
func TestShow_SeedPassword(t *testing.T) {
	w := testWS(t)
	w.Seed.Password = "seed-test-pw"
	rep, err := Show(context.Background(), showRunner(), w, "/home/u", "devbox2.test", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range rep.Accounts {
		if a.Password != "seed-test-pw" || a.Tenant != "demotenant" {
			t.Fatalf("account = %+v", a)
		}
	}
	if rep.LoginHelp != "" {
		t.Fatalf("loginHelp = %q with seed.password set", rep.LoginHelp)
	}
}

// Only phase-2 apps with a child Application carrying an image.tag are
// "deployed"; an app with no Application or no tag is not.
func TestDeployedTags_OnlyAppsRunningATag(t *testing.T) {
	r := showRunner(showRoot, child("shop-api-base-local", "sha-be00001"), child("shop-gateway-base-local", ""),
		child("app-config-base-local", "sha-seed"))
	got, err := DeployedTags(context.Background(), r, testWS(t), "base")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["shop-api"] != "sha-be00001" {
		t.Errorf("DeployedTags = %v, want only shop-api at sha-be00001", got)
	}
}
