package infra

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var slugRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$`)

// EnvAppName is the root Application of the env slug.
func EnvAppName(slug string) string { return "env-" + slug }

// EnvNamespace is the namespace holding the env's workloads.
func EnvNamespace(slug string) string { return "env-" + slug }

// ChildAppName is the Application app-of-apps renders for app in env slug.
func ChildAppName(app, slug string) string { return app + "-" + slug + "-local" }

// ChildApps lists the child Application names of env slug enabled up to phase.
func ChildApps(w Workspace, slug string, phase int) []string {
	var out []string
	for _, a := range w.EnvApps {
		if a.Phase <= phase {
			out = append(out, ChildAppName(a.Key, slug))
		}
	}
	return out
}

// PhaseTwoApps are the child Applications that exist only once phase 2 ran.
func PhaseTwoApps(w Workspace, slug string) []string {
	var out []string
	for _, a := range w.EnvApps {
		if a.Phase == 2 {
			out = append(out, ChildAppName(a.Key, slug))
		}
	}
	return out
}

// EnvURLs are the browser URLs of env slug, one per app with a host prefix.
func EnvURLs(w Workspace, slug, domain string) []string {
	var out []string
	for _, a := range w.svcApps() {
		out = append(out, "https://"+Host(a.Svc, slug, domain))
	}
	return out
}

// setDotted sets m[k1][k2]...=v for key "k1.k2...", creating maps on the way.
func setDotted(m map[string]any, key string, v any) error {
	parts := strings.Split(key, ".")
	for i, p := range parts[:len(parts)-1] {
		next, ok := m[p]
		if !ok {
			nm := map[string]any{}
			m[p] = nm
			m = nm
			continue
		}
		nm, ok := next.(map[string]any)
		if !ok {
			return fmt.Errorf("values key %s: %s is not a map", key, strings.Join(parts[:i+1], "."))
		}
		m = nm
	}
	last := parts[len(parts)-1]
	if _, dup := m[last]; dup {
		return fmt.Errorf("values key %s set twice", key)
	}
	m[last] = v
	return nil
}

// appValues returns the values the kit owns for app a: for the seed app its
// slug, domain and seed.tag; for an app with an image its image.tag and the
// ingress hosts lists; nothing for a datastore.
func appValues(w Workspace, a WorkspaceApp, slug, domain string, images map[string]string) (map[string]any, error) {
	if a.Seed {
		tag := images[w.Seed.Repo]
		if tag == "" {
			return nil, fmt.Errorf("no image tag for %s", w.Seed.Repo)
		}
		return map[string]any{"slug": slug, "domain": domain, "seed": map[string]any{"tag": tag}}, nil
	}
	if a.Repo == "" {
		return nil, nil
	}
	tag := images[a.Repo]
	if tag == "" {
		return nil, fmt.Errorf("no image tag for %s", a.Repo)
	}
	v := map[string]any{"image": map[string]any{"tag": tag}}
	for _, in := range a.Ingress {
		hosts := []any{map[string]any{
			"host":  Host(a.Svc, slug, domain),
			"paths": []any{map[string]any{"path": in.Path, "pathType": "Prefix"}},
		}}
		if err := setDotted(v, in.Values, hosts); err != nil {
			return nil, fmt.Errorf("%s: %w", a.Key, err)
		}
	}
	return v, nil
}

// RenderEnvApp renders the root Application env-<slug> (charts/app-of-apps
// over envs/local/map.yaml). images maps a build repo to its image tag. Phase 1
// enables only the phase-1 apps (datastores and the seed); phase 2 all.
func RenderEnvApp(w Workspace, slug, domain, chartRef string, images map[string]string, phase int) ([]byte, error) {
	if phase != 1 && phase != 2 {
		return nil, fmt.Errorf("env app: phase %d, want 1 or 2", phase)
	}
	if !slugRe.MatchString(slug) {
		return nil, fmt.Errorf("env app: invalid slug %q", slug)
	}
	if err := validDomainName(domain); err != nil {
		return nil, fmt.Errorf("env app: %w", err)
	}
	if chartRef == "" {
		return nil, fmt.Errorf("env app: chartRef is required")
	}
	vals := map[string]any{
		"env":    "local",
		"global": map[string]any{"targetRevision": chartRef},
	}
	for _, a := range w.EnvApps {
		entry := map[string]any{"name": a.Key + "-" + slug, "namespace": EnvNamespace(slug)}
		if a.Phase > phase {
			entry["enabled"] = false
		} else {
			v, err := appValues(w, a, slug, domain, images)
			if err != nil {
				return nil, fmt.Errorf("env app: %w", err)
			}
			if v != nil {
				entry["values"] = v
			}
		}
		vals[a.Key] = entry
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
				"repoURL":        w.HelmRepoURL,
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

// EnvCAConfigMap is the ConfigMap (key ca.crt) the env charts mount to trust
// the local CA.
const EnvCAConfigMap = "local-ca"

// ApplyEnvCA creates or updates namespace env-<slug> and ConfigMap local-ca
// (key ca.crt) holding the local CA's public PEM, by server-side apply.
func ApplyEnvCA(ctx context.Context, r Runner, slug string, caPEM []byte) error {
	if !slugRe.MatchString(slug) {
		return fmt.Errorf("env ca: invalid slug %q", slug)
	}
	crt, err := validateCA(caPEM)
	if err != nil {
		return fmt.Errorf("env ca: %w", err)
	}
	ns := EnvNamespace(slug)
	docs := []map[string]any{
		{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": ns}},
		{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": EnvCAConfigMap, "namespace": ns},
			"data": map[string]any{caSecretKey: string(crt)}},
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	for _, d := range docs {
		if err := enc.Encode(d); err != nil {
			return fmt.Errorf("env ca: encode: %w", err)
		}
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("env ca: encode: %w", err)
	}
	if _, err := kubectl(ctx, r, &buf, "apply", "--server-side", "--field-manager=zenify", "-f", "-"); err != nil {
		return fmt.Errorf("env ca: apply %s/%s: %w", ns, EnvCAConfigMap, err)
	}
	return nil
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
