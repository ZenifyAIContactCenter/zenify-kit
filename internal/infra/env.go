package infra

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProfileCore is the env profile with the core apps (be, hub, web) only.
const ProfileCore = "core"

// envApps are the child apps of a core env, in phase order: phase 1 brings
// the datastores and the seed up, phase 2 the apps (the seed runs --if-empty
// and refuses once an app created its collections).
var envApps = []struct {
	name  string
	phase int
}{
	{"mongo", 1}, {"mysql", 1}, {"redis", 1}, {"local-env", 1},
	{"contact-center-be", 2}, {"contact-center-hub", 2}, {"contact-center-web", 2},
}

// SeedRepo is the repo whose image seeds an env's databases.
const SeedRepo = "zenify-data-seed"

var slugRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$`)

// EnvAppName is the root Application of the env slug.
func EnvAppName(slug string) string { return "env-" + slug }

// EnvNamespace is the namespace holding the env's workloads.
func EnvNamespace(slug string) string { return "env-" + slug }

// ChildAppName is the Application app-of-apps renders for app in env slug.
func ChildAppName(app, slug string) string { return app + "-" + slug + "-local" }

// ChildApps lists the child Application names of env slug enabled up to phase.
func ChildApps(slug string, phase int) []string {
	var out []string
	for _, a := range envApps {
		if a.phase <= phase {
			out = append(out, ChildAppName(a.name, slug))
		}
	}
	return out
}

// AppsProbe is the child Application whose presence means phase 2 already ran.
func AppsProbe(slug string) string { return ChildAppName("contact-center-be", slug) }

// EnvURLs are the browser URLs of env slug: web, be, hub.
func EnvURLs(slug, domain string) []string {
	return []string{
		"https://" + Host("web", slug, domain),
		"https://" + Host("be", slug, domain),
		"https://" + Host("hub", slug, domain),
	}
}

func ingressHosts(host, path string) []any {
	return []any{map[string]any{
		"host":  host,
		"paths": []any{map[string]any{"path": path, "pathType": "Prefix"}},
	}}
}

// appValues returns the charts/app values the kit owns for app (helm
// envs/local/<app>.yaml headers): the image tag and the ingress hosts.
func appValues(app, slug, domain string, images map[string]string) (map[string]any, error) {
	if app == "local-env" {
		tag := images[SeedRepo]
		if tag == "" {
			return nil, fmt.Errorf("no image tag for %s", SeedRepo)
		}
		return map[string]any{"slug": slug, "domain": domain, "seed": map[string]any{"tag": tag}}, nil
	}
	var apps map[string]any
	switch app {
	case "contact-center-be":
		apps = map[string]any{"api": map[string]any{"ingress": map[string]any{"hosts": ingressHosts(Host("be", slug, domain), "/")}}}
	case "contact-center-hub":
		hub := Host("hub", slug, domain)
		apps = map[string]any{
			"hub":  map[string]any{"ingress": map[string]any{"hosts": ingressHosts(hub, "/")}},
			"auth": map[string]any{"ingress": map[string]any{"hosts": ingressHosts(hub, "/auth")}},
		}
	case "contact-center-web":
		apps = map[string]any{"contact-center-web": map[string]any{"ingress": map[string]any{"hosts": ingressHosts(Host("web", slug, domain), "/")}}}
	default:
		return nil, nil // datastores take no kit values
	}
	tag := images[app]
	if tag == "" {
		return nil, fmt.Errorf("no image tag for %s", app)
	}
	return map[string]any{"image": map[string]any{"tag": tag}, "apps": apps}, nil
}

// RenderEnvApp renders the root Application env-<slug> (charts/app-of-apps
// over envs/local/map.yaml). images maps a repo name to its image tag. Phase 1
// enables mongo, mysql, redis and local-env only; phase 2 enables the apps too.
func RenderEnvApp(slug, domain, chartRef string, images map[string]string, profile string, phase int) ([]byte, error) {
	if profile != ProfileCore {
		return nil, fmt.Errorf("env app: unknown profile %q (supported: %s)", profile, ProfileCore)
	}
	if phase != 1 && phase != 2 {
		return nil, fmt.Errorf("env app: phase %d, want 1 or 2", phase)
	}
	if !slugRe.MatchString(slug) {
		return nil, fmt.Errorf("env app: invalid slug %q", slug)
	}
	if len(domain) > 253 || !domainRe.MatchString(domain) {
		return nil, fmt.Errorf("env app: invalid domain %q", domain)
	}
	if chartRef == "" {
		return nil, fmt.Errorf("env app: chartRef is required")
	}
	vals := map[string]any{
		"env":    "local",
		"global": map[string]any{"targetRevision": chartRef},
	}
	for _, a := range envApps {
		entry := map[string]any{"name": a.name + "-" + slug, "namespace": EnvNamespace(slug)}
		if a.phase > phase {
			entry["enabled"] = false
		} else {
			v, err := appValues(a.name, slug, domain, images)
			if err != nil {
				return nil, fmt.Errorf("env app: %w", err)
			}
			if v != nil {
				entry["values"] = v
			}
		}
		vals[a.name] = entry
	}
	app := map[string]any{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata": map[string]any{
			"name":       EnvAppName(slug),
			"namespace":  argoNS,
			"finalizers": []any{"resources-finalizer.argocd.argoproj.io"},
		},
		"spec": map[string]any{
			"project": "default",
			"source": map[string]any{
				"repoURL":        HelmRepoURL,
				"path":           "charts/app-of-apps",
				"targetRevision": chartRef,
				"helm": map[string]any{
					"valueFiles":   []any{"../../envs/local/map.yaml"},
					"valuesObject": vals,
				},
			},
			"destination": map[string]any{"server": "https://kubernetes.default.svc", "namespace": argoNS},
			"syncPolicy": map[string]any{
				"automated": map[string]any{"prune": true, "selfHeal": true},
			},
		},
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(app); err != nil {
		return nil, fmt.Errorf("env app: encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("env app: encode: %w", err)
	}
	return buf.Bytes(), nil
}

// Apply runs `kubectl apply -f -` on manifest; changed is false when kubectl
// reports every object unchanged.
func Apply(ctx context.Context, r Runner, manifest []byte) (changed bool, err error) {
	out, err := kubectl(ctx, r, bytes.NewReader(manifest), "apply", "-f", "-")
	if err != nil {
		return false, fmt.Errorf("kubectl apply: %w", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if !strings.HasSuffix(strings.TrimSpace(line), " unchanged") {
			return true, nil
		}
	}
	return false, nil
}

// AppExists reports whether the ArgoCD Application name exists.
func AppExists(ctx context.Context, r Runner, name string) (bool, error) {
	out, err := kubectl(ctx, r, nil, "-n", argoNS, "get", "application", name, "--ignore-not-found", "-o", "name")
	if err != nil {
		return false, fmt.Errorf("get application %s: %w", name, err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}
