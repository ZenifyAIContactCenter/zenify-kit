package infra

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
)

// WorkspaceFile is the name of the workspace's infra truth under
// <knowledge store>/.config: repos, images, env apps and seed accounts are
// read from it at runtime, never compiled into the public binary.
const WorkspaceFile = "infra.json"

// Workspace is the workspace-specific part of `zenify infra`.
type Workspace struct {
	// HelmRepoURL is the git repo holding charts/local-platform, charts/app-of-apps and envs/.
	HelmRepoURL string `json:"helmRepoURL"`
	// Builds are the images of an env, in build order (a base image before its users).
	Builds []WorkspaceBuild `json:"builds"`
	// EnvApps are the app-of-apps keys of an env, in phase order.
	EnvApps []WorkspaceApp `json:"envApps"`
	// Seed describes the seed data restored into every env.
	Seed WorkspaceSeed `json:"seed"`
}

// WorkspaceBuild is one image built from a repo's own Dockerfile.
type WorkspaceBuild struct {
	Repo       string `json:"repo"`       // repo name under <workspace>/repos
	Context    string `json:"context"`    // build context relative to the repo root
	Dockerfile string `json:"dockerfile"` // file name inside the context
	Target     string `json:"target,omitempty"`
	Image      string `json:"image"`         // image path under the local registry
	Tag        string `json:"tag,omitempty"` // fixed tag; empty = sha-<commit>
	// Args are fixed build args.
	Args map[string]string `json:"args,omitempty"`
	// EnvArgs are build args that differ per env, as arg -> variable. Variables:
	// "slug", "domain", "registry", "baseOnly" ("1" for the base env, else "0").
	// They are baked into the image, so they enter its tag.
	EnvArgs map[string]string `json:"envArgs,omitempty"`
	// Secrets must be set (env or the kit env store); OptionalSecrets are passed only when set.
	Secrets         []string `json:"secrets,omitempty"`
	OptionalSecrets []string `json:"optionalSecrets,omitempty"`
}

// WorkspaceApp is one app-of-apps key of an env.
type WorkspaceApp struct {
	Key   string `json:"key"`   // app-of-apps key; the child Application is <key>-<slug>-local
	Phase int    `json:"phase"` // 1 = datastores and seed, 2 = apps
	// Repo is the build whose tag goes to image.tag (empty for a datastore).
	Repo string `json:"repo,omitempty"`
	// Svc is the host prefix (<svc>.<domain> for base, <svc>-<slug>.<domain> otherwise).
	Svc string `json:"svc,omitempty"`
	// Ingress lists the values keys that receive [{host, paths:[{path}]}].
	Ingress []WorkspaceIngress `json:"ingress,omitempty"`
	// Seed marks the app that receives values slug, domain and seed.tag.
	Seed bool `json:"seed,omitempty"`
}

// WorkspaceIngress is one ingress hosts list in an app's values.
type WorkspaceIngress struct {
	Values string `json:"values"` // dotted values key, e.g. apps.api.ingress.hosts
	Path   string `json:"path"`   // URL path prefix
}

// WorkspaceSeed is the seed repo and its logins; passwords are never stored.
type WorkspaceSeed struct {
	Repo         string        `json:"repo"`
	Tenant       string        `json:"tenant"`
	PasswordHint string        `json:"passwordHint"`
	Accounts     []SeedAccount `json:"accounts"`
}

var envArgVars = []string{"slug", "domain", "registry", "baseOnly"}

// LoadWorkspace reads and validates path; a missing file is an error naming
// the file and the fields it must hold (there are no compiled-in defaults).
func LoadWorkspace(path string) (Workspace, error) {
	b, err := os.ReadFile(path) //nolint:gosec // G304 -- path is the kit's store config file
	if errors.Is(err, os.ErrNotExist) {
		return Workspace{}, fmt.Errorf("infra: %s not found: create it with helmRepoURL, builds, envApps and seed (schema: the zenify infra reference page)", path)
	}
	if err != nil {
		return Workspace{}, fmt.Errorf("infra: read %s: %w", path, err)
	}
	var w Workspace
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&w); err != nil {
		return Workspace{}, fmt.Errorf("infra: parse %s: %w", path, err)
	}
	if err := w.validate(); err != nil {
		return Workspace{}, fmt.Errorf("infra: %s: %w", path, err)
	}
	return w, nil
}

func (w Workspace) validate() error {
	var miss []string
	need := func(ok bool, field string) {
		if !ok {
			miss = append(miss, field)
		}
	}
	need(w.HelmRepoURL != "", "helmRepoURL")
	need(len(w.Builds) > 0, "builds")
	need(len(w.EnvApps) > 0, "envApps")
	need(w.Seed.Repo != "", "seed.repo")
	need(w.Seed.Tenant != "", "seed.tenant")
	need(w.Seed.PasswordHint != "", "seed.passwordHint")
	need(len(w.Seed.Accounts) > 0, "seed.accounts")
	builds := map[string]bool{}
	for i, b := range w.Builds {
		p := fmt.Sprintf("builds[%d].", i)
		need(b.Repo != "", p+"repo")
		need(b.Context != "", p+"context")
		need(b.Dockerfile != "", p+"dockerfile")
		need(b.Image != "", p+"image")
		for arg, v := range b.EnvArgs {
			need(slices.Contains(envArgVars, v), p+"envArgs."+arg+" (one of "+strings.Join(envArgVars, ", ")+")")
		}
		builds[b.Repo] = true
	}
	seeds := 0
	for i, a := range w.EnvApps {
		p := fmt.Sprintf("envApps[%d].", i)
		need(a.Key != "" && !strings.HasPrefix(a.Key, "env-"), p+"key (not starting with env-, the prefix of env root Applications)")
		need(a.Phase == 1 || a.Phase == 2, p+"phase (1 or 2)")
		need(a.Repo == "" || builds[a.Repo], p+"repo (must be a builds[].repo)")
		need(a.Svc == "" || a.Repo != "", p+"repo (an app with svc needs an image)")
		need(len(a.Ingress) == 0 || a.Svc != "", p+"svc (ingress needs a host prefix)")
		for j, in := range a.Ingress {
			need(in.Values != "" && in.Path != "", fmt.Sprintf("%singress[%d].values/path", p, j))
		}
		if a.Seed {
			seeds++
		}
	}
	need(seeds == 1, "envApps[].seed (exactly one app)")
	need(w.Seed.Repo == "" || builds[w.Seed.Repo], "seed.repo (must be a builds[].repo)")
	for i, a := range w.Seed.Accounts {
		need(a.Role != "" && a.Email != "", fmt.Sprintf("seed.accounts[%d].role/email", i))
	}
	if len(miss) > 0 {
		return fmt.Errorf("missing or invalid fields: %s", strings.Join(miss, ", "))
	}
	return nil
}

// Repos lists the build repos in build order.
func (w Workspace) Repos() []string {
	out := make([]string, 0, len(w.Builds))
	for _, b := range w.Builds {
		out = append(out, b.Repo)
	}
	return out
}

// Accounts returns the seed accounts with the tenant filled in.
func (w Workspace) Accounts() []SeedAccount {
	out := make([]SeedAccount, 0, len(w.Seed.Accounts))
	for _, a := range w.Seed.Accounts {
		a.Tenant = w.Seed.Tenant
		out = append(out, a)
	}
	return out
}

// seedApp is the env app marked seed (validate guarantees exactly one).
func (w Workspace) seedApp() WorkspaceApp {
	for _, a := range w.EnvApps {
		if a.Seed {
			return a
		}
	}
	return WorkspaceApp{}
}

// svcApps are the env apps with a host prefix, in file order.
func (w Workspace) svcApps() []WorkspaceApp {
	var out []WorkspaceApp
	for _, a := range w.EnvApps {
		if a.Svc != "" {
			out = append(out, a)
		}
	}
	return out
}

// envArgValue resolves an EnvArgs variable for env slug.
func envArgValue(v, slug, domain string) string {
	switch v {
	case "slug":
		return slug
	case "domain":
		return domain
	case "registry":
		return RegistryHost
	case "baseOnly":
		if slug == BaseSlug {
			return "1"
		}
		return "0"
	}
	return ""
}

func sortedKeys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
