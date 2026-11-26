package infra

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"
)

const (
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

// argoWorkloads are the ArgoCD workloads InstallArgo checks and waits for.
var argoWorkloads = []string{"deploy/argocd-server", "deploy/argocd-repo-server", "statefulset/argocd-application-controller"}

// InstallArgo applies the pinned ArgoCD manifest unless every ArgoCD workload
// already runs the pinned image (a partial earlier apply is re-applied), then
// waits for the workloads either way.
func InstallArgo(ctx context.Context, r Runner) (changed bool, err error) {
	pinned := true
	for _, w := range argoWorkloads {
		img, err := kubectl(ctx, r, nil, "-n", argoNS, "get", w, "--ignore-not-found",
			"-o", "jsonpath={.spec.template.spec.containers[0].image}")
		if err != nil {
			return false, fmt.Errorf("install argocd: read %s: %w", w, err)
		}
		if !strings.HasSuffix(strings.TrimSpace(string(img)), ":"+ArgoCDVersion) {
			pinned = false
			break
		}
	}
	if !pinned {
		report(ctx, "applying ArgoCD %s manifest", ArgoCDVersion)
		if err := applyArgo(ctx, r); err != nil {
			return true, err
		}
		changed = true
	}
	// always wait: an earlier run may have applied the manifest and failed here
	for _, w := range argoWorkloads {
		report(ctx, "waiting rollout %s", w)
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
func SetRepoCredential(ctx context.Context, r Runner, repoURL, token string) error {
	if strings.TrimSpace(token) == "" {
		return errors.New("set repo credential: empty token")
	}
	if repoURL == "" {
		return errors.New("set repo credential: empty repo URL")
	}
	b, err := renderAsset("repo-secret.yaml.tmpl", map[string]string{"RepoURL": repoURL, "Token": token})
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

func renderPlatformApp(repoURL, domain, chartRef string) ([]byte, error) {
	if repoURL == "" || domain == "" || chartRef == "" {
		return nil, fmt.Errorf("platform app: repoURL %q, domain %q and chartRef %q are all required", repoURL, domain, chartRef)
	}
	return renderAsset("platform-app.yaml.tmpl", map[string]string{
		"Name": PlatformApp, "RepoURL": repoURL, "ChartRef": chartRef,
		"Domain": domain, "Namespace": PlatformNamespace,
	})
}

// ApplyPlatform creates or updates the local-platform Application at chartRef
// of the helm repo repoURL.
func ApplyPlatform(ctx context.Context, r Runner, repoURL, domain, chartRef string) error {
	b, err := renderPlatformApp(repoURL, domain, chartRef)
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
			Revision   string `json:"revision"`
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
			Phase     string `json:"phase"`
			Message   string `json:"message"`
			StartedAt string `json:"startedAt"`
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
	report(ctx, "waiting application %s Synced/Healthy", name)
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

// getApp reads one Application; a missing one is an error.
func getApp(ctx context.Context, r Runner, name string) (appStatus, error) {
	out, err := kubectl(ctx, r, nil, "-n", argoNS, "get", "application", name, "--ignore-not-found", "-o", "json")
	if err != nil {
		return appStatus{}, fmt.Errorf("read application %s: %w", name, err)
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return appStatus{}, fmt.Errorf("application %s not found", name)
	}
	var a appStatus
	if err := json.Unmarshal(out, &a); err != nil {
		return appStatus{}, fmt.Errorf("read application %s: parse: %w", name, err)
	}
	return a, nil
}

// refreshAnnotation asks the ArgoCD controller to refresh an Application;
// the controller removes it once the refresh is done.
const refreshAnnotation = "argocd.argoproj.io/refresh"

// refreshWait bounds the wait for the controller to finish the refreshes.
var refreshWait = 2 * time.Minute

// RefreshApps hard-refreshes the Applications of names that exist (ArgoCD
// otherwise keeps serving a cached helm commit) with one kubectl call, then
// waits until the controller has finished them (it removes the annotation), so
// a status read afterwards is not the cached one. One that does not exist yet
// is created from the current repo anyway and is left out.
func RefreshApps(ctx context.Context, r Runner, names []string) error {
	apps, err := listApps(ctx, r)
	if err != nil {
		return fmt.Errorf("refresh applications: %w", err)
	}
	have := map[string]bool{}
	for _, it := range apps.Items {
		have[it.Metadata.Name] = true
	}
	var todo []string
	for _, n := range names {
		if have[n] {
			todo = append(todo, n)
		}
	}
	if len(todo) == 0 {
		return nil
	}
	report(ctx, "refreshing %d application(s)", len(todo))
	args := append([]string{"-n", argoNS, "annotate", "application"}, todo...)
	if _, err := kubectl(ctx, r, nil, append(args, refreshAnnotation+"=hard", "--overwrite")...); err != nil {
		return fmt.Errorf("refresh applications %s: %w", strings.Join(todo, ", "), err)
	}
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, refreshWait)
	defer cancel()
	var pending []string
	// a slow refresh is only a warning (the WaitApp that follows guards health);
	// a cancelled run is an error
	slow := func() error {
		if err := parent.Err(); err != nil {
			return fmt.Errorf("refresh applications: cancelled: %w", err)
		}
		report(parent, "refresh not confirmed within %s: %s; continuing", refreshWait, strings.Join(pending, ", "))
		return nil
	}
	for {
		l, err := listApps(ctx, r)
		if err != nil {
			if ctx.Err() != nil {
				return slow()
			}
			return fmt.Errorf("refresh applications: %w", err)
		}
		pending = pending[:0]
		for _, it := range l.Items {
			if _, ok := it.Metadata.Annotations[refreshAnnotation]; ok && slices.Contains(todo, it.Metadata.Name) {
				pending = append(pending, it.Metadata.Name)
			}
		}
		if len(pending) == 0 {
			return nil
		}
		if err := sleepCtx(ctx, pollInterval); err != nil {
			return slow()
		}
	}
}

// SyncedRevision returns Application name's status.sync.revision and whether
// it is Synced and Healthy against its current spec.
func SyncedRevision(ctx context.Context, r Runner, name string) (rev string, synced bool, err error) {
	a, err := getApp(ctx, r, name)
	if err != nil {
		return "", false, fmt.Errorf("read synced revision: %w", err)
	}
	return a.Status.Sync.Revision, a.fresh() && a.Status.Sync.Status == "Synced" && a.Status.Health.Status == "Healthy", nil
}

// SyncApp starts a sync operation on Application name at its target revision
// (auto-sync runs no hooks when nothing in the diff changed), waits for that
// operation, not an earlier one, to Succeed, then for Synced/Healthy, and
// returns the commit it synced (status.sync.revision).
func SyncApp(ctx context.Context, r Runner, name string, timeout time.Duration) (string, error) {
	report(ctx, "syncing application %s", name)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	a, err := getApp(ctx, r, name)
	if err != nil {
		return "", fmt.Errorf("sync app: %w", err)
	}
	var src struct {
		TargetRevision string `json:"targetRevision"`
	}
	if err := json.Unmarshal(a.Spec.Source, &src); err != nil || src.TargetRevision == "" {
		return "", fmt.Errorf("sync app %s: no spec.source.targetRevision (%v)", name, err)
	}
	prior := a.Status.OperationState.StartedAt
	patch, err := json.Marshal(map[string]any{"operation": map[string]any{
		"initiatedBy": map[string]any{"username": "zenify"},
		"sync":        map[string]any{"revision": src.TargetRevision},
	}})
	if err != nil {
		return "", fmt.Errorf("sync app %s: encode patch: %w", name, err)
	}
	if _, err := kubectl(ctx, r, nil, "-n", argoNS, "patch", "application", name, "--type", "merge", "-p", string(patch)); err != nil {
		return "", fmt.Errorf("sync app %s: start operation: %w", name, err)
	}
	for {
		cur, err := getApp(ctx, r, name)
		if err != nil {
			return "", fmt.Errorf("sync app: %w", err)
		}
		op := cur.Status.OperationState
		// an operationState older than the patch (same startedAt) is not ours
		if op.StartedAt != "" && op.StartedAt != prior {
			switch op.Phase {
			case "Succeeded":
				goto done
			case "Failed", "Error":
				return "", fmt.Errorf("sync app %s: operation %s: %s", name, op.Phase, op.Message)
			}
		}
		if err := sleepCtx(ctx, pollInterval); err != nil {
			return "", fmt.Errorf("sync app %s: operation not finished within %s (%w)", name, timeout, err)
		}
	}
done:
	if err := WaitApp(ctx, r, name, timeout); err != nil {
		return "", err
	}
	cur, err := getApp(ctx, r, name)
	if err != nil {
		return "", fmt.Errorf("sync app: %w", err)
	}
	if cur.Status.Sync.Revision == "" {
		return "", fmt.Errorf("sync app %s: status.sync.revision is empty after the sync", name)
	}
	return cur.Status.Sync.Revision, nil
}
