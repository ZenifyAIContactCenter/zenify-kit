package infra

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func argoManifestURL() string {
	return "https://raw.githubusercontent.com/argoproj/argo-cd/" + ArgoCDVersion + "/manifests/install.yaml"
}

// InstallArgo applies the pinned ArgoCD manifest and waits for its workloads;
// it does nothing when argocd-server already runs the pinned image.
func InstallArgo(ctx context.Context, r Runner) (changed bool, err error) {
	img, err := kubectl(ctx, r, nil, "-n", argoNS, "get", "deploy", "argocd-server", "--ignore-not-found",
		"-o", "jsonpath={.spec.template.spec.containers[0].image}")
	if err != nil {
		return false, fmt.Errorf("install argocd: read installed version: %w", err)
	}
	if strings.HasSuffix(strings.TrimSpace(string(img)), ":"+ArgoCDVersion) {
		return false, nil
	}
	ns := "apiVersion: v1\nkind: Namespace\nmetadata:\n  name: " + argoNS + "\n"
	if _, err := kubectl(ctx, r, strings.NewReader(ns), "apply", "-f", "-"); err != nil {
		return false, fmt.Errorf("install argocd: create namespace: %w", err)
	}
	if _, err := kubectl(ctx, r, nil, "apply", "--server-side", "--force-conflicts", "-n", argoNS, "-f", argoManifestURL()); err != nil {
		return true, fmt.Errorf("install argocd %s: apply manifest: %w", ArgoCDVersion, err)
	}
	for _, w := range []string{"deploy/argocd-server", "deploy/argocd-repo-server", "statefulset/argocd-application-controller"} {
		if _, err := kubectl(ctx, r, nil, "-n", argoNS, "rollout", "status", w, "--timeout=5m"); err != nil {
			return true, fmt.Errorf("install argocd: wait %s: %w", w, err)
		}
	}
	return true, nil
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
	if _, err := kubectl(ctx, r, bytes.NewReader(b), "apply", "-f", "-"); err != nil {
		return fmt.Errorf("set repo credential: apply secret %s/repo-helm: %w", argoNS, err)
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
	Status struct {
		Sync struct {
			Status string `json:"status"`
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

// problems lists what keeps the app from Synced+Healthy, one line each.
func (a appStatus) problems() string {
	s := a.Status
	lines := []string{fmt.Sprintf("app sync=%s health=%s", s.Sync.Status, s.Health.Status)}
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

// WaitApp polls the Application until it is Synced and Healthy. On timeout the
// error lists the app's conditions and every resource not Synced+Healthy.
func WaitApp(ctx context.Context, r Runner, name string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var last appStatus
	for {
		out, err := kubectl(ctx, r, nil, "-n", argoNS, "get", "application", name, "-o", "json")
		if err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("wait app %s: not ready within %s (%w):\n  %s", name, timeout, ctx.Err(), last.problems())
			}
			return fmt.Errorf("wait app %s: %w", name, err)
		}
		var a appStatus
		if err := json.Unmarshal(out, &a); err != nil {
			return fmt.Errorf("wait app %s: parse status: %w", name, err)
		}
		last = a
		if a.Status.Sync.Status == "Synced" && a.Status.Health.Status == "Healthy" {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait app %s: not ready within %s (%w):\n  %s", name, timeout, ctx.Err(), last.problems())
		case <-time.After(pollInterval):
		}
	}
}
