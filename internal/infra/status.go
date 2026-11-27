package infra

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// NotInstalled is the status value of a component that is absent.
const NotInstalled = "not installed"

// AppState is an ArgoCD Application's sync and health.
type AppState struct {
	Sync   string `json:"sync"`
	Health string `json:"health"`
}

// EnvStatus is one env's root Application and its URLs.
type EnvStatus struct {
	Slug   string   `json:"slug"`
	Sync   string   `json:"sync"`
	Health string   `json:"health"`
	URLs   []string `json:"urls"`
}

// Report is the `zenify infra status --local` result.
type Report struct {
	K3s      string      `json:"k3s"`
	ArgoCD   string      `json:"argocd"`
	Platform AppState    `json:"platform"`
	Envs     []EnvStatus `json:"envs"`
	DNS      struct {
		Host     string `json:"host"`
		Resolves bool   `json:"resolves"`
	} `json:"dns"`
	CA struct {
		Trusted bool `json:"trusted"`
	} `json:"ca"`
}

type appList struct {
	Items []struct {
		Metadata struct {
			Name        string            `json:"name"`
			Annotations map[string]string `json:"annotations"`
		} `json:"metadata"`
		appStatus
	} `json:"items"`
}

// envDomain is the domain an env root Application was rendered with
// (valuesObject.<seedKey>.values.domain), or fallback when the source holds
// none. A source that does not parse, or spec.sources, is an error.
func (a appStatus) envDomain(seedKey, fallback string) (string, error) {
	if len(a.Spec.Source) == 0 {
		if len(a.Spec.Sources) > 0 && string(a.Spec.Sources) != "null" {
			return "", errors.New("spec.sources (multi-source) is not supported, want spec.source")
		}
		return fallback, nil
	}
	var src struct {
		Helm struct {
			ValuesObject map[string]json.RawMessage `json:"valuesObject"`
		} `json:"helm"`
	}
	if err := json.Unmarshal(a.Spec.Source, &src); err != nil {
		return "", fmt.Errorf("parse spec.source: %w", err)
	}
	raw, ok := src.Helm.ValuesObject[seedKey]
	if !ok {
		return fallback, nil
	}
	var seed struct {
		Values struct {
			Domain string `json:"domain"`
		} `json:"values"`
	}
	if err := json.Unmarshal(raw, &seed); err != nil {
		return "", fmt.Errorf("parse valuesObject.%s: %w", seedKey, err)
	}
	if seed.Values.Domain != "" {
		return seed.Values.Domain, nil
	}
	return fallback, nil
}

// PlatformTarget is what the deployed local-platform Application runs.
type PlatformTarget struct {
	Domain, ChartRef string
}

// LivePlatform reads the deployed local-platform's domain and targetRevision;
// ok is false when k3s, ArgoCD or the Application is absent.
func LivePlatform(ctx context.Context, r Runner) (PlatformTarget, bool, error) {
	k3s, err := installedK3s(ctx, r)
	if err != nil || k3s == "" {
		return PlatformTarget{}, false, err
	}
	argo, err := ArgoInstalledVersion(ctx, r)
	if err != nil || argo == "" {
		return PlatformTarget{}, false, err
	}
	out, err := kubectl(ctx, r, nil, "-n", argoNS, "get", "application", PlatformApp, "--ignore-not-found", "-o", "json")
	if err != nil {
		return PlatformTarget{}, false, fmt.Errorf("read %s: %w", PlatformApp, err)
	}
	if len(strings.TrimSpace(string(out))) == 0 {
		return PlatformTarget{}, false, nil
	}
	var app struct {
		Spec struct {
			Source struct {
				TargetRevision string `json:"targetRevision"`
				Helm           struct {
					ValuesObject struct {
						Domain string `json:"domain"`
					} `json:"valuesObject"`
				} `json:"helm"`
			} `json:"source"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(out, &app); err != nil {
		return PlatformTarget{}, false, fmt.Errorf("read %s: parse: %w", PlatformApp, err)
	}
	return PlatformTarget{Domain: app.Spec.Source.Helm.ValuesObject.Domain, ChartRef: app.Spec.Source.TargetRevision}, true, nil
}

// K3sInstalledVersion returns the installed k3s version, or "" when absent.
func K3sInstalledVersion(ctx context.Context, r Runner) (string, error) {
	return installedK3s(ctx, r)
}

func listApps(ctx context.Context, r Runner) (appList, error) {
	out, err := kubectl(ctx, r, nil, "-n", argoNS, "get", "applications", "-o", "json")
	if err != nil {
		return appList{}, fmt.Errorf("list applications: %w", err)
	}
	var l appList
	if err := json.Unmarshal(out, &l); err != nil {
		return appList{}, fmt.Errorf("list applications: parse: %w", err)
	}
	return l, nil
}

// ArgoInstalledVersion returns the argocd-server image tag, or "" when absent.
func ArgoInstalledVersion(ctx context.Context, r Runner) (string, error) {
	img, err := kubectl(ctx, r, nil, "-n", argoNS, "get", "deploy", "argocd-server", "--ignore-not-found",
		"-o", "jsonpath={.spec.template.spec.containers[0].image}")
	if err != nil {
		return "", fmt.Errorf("read argocd version: %w", err)
	}
	s := strings.TrimSpace(string(img))
	if i := strings.LastIndex(s, ":"); i >= 0 {
		return s[i+1:], nil
	}
	return s, nil
}

// Status reads the devbox: k3s, ArgoCD, the platform and env Applications, a
// real DNS lookup of the first env's first app host (on that env's own
// domain) and the CA trust.
func Status(ctx context.Context, r Runner, w Workspace, home, domain string) (Report, error) {
	rep := Report{K3s: NotInstalled, ArgoCD: NotInstalled, Platform: AppState{"Missing", "Missing"}, Envs: []EnvStatus{}}
	k3s, err := installedK3s(ctx, r)
	if err != nil {
		return rep, fmt.Errorf("status: %w", err)
	}
	if k3s != "" {
		rep.K3s = k3s
		argo, err := ArgoInstalledVersion(ctx, r)
		if err != nil {
			return rep, fmt.Errorf("status: %w", err)
		}
		if argo != "" {
			rep.ArgoCD = argo
			apps, err := listApps(ctx, r)
			if err != nil {
				return rep, fmt.Errorf("status: %w", err)
			}
			for _, it := range apps.Items {
				st := AppState{it.Status.Sync.Status, it.Status.Health.Status}
				switch name := it.Metadata.Name; {
				case name == PlatformApp:
					rep.Platform = st
				case strings.HasPrefix(name, "env-"):
					slug := strings.TrimPrefix(name, "env-")
					envDomain, err := it.envDomain(w.seedApp().Key, domain)
					if err != nil {
						return rep, fmt.Errorf("status: application %s: env domain unreadable: %w", name, err)
					}
					rep.Envs = append(rep.Envs, EnvStatus{Slug: slug, Sync: st.Sync, Health: st.Health, URLs: EnvURLs(w, slug, envDomain)})
				}
			}
		}
	}
	svcs := w.svcApps()
	switch {
	case len(rep.Envs) > 0 && len(rep.Envs[0].URLs) > 0:
		rep.DNS.Host = strings.TrimPrefix(rep.Envs[0].URLs[0], "https://")
	case len(svcs) > 0:
		rep.DNS.Host = Host(svcs[0].Svc, BaseSlug, domain)
	default:
		return rep, fmt.Errorf("status: %s has no env app with svc to probe DNS with", WorkspaceFile)
	}
	if rep.DNS.Resolves, err = DNSResolves(ctx, r, rep.DNS.Host); err != nil {
		return rep, fmt.Errorf("status: %w", err)
	}
	if rep.CA.Trusted, err = CATrusted(ctx, r, home); err != nil {
		return rep, fmt.Errorf("status: %w", err)
	}
	return rep, nil
}

// EnvApps lists the root env Applications (env-<slug>).
func EnvApps(ctx context.Context, r Runner) ([]string, error) {
	apps, err := listApps(ctx, r)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, it := range apps.Items {
		if strings.HasPrefix(it.Metadata.Name, "env-") {
			out = append(out, it.Metadata.Name)
		}
	}
	return out, nil
}

// DeleteApp deletes the Application name and waits until its finalizer has
// removed what it deployed. cascade adds the resources finalizer first, for an
// Application created without one (local-platform).
func DeleteApp(ctx context.Context, r Runner, name string, cascade bool) error {
	if cascade {
		patch := `{"metadata":{"finalizers":["resources-finalizer.argocd.argoproj.io"]}}`
		if _, err := kubectl(ctx, r, nil, "-n", argoNS, "patch", "application", name, "--type", "merge", "-p", patch); err != nil {
			return fmt.Errorf("delete application %s: add finalizer: %w", name, err)
		}
	}
	if _, err := kubectl(ctx, r, nil, "-n", argoNS, "delete", "application", name, "--ignore-not-found", "--wait=true", "--timeout=10m"); err != nil {
		return fmt.Errorf("delete application %s: %w", name, err)
	}
	return nil
}

// DeleteNamespace deletes ns and waits for it to go.
func DeleteNamespace(ctx context.Context, r Runner, ns string) error {
	if _, err := kubectl(ctx, r, nil, "delete", "namespace", ns, "--ignore-not-found", "--wait=true", "--timeout=10m"); err != nil {
		return fmt.Errorf("delete namespace %s: %w", ns, err)
	}
	return nil
}

const (
	k3sUninstall   = "/usr/local/bin/k3s-uninstall.sh"
	k3sServiceUnit = "/etc/systemd/system/k3s.service"
	// buildkitStateDir is the buildkitd cache hostPath (helm envs/local-platform
	// values.yaml buildkitd hostPath); k3s-uninstall.sh does not remove it
	buildkitStateDir = "/var/lib/zenify/buildkit"
)

// k3sGone is nil when neither the k3s binary nor its systemd unit is left.
func k3sGone(ctx context.Context, r Runner) error {
	ver, err := installedK3s(ctx, r)
	if err != nil {
		return err
	}
	_, unit, err := readRootFile(ctx, r, k3sServiceUnit)
	if err != nil {
		return err
	}
	if ver != "" || unit {
		return fmt.Errorf("k3s is still installed (binary %q, unit %s present=%v) but %s is missing: remove k3s by hand, then rerun", ver, k3sServiceUnit, unit, k3sUninstall)
	}
	return nil
}

// UninstallK3s runs k3s-uninstall.sh when present, then removes the API
// firewall unit (after k3s, so its ports are never open while k3s runs).
func UninstallK3s(ctx context.Context, r Runner) error {
	if _, exists, err := readRootFile(ctx, r, k3sUninstall); err != nil {
		return fmt.Errorf("uninstall k3s: %w", err)
	} else if exists {
		if _, err := r.Run(ctx, nil, "sudo", k3sUninstall); err != nil {
			return fmt.Errorf("uninstall k3s: %w", err)
		}
	} else if err := k3sGone(ctx, r); err != nil {
		// keep the firewall (and the caller keeps state) while k3s can still start
		return fmt.Errorf("uninstall k3s: %w", err)
	}
	if _, exists, err := readRootFile(ctx, r, unitPath); err != nil {
		return fmt.Errorf("remove firewall: %w", err)
	} else if exists {
		if _, err := r.Run(ctx, nil, "sudo", "systemctl", "disable", "--now", firewallUnit); err != nil {
			return fmt.Errorf("remove firewall: disable: %w", err)
		}
	}
	if _, err := r.Run(ctx, nil, "sudo", "rm", "-f", unitPath, nftPath, fwMarker); err != nil {
		return fmt.Errorf("remove firewall: rm: %w", err)
	}
	if _, err := r.Run(ctx, nil, "sudo", "rm", "-rf", buildkitStateDir); err != nil {
		return fmt.Errorf("remove build cache %s: %w", buildkitStateDir, err)
	}
	if _, err := r.Run(ctx, nil, "sudo", "systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("remove firewall: daemon-reload: %w", err)
	}
	return nil
}
