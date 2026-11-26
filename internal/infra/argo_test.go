package infra

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

const testRepoURL = "https://github.com/example-team/charts.git"

// Business rule: the GitHub token must never reach argv (visible in ps and error text).
func TestSetRepoCredentialTokenOnlyOnStdin(t *testing.T) {
	const token = "ghp_secretTOKEN123"
	r := &fakeRunner{}
	if err := SetRepoCredential(context.Background(), r, testRepoURL, token); err != nil {
		t.Fatal(err)
	}
	inStdin := false
	for _, c := range r.calls {
		for _, a := range append([]string{c.name}, c.args...) {
			if strings.Contains(a, token) {
				t.Fatalf("token in argv: %v", c.args)
			}
		}
		if strings.Contains(c.stdin, token) {
			inStdin = true
			if !strings.Contains(c.stdin, "argocd.argoproj.io/secret-type: repository") || !strings.Contains(c.stdin, `url: "`+testRepoURL+`"`) {
				t.Fatalf("secret manifest incomplete:\n%s", c.stdin)
			}
		}
	}
	if !inStdin {
		t.Fatal("token not sent on stdin")
	}
}

// Business rule: the token must not be copied into the last-applied annotation
// (server-side apply) nor surface in an error message.
func TestSetRepoCredentialServerSideAndRedacted(t *testing.T) {
	const token = "ghp_secretTOKEN123"
	r := &fakeRunner{respond: func(call) ([]byte, error) {
		return nil, errors.New("admission denied: password " + token)
	}}
	err := SetRepoCredential(context.Background(), r, testRepoURL, token)
	if err == nil || strings.Contains(err.Error(), token) || !strings.Contains(err.Error(), "***") {
		t.Fatalf("err = %v", err)
	}
	if !r.ran("kubectl --kubeconfig " + K3sKubeconfig + " apply --server-side") {
		t.Fatalf("not server-side: %v", r.calls)
	}
}

func TestSetRepoCredentialRejectsEmptyToken(t *testing.T) {
	if err := SetRepoCredential(context.Background(), &fakeRunner{}, testRepoURL, " "); err == nil {
		t.Fatal("want error for an empty token")
	}
	if err := SetRepoCredential(context.Background(), &fakeRunner{}, "", "tok"); err == nil {
		t.Fatal("want error for an empty repo URL (infra.json helmRepoURL)")
	}
}

// Business rule: the platform app syncs the chart from the requested ref with the user's domain.
func TestRenderPlatformApp(t *testing.T) {
	b, err := renderPlatformApp(testRepoURL, "devbox2.test", "feat/local")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		"name: local-platform",
		`targetRevision: "feat/local"`,
		`domain: "devbox2.test"`,
		"path: charts/local-platform",
		"- ../../envs/local-platform/values.yaml",
		"namespace: " + PlatformNamespace,
		`repoURL: "` + testRepoURL + `"`,
		"- ServerSideApply=true",
		"- CreateNamespace=true",
		"selfHeal: true",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

func TestApplyPlatformSendsManifestOnStdin(t *testing.T) {
	r := &fakeRunner{}
	if err := ApplyPlatform(context.Background(), r, testRepoURL, "devbox2.test", "main"); err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 1 || !strings.Contains(r.calls[0].line(), "--kubeconfig "+K3sKubeconfig+" apply") ||
		!strings.Contains(r.calls[0].stdin, "name: local-platform") {
		t.Fatalf("calls = %+v", r.calls)
	}
}

// Business rule: argocd install is skipped when every ArgoCD workload already
// runs the pinned version.
func TestInstallArgoPinnedIsNoop(t *testing.T) {
	r := &fakeRunner{respond: func(c call) ([]byte, error) {
		if strings.Contains(c.line(), " get ") && strings.Contains(c.line(), "jsonpath") {
			return []byte("quay.io/argoproj/argocd:" + ArgoCDVersion), nil
		}
		return nil, nil
	}}
	changed, err := InstallArgo(context.Background(), r)
	if err != nil || changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if r.ran("kubectl --kubeconfig " + K3sKubeconfig + " apply") {
		t.Fatal("apply ran on a pinned install")
	}
	// an earlier run may have applied and then failed its wait
	if !r.ran("kubectl --kubeconfig " + K3sKubeconfig + " -n argocd rollout status statefulset/argocd-application-controller") {
		t.Fatalf("workloads not awaited: %v", r.calls)
	}
}

// Business rule: a partial earlier apply (server pinned, controller missing)
// must be re-applied, or every rerun fails at the rollout wait.
func TestInstallArgoReappliesPartialInstall(t *testing.T) {
	r := &fakeRunner{respond: func(c call) ([]byte, error) {
		if strings.Contains(c.line(), "get deploy/argocd-server") || strings.Contains(c.line(), "get deploy/argocd-repo-server") {
			return []byte("quay.io/argoproj/argocd:" + ArgoCDVersion), nil
		}
		return nil, nil // statefulset/argocd-application-controller absent
	}}
	changed, err := InstallArgo(context.Background(), r)
	if err != nil || !changed || !r.ran("kubectl --kubeconfig "+K3sKubeconfig+" apply --server-side --force-conflicts -n argocd") {
		t.Fatalf("changed=%v err=%v calls=%v", changed, err, r.calls)
	}
}

func TestInstallArgoAppliesPinnedManifest(t *testing.T) {
	r := &fakeRunner{}
	changed, err := InstallArgo(context.Background(), r)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	want := "apply --server-side --force-conflicts -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/" + ArgoCDVersion + "/manifests/install.yaml"
	if !r.ran("kubectl --kubeconfig " + K3sKubeconfig + " " + want) {
		t.Fatalf("calls = %+v", r.calls)
	}
}

const degradedApp = `{"status":{"sync":{"status":"Synced"},"health":{"status":"Degraded"},
"conditions":[{"type":"SyncError","message":"one or more objects failed"}],
"resources":[
 {"kind":"Deployment","namespace":"zenify-system","name":"local-dns","status":"Synced","health":{"status":"Degraded","message":"Deployment exceeded its progress deadline"}},
 {"kind":"Service","namespace":"zenify-system","name":"registry","status":"Synced","health":{"status":"Healthy"}}]}}`

// Business rule: a failed platform sync must name the broken resource so the user can act.
func TestWaitAppTimeoutNamesUnhealthyResources(t *testing.T) {
	old := pollInterval
	pollInterval = time.Millisecond
	defer func() { pollInterval = old }()
	r := &fakeRunner{respond: func(call) ([]byte, error) { return []byte(degradedApp), nil }}
	err := WaitApp(context.Background(), r, "local-platform", 20*time.Millisecond)
	if err == nil {
		t.Fatal("want timeout error")
	}
	msg := err.Error()
	for _, want := range []string{"Deployment zenify-system/local-dns", "Degraded", "progress deadline", "one or more objects failed"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error lacks %q: %s", want, msg)
		}
	}
	if strings.Contains(msg, "registry") {
		t.Errorf("healthy resource listed: %s", msg)
	}
}

func TestWaitAppHealthy(t *testing.T) {
	r := &fakeRunner{respond: func(call) ([]byte, error) {
		return []byte(`{"status":{"sync":{"status":"Synced"},"health":{"status":"Healthy"}}}`), nil
	}}
	if err := WaitApp(context.Background(), r, "local-platform", time.Second); err != nil {
		t.Fatal(err)
	}
}

// Business rule: a brief API outage (k3s restart) must not abort the wait.
func TestWaitAppToleratesTransientErrors(t *testing.T) {
	old := pollInterval
	pollInterval = time.Millisecond
	defer func() { pollInterval = old }()
	n := 0
	r := &fakeRunner{respond: func(call) ([]byte, error) {
		if n++; n <= maxGetFailures-1 {
			return nil, errors.New("connection refused")
		}
		return []byte(`{"status":{"sync":{"status":"Synced"},"health":{"status":"Healthy"}}}`), nil
	}}
	if err := WaitApp(context.Background(), r, "local-platform", time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestWaitAppGivesUpAfterRepeatedErrors(t *testing.T) {
	old := pollInterval
	pollInterval = time.Millisecond
	defer func() { pollInterval = old }()
	r := &fakeRunner{respond: func(call) ([]byte, error) { return nil, errors.New("connection refused") }}
	err := WaitApp(context.Background(), r, "local-platform", time.Second)
	if err == nil || !strings.Contains(err.Error(), "connection refused") || len(r.calls) != maxGetFailures {
		t.Fatalf("err=%v calls=%d", err, len(r.calls))
	}
}

const (
	staleApp = `{"spec":{"source":{"targetRevision":"main","helm":{"valuesObject":{"image":{"tag":"sha-new"}}}}},
"status":{"sync":{"status":"Synced","comparedTo":{"source":{"targetRevision":"main","helm":{"valuesObject":{"image":{"tag":"sha-old"}}}}}},"health":{"status":"Healthy"}}}`
	freshApp = `{"spec":{"source":{"targetRevision":"main","helm":{"valuesObject":{"image":{"tag":"sha-new"}}}}},
"status":{"sync":{"status":"Synced","comparedTo":{"source":{"helm":{"valuesObject":{"image":{"tag":"sha-new"}}},"targetRevision":"main"}}},"health":{"status":"Healthy"}}}`
)

// Business rule: right after an apply Argo still shows the old comparison as
// Synced/Healthy; accepting it would report an env ready on the old images.
func TestWaitAppIgnoresStatusOfPreviousSpec(t *testing.T) {
	old := pollInterval
	pollInterval = time.Millisecond
	defer func() { pollInterval = old }()
	stale := &fakeRunner{respond: func(call) ([]byte, error) { return []byte(staleApp), nil }}
	if err := WaitApp(context.Background(), stale, "env-base", 20*time.Millisecond); err == nil || !strings.Contains(err.Error(), "fresh=false") {
		t.Fatalf("stale status accepted or unnamed: %v", err)
	}
	n := 0
	r := &fakeRunner{respond: func(call) ([]byte, error) {
		if n++; n <= 3 {
			return []byte(staleApp), nil
		}
		return []byte(freshApp), nil
	}}
	if err := WaitApp(context.Background(), r, "env-base", time.Second); err != nil || n != 4 {
		t.Fatalf("err=%v reads=%d", err, n)
	}
}

// Business rule: a child Application appears only after its app-of-apps
// synced; NotFound while waiting is "not yet", not a failure.
func TestWaitAppWaitsForAppToBeCreated(t *testing.T) {
	old := pollInterval
	pollInterval = time.Millisecond
	defer func() { pollInterval = old }()
	n := 0
	r := &fakeRunner{respond: func(c call) ([]byte, error) {
		if !strings.Contains(c.line(), "--ignore-not-found") {
			t.Errorf("get without --ignore-not-found: %s", c.line())
		}
		if n++; n <= 4*maxGetFailures { // absent far longer than the read-failure budget
			return nil, nil
		}
		return []byte(freshApp), nil
	}}
	if err := WaitApp(context.Background(), r, "local-env-base-local", time.Second); err != nil {
		t.Fatal(err)
	}
	absent := &fakeRunner{respond: func(call) ([]byte, error) { return nil, nil }}
	if err := WaitApp(context.Background(), absent, "x", 10*time.Millisecond); err == nil || !strings.Contains(err.Error(), "not created") {
		t.Fatalf("err = %v", err)
	}
}
