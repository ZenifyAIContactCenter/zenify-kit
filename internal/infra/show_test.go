package infra

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func child(name, tag string) string {
	return `{"metadata":{"name":"` + name + `"},"spec":{"source":{"helm":{"valuesObject":{"image":{"tag":"` + tag + `"}}}}},"status":{"sync":{"status":"Synced"},"health":{"status":"Healthy"}}}`
}

const showRoot = `{"metadata":{"name":"env-base"},"spec":{"source":{"helm":{"valuesObject":{"local-env":{"values":{"domain":"quyentm.test"}}}}}},"status":{"sync":{"status":"Synced"},"health":{"status":"Healthy"}}}`

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
		child("contact-center-be-base-local", "sha-be00001"),
		child("contact-center-hub-base-local", "sha-hub0001"),
		child("contact-center-web-base-local", "sha-web0001-v1234abcd"),
		child("local-env-base-local", ""))
	rep, err := Show(context.Background(), r, "/home/u", "other.test", "")
	if err != nil {
		t.Fatal(err)
	}
	if rep.CA != "/home/u/.zenify/infra/ca.crt" || len(rep.Envs) != 1 || rep.Envs[0].Sync != "Synced" {
		t.Fatalf("report = %+v", rep)
	}
	want := []ShowApp{
		{"be", "https://be-base.quyentm.test", "contact-center-be", "sha-be00001"},
		{"hub", "https://hub-base.quyentm.test", "contact-center-hub", "sha-hub0001"},
		{"web", "https://web-base.quyentm.test", "contact-center-web", "sha-web0001-v1234abcd"},
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
		if c.name == "kubectl" && (!strings.HasPrefix(c.line(), "kubectl --kubeconfig "+K3sKubeconfig) || !strings.Contains(c.line(), " get ")) {
			t.Errorf("show ran a non-read or non-local kubectl: %s", c.line())
		}
	}
}

// A mistyped --slug must fail and list the envs that exist.
func TestShow_UnknownSlugListsEnvs(t *testing.T) {
	_, err := Show(context.Background(), showRunner(showRoot), "/home/u", "quyentm.test", "nope")
	if err == nil || !strings.Contains(err.Error(), `"nope"`) || !strings.Contains(err.Error(), "envs: base") {
		t.Fatalf("err = %v", err)
	}
}

// The seed password must never leave the kit through show (JSON or struct).
func TestShow_JSONHasAccountsButNoPassword(t *testing.T) {
	rep, err := Show(context.Background(), showRunner(), "/home/u", "quyentm.test", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(b)), "password") {
		t.Fatalf("password in show JSON: %s", b)
	}
	var got struct {
		Domain   string `json:"domain"`
		CA       string `json:"ca"`
		Envs     []any  `json:"envs"`
		Accounts []struct {
			Role, Email, Tenant string
		} `json:"accounts"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Domain != "quyentm.test" || got.CA == "" || got.Envs == nil || len(got.Accounts) != 2 ||
		got.Accounts[0].Email != "admin@zenifytest.vn" || got.Accounts[1].Role != "agent" || got.Accounts[0].Tenant != "zenifytest" {
		t.Fatalf("show json = %s", b)
	}
}

// The running tag is what show exists for: a source that cannot be read must
// fail naming the app, never look like "not deployed yet" (-).
func TestShow_UnreadableTagIsAnError(t *testing.T) {
	bad := `{"metadata":{"name":"contact-center-web-base-local"},"spec":{"source":{"helm":"oops"}},"status":{}}`
	_, err := Show(context.Background(), showRunner(showRoot, bad), "/home/u", "quyentm.test", "")
	if err == nil || !strings.Contains(err.Error(), "contact-center-web-base-local") || !strings.Contains(err.Error(), "unreadable") {
		t.Fatalf("err = %v", err)
	}
	multi := `{"metadata":{"name":"contact-center-be-base-local"},"spec":{"sources":[{"helm":{}}]},"status":{}}`
	if _, err := Show(context.Background(), showRunner(showRoot, multi), "/home/u", "quyentm.test", ""); err == nil || !strings.Contains(err.Error(), "multi-source") {
		t.Fatalf("multi-source err = %v", err)
	}
}

// A child that exists without image.tag, or not at all yet, has no tag ("-").
func TestShow_MissingTagIsEmpty(t *testing.T) {
	noTag := `{"metadata":{"name":"contact-center-be-base-local"},"spec":{"source":{"helm":{"valuesObject":{}}}},"status":{}}`
	rep, err := Show(context.Background(), showRunner(showRoot, noTag), "/home/u", "quyentm.test", "")
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
	if _, err := Show(context.Background(), showRunner(badRoot), "/home/u", "quyentm.test", ""); err == nil ||
		!strings.Contains(err.Error(), "env-base") || !strings.Contains(err.Error(), "env domain unreadable") {
		t.Fatalf("show err = %v", err)
	}
	if _, err := Status(context.Background(), showRunner(badRoot), t.TempDir(), "quyentm.test"); err == nil ||
		!strings.Contains(err.Error(), "env-base") || !strings.Contains(err.Error(), "env domain unreadable") {
		t.Fatalf("status err = %v", err)
	}
}

// A root without local-env.values.domain uses the resolved domain.
func TestEnvDomain_MissingFallsBack(t *testing.T) {
	root := `{"metadata":{"name":"env-base"},"spec":{"source":{"helm":{"valuesObject":{}}}},"status":{}}`
	rep, err := Show(context.Background(), showRunner(root), "/home/u", "fallback.test", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := rep.Envs[0].Apps[0].URL; got != "https://be-base.fallback.test" {
		t.Fatalf("url = %s", got)
	}
}
