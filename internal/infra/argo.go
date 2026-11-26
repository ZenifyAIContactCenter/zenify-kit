package infra

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
)

const (
	// HelmRepoURL is the git repo holding the platform and env charts.
	HelmRepoURL = "https://github.com/ZenifyAIContactCenter/helm.git"
	// PlatformApp is the ArgoCD Application that installs the platform chart.
	PlatformApp = "local-platform"
	argoNS      = "argocd"
)

// pollInterval is how often WaitApp re-reads the Application (tests shorten it).
var pollInterval = 5 * time.Second

// maxGetFailures is how many consecutive failed Application reads WaitApp
// tolerates (e.g. the API server restarting) before it gives up.
const maxGetFailures = 3

func argoManifestURL() string {
	return "https://raw.githubusercontent.com/argoproj/argo-cd/" + ArgoCDVersion + "/manifests/install.yaml"
}

// InstallArgo applies the pinned ArgoCD manifest unless argocd-server already
// runs the pinned image, then waits for the ArgoCD workloads either way.
func InstallArgo(ctx context.Context, r Runner) (changed bool, err error) {
	img, err := kubectl(ctx, r, nil, "-n", argoNS, "get", "deploy", "argocd-server", "--ignore-not-found",
		"-o", "jsonpath={.spec.template.spec.containers[0].image}")
	if err != nil {
		return false, fmt.Errorf("install argocd: read installed version: %w", err)
	}
	if !strings.HasSuffix(strings.TrimSpace(string(img)), ":"+ArgoCDVersion) {
		if err := applyArgo(ctx, r); err != nil {
			return true, err
		}
		changed = true
	}
	// always wait: an earlier run may have applied the manifest and failed here
	for _, w := range []string{"deploy/argocd-server", "deploy/argocd-repo-server", "statefulset/argocd-application-controller"} {
		if _, err := kubectl(ctx, r, nil, "-n", argoNS, "rollout", "status", w, "--timeout=5m"); err != nil {
			return changed, fmt.Errorf("install argocd: wait %s: %w", w, err)
		}
	}
	return changed, nil
}

func applyArgo(ctx context.Context, r Runner) error {
	ns := "apiVersion: v1\nkind: Namespace\nmetadata:\n  name: " + argoNS + "\n"
	if _, err := kubectl(ctx, r, strings.NewReader(ns), "apply", "-f", "-"); err != nil {
		return fmt.Errorf("install argocd: create namespace: %w", err)
	}
	if _, err := kubectl(ctx, r, nil, "apply", "--server-side", "--force-conflicts", "-n", argoNS, "-f", argoManifestURL()); err != nil {
		return fmt.Errorf("install argocd %s: apply manifest: %w", ArgoCDVersion, err)
	}
	return nil
}

// SetRepoCredential stores the GitHub token ArgoCD uses to read the helm repo.
// The token travels on stdin only.
func SetRepoCredential(ctx context.Context, r Runner, token string) error {
	if strings.TrimSpace(token) == "" {
		return errors.New("set repo credential: empty token")
	}
	b, err := renderAsset("repo-secret.yaml.tmpl", map[string]string{"RepoURL": HelmRepoURL, "Token": token})
	if err != nil {
		return fmt.Errorf("set repo credential: %w", err)
	}
	// server-side apply: client-side apply would copy stringData (the token)
	// into the last-applied-configuration annotation
	if _, err := kubectl(ctx, r, bytes.NewReader(b), "apply", "--server-side", "--field-manager=zenify", "-f", "-"); err != nil {
		return fmt.Errorf("set repo credential: apply secret %s/repo-helm: %s", argoNS, strings.ReplaceAll(err.Error(), token, "***"))
	}
	return nil
}

func renderPlatformApp(domain, chartRef string) ([]byte, error) {
	if domain == "" || chartRef == "" {
		return nil, fmt.Errorf("platform app: domain %q and chartRef %q are both required", domain, chartRef)
	}
	return renderAsset("platform-app.yaml.tmpl", map[string]string{
		"Name": PlatformApp, "RepoURL": HelmRepoURL, "ChartRef": chartRef,
		"Domain": domain, "Namespace": PlatformNamespace,
	})
}

// ApplyPlatform creates or updates the local-platform Application at chartRef.
func ApplyPlatform(ctx context.Context, r Runner, domain, chartRef string) error {
	b, err := renderPlatformApp(domain, chartRef)
	if err != nil {
		return fmt.Errorf("apply platform: %w", err)
	}
	if _, err := kubectl(ctx, r, bytes.NewReader(b), "apply", "-f", "-"); err != nil {
		return fmt.Errorf("apply platform: %w", err)
	}
	return nil
}

type appStatus struct {
	Spec struct {
		Source  json.RawMessage `json:"source"`
		Sources json.RawMessage `json:"sources"`
	} `json:"spec"`
	Status struct {
		Sync struct {
			Status     string `json:"status"`
			ComparedTo struct {
				Source json.RawMessage `json:"source"`
			} `json:"comparedTo"`
		} `json:"sync"`
		Health struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"health"`
		Conditions []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"conditions"`
		OperationState struct {
			Phase   string `json:"phase"`
			Message string `json:"message"`
		} `json:"operationState"`
		Resources []struct {
			Kind      string `json:"kind"`
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
			Status    string `json:"status"`
			Health    *struct {
				Status  string `json:"status"`
				Message string `json:"message"`
			} `json:"health"`
		} `json:"resources"`
	} `json:"status"`
}

// fresh reports whether the status was computed against the current spec:
// right after an apply the controller still shows the previous comparison.
func (a appStatus) fresh() bool {
	var spec, compared any
	if len(a.Spec.Source) > 0 {
		if err := json.Unmarshal(a.Spec.Source, &spec); err != nil {
			return false
		}
	}
	if len(a.Status.Sync.ComparedTo.Source) > 0 {
		if err := json.Unmarshal(a.Status.Sync.ComparedTo.Source, &compared); err != nil {
			return false
		}
	}
	return reflect.DeepEqual(spec, compared)
}

// problems lists what keeps the app from Synced+Healthy, one line each.
func (a appStatus) problems() string {
	s := a.Status
	lines := []string{fmt.Sprintf("app sync=%s health=%s fresh=%v", s.Sync.Status, s.Health.Status, a.fresh())}
	if s.OperationState.Message != "" {
		lines = append(lines, fmt.Sprintf("operation %s: %s", s.OperationState.Phase, s.OperationState.Message))
	}
	for _, c := range s.Conditions {
		lines = append(lines, fmt.Sprintf("condition %s: %s", c.Type, c.Message))
	}
	for _, res := range s.Resources {
		health, msg := "", ""
		if res.Health != nil {
			health, msg = res.Health.Status, res.Health.Message
		}
		if res.Status == "Synced" && (health == "" || health == "Healthy") {
			continue
		}
		line := fmt.Sprintf("%s %s/%s sync=%s health=%s", res.Kind, res.Namespace, res.Name, res.Status, health)
		if msg != "" {
			line += ": " + msg
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n  ")
}

// WaitApp polls the Application until it is Synced and Healthy against its
// current spec (status.sync.comparedTo.source equals spec.source), so a status
// left from before an apply is not accepted. An Application that does not
// exist yet (a child its app-of-apps has still to create) is waited for until
// the timeout. On timeout the error lists the app's conditions and every
// resource not Synced+Healthy.
func WaitApp(ctx context.Context, r Runner, name string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var last appStatus
	failures := 0
	for {
		out, err := kubectl(ctx, r, nil, "-n", argoNS, "get", "application", name, "--ignore-not-found", "-o", "json")
		if err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("wait app %s: not ready within %s (%w):\n  %s", name, timeout, ctx.Err(), last.problems())
			}
			if failures++; failures >= maxGetFailures {
				return fmt.Errorf("wait app %s: %d consecutive reads failed: %w", name, failures, err)
			}
			if err := sleepCtx(ctx, pollInterval); err != nil {
				return fmt.Errorf("wait app %s: not ready within %s (%w):\n  %s", name, timeout, err, last.problems())
			}
			continue
		}
		failures = 0
		if len(bytes.TrimSpace(out)) == 0 { // not created yet
			if err := sleepCtx(ctx, pollInterval); err != nil {
				return fmt.Errorf("wait app %s: not created within %s (%w)", name, timeout, err)
			}
			continue
		}
		var a appStatus
		if err := json.Unmarshal(out, &a); err != nil {
			return fmt.Errorf("wait app %s: parse status: %w", name, err)
		}
		last = a
		if a.fresh() && a.Status.Sync.Status == "Synced" && a.Status.Health.Status == "Healthy" {
			return nil
		}
		if err := sleepCtx(ctx, pollInterval); err != nil {
			return fmt.Errorf("wait app %s: not ready within %s (%w):\n  %s", name, timeout, err, last.problems())
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
