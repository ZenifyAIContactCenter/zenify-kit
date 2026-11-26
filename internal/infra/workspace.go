package infra

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
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

// infra.json lives in a team-shared store: every value is validated before it
// reaches a token, a path or a command line.
var (
	repoNameRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	imagePathRe = regexp.MustCompile(`^[a-z0-9]+([._-][a-z0-9]+)*(/[a-z0-9]+([._-][a-z0-9]+)*)*$`)
	imageTagRe  = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)
	secretIDRe  = regexp.MustCompile(`^[a-z0-9_]+$`)
	dnsLabelRe  = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
)

// reservedAppKeys are top-level app-of-apps values, not apps: an app with
// such a key would overwrite env or global.targetRevision.
var reservedAppKeys = []string{"env", "environment", "global", "apps"}

// maxSlugLen is the longest env slug slugRe allows; an app host label is
// <svc>-<slug>, so svc (and key, for <key>-<slug>) gets 63-1-maxSlugLen bytes.
const (
	maxSlugLen  = 32
	maxLabelLen = 63 - 1 - maxSlugLen
)

// helmRepoOK: the user's GitHub token is sent to this URL, so only an https
// github.com URL without embedded credentials is accepted.
func helmRepoOK(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == "github.com" && u.User == nil && u.Path != "" && u.Path != "/"
}

func labelOK(s string) bool { return len(s) <= maxLabelLen && dnsLabelRe.MatchString(s) }

func (w Workspace) validate() error {
	var miss []string
	need := func(ok bool, field string) {
		if !ok {
			miss = append(miss, field)
		}
	}
	need(helmRepoOK(w.HelmRepoURL), "helmRepoURL (an https://github.com/<owner>/<repo> URL without credentials)")
	need(len(w.Builds) > 0, "builds")
	need(len(w.EnvApps) > 0, "envApps")
	need(w.Seed.Repo != "", "seed.repo")
	need(w.Seed.Tenant != "", "seed.tenant")
	need(w.Seed.PasswordHint != "", "seed.passwordHint")
	need(len(w.Seed.Accounts) > 0, "seed.accounts")
	builds, images := map[string]bool{}, map[string]bool{}
	for i, b := range w.Builds {
		p := fmt.Sprintf("builds[%d].", i)
		need(repoNameRe.MatchString(b.Repo) && !strings.Contains(b.Repo, ".."), p+"repo (^[a-z0-9][a-z0-9._-]*$, no ..)")
		need(!builds[b.Repo], p+"repo (duplicate)")
		need(b.Context != "" && filepath.IsLocal(b.Context), p+"context (a relative path inside the repo)")
		need(b.Dockerfile != "" && filepath.IsLocal(b.Dockerfile), p+"dockerfile (a relative path inside the context)")
		need(imagePathRe.MatchString(b.Image), p+"image (registry path: lower-case [a-z0-9] parts joined by . _ - /)")
		need(b.Tag == "" || imageTagRe.MatchString(b.Tag), p+"tag (^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$)")
		// a fixed tag is reused as is, so it cannot carry per-env args
		need(b.Tag == "" || len(b.EnvArgs) == 0, p+"tag (a fixed tag cannot be combined with envArgs)")
		need(!images[b.Image], p+"image (duplicate: two builds would share tags)")
		images[b.Image] = true
		for _, id := range append(slices.Clone(b.Secrets), b.OptionalSecrets...) {
			need(secretIDRe.MatchString(id), p+"secrets/optionalSecrets "+strconv.Quote(id)+" (^[a-z0-9_]+$)")
		}
		for arg, v := range b.EnvArgs {
			need(slices.Contains(envArgVars, v), p+"envArgs."+arg+" (one of "+strings.Join(envArgVars, ", ")+")")
		}
		builds[b.Repo] = true
	}
	seeds := 0
	keys := map[string]bool{}
	for i, a := range w.EnvApps {
		p := fmt.Sprintf("envApps[%d].", i)
		need(labelOK(a.Key) && !strings.HasPrefix(a.Key, "env-"), fmt.Sprintf("%skey (DNS label of at most %d bytes, not starting with env-)", p, maxLabelLen))
		need(!keys[a.Key], p+"key (duplicate)")
		need(!slices.Contains(reservedAppKeys, a.Key), p+"key ("+strings.Join(reservedAppKeys, ", ")+" are app-of-apps values, not apps)")
		keys[a.Key] = true
		need(a.Phase == 1 || a.Phase == 2, p+"phase (1 or 2)")
		need(a.Repo == "" || builds[a.Repo], p+"repo (must be a builds[].repo)")
		need(a.Svc == "" || a.Repo != "", p+"repo (an app with svc needs an image)")
		need(a.Svc == "" || labelOK(a.Svc), fmt.Sprintf("%ssvc (DNS label of at most %d bytes)", p, maxLabelLen))
		need(len(a.Ingress) == 0 || a.Svc != "", p+"svc (ingress needs a host prefix)")
		for j, in := range a.Ingress {
			need(in.Values != "" && strings.HasPrefix(in.Path, "/"), fmt.Sprintf("%singress[%d].values/path (path starts with /)", p, j))
			// image.tag is the kit's own key in the same values map
			need(in.Values != "image" && !strings.HasPrefix(in.Values, "image."), fmt.Sprintf("%singress[%d].values (image is the kit's image.tag key)", p, j))
		}
		if a.Seed {
			seeds++
			need(a.Phase == 1, p+"phase (the seed app runs in phase 1, before the apps)")
			need(a.Repo == "" && a.Svc == "" && len(a.Ingress) == 0, p+"seed (the seed app takes no repo, svc or ingress)")
		}
		// an app built by the kit starts after the seed, never with it
		need(a.Repo == "" || a.Phase == 2, p+"phase (an app with repo runs in phase 2)")
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
