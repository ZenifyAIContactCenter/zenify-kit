package infra

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/devenv"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/wt"
)

// BaseSpaTag is the fixed tag of the organisation's SPA base image; web's
// Dockerfile defaults BASE_TAG to it.
const BaseSpaTag = "1.30"

// BuildTarget is one image built from a repo's own Dockerfile.
type BuildTarget struct {
	Repo       string            // repo name under <ws>/repos
	Dir        string            // build context: relative to the repo root in CoreTargets, absolute when built
	Dockerfile string            // file name inside Dir
	Target     string            // Dockerfile stage; empty = last stage
	Image      string            // ref without tag, under RegistryHost
	Tag        string            // fixed tag; empty = derived from the commit (ImageTag)
	Args       map[string]string // build args
	Secrets    []string          // BuildKit secret ids; each is read from env key strings.ToUpper(id)
	EnvArgs    []string          // keys of Args that differ per env; they are baked into the image, so they enter its tag
}

// ImageTag is sha-<7>, plus -dirty-<8 hex of sha256(diff)> for a dirty worktree.
func ImageTag(sha string, dirtyDiff []byte) string {
	short := sha
	if len(short) > 7 {
		short = short[:7]
	}
	if len(dirtyDiff) == 0 {
		return "sha-" + short
	}
	sum := sha256.Sum256(dirtyDiff)
	return "sha-" + short + "-dirty-" + hex.EncodeToString(sum[:])[:8]
}

// VariantTag appends -v<8 hex of sha256 of the sorted k=v pairs of t.EnvArgs>
// to tag, so the same commit built for another env gets its own image. A
// target without EnvArgs keeps tag.
func VariantTag(tag string, t BuildTarget) string {
	if len(t.EnvArgs) == 0 {
		return tag
	}
	pairs := make([]string, 0, len(t.EnvArgs))
	for _, k := range t.EnvArgs {
		pairs = append(pairs, k+"="+t.Args[k])
	}
	sort.Strings(pairs)
	sum := sha256.Sum256([]byte(strings.Join(pairs, "\n")))
	return tag + "-v" + hex.EncodeToString(sum[:])[:8]
}

// CoreTargets returns the images of a base env in build order: base-spa before
// web (web's runtime stage is FROM base-spa). No ref points at repo.zenify.vn,
// which needs a login.
func CoreTargets(ws, domain, slug string) ([]BuildTarget, error) {
	if domain == "" || slug == "" {
		return nil, fmt.Errorf("build targets: domain %q and slug %q are both required", domain, slug)
	}
	ts := coreTargets(domain, slug)
	for _, t := range ts {
		if _, err := os.Stat(filepath.Join(ws, "repos", t.Repo)); err != nil { //nolint:gosec // G703 -- t.Repo is a literal from coreTargets, ws is the kit's own workspace root
			return nil, fmt.Errorf("build targets: repo %s: %w", t.Repo, err)
		}
	}
	return ts, nil
}

// CoreRepos lists the repos CoreTargets builds, in build order.
func CoreRepos() []string {
	var out []string
	for _, t := range coreTargets("x.test", BaseSlug) {
		out = append(out, t.Repo)
	}
	return out
}

func coreTargets(domain, slug string) []BuildTarget {
	return []BuildTarget{
		{Repo: "zenify-ci", Dir: "stacks/spa", Dockerfile: "base.Dockerfile", Image: RegistryHost + "/zenify/base-spa", Tag: BaseSpaTag,
			Args: map[string]string{"VERSION": BaseSpaTag}},
		{Repo: "contact-center-be", Dir: ".", Dockerfile: "Dockerfile", Target: "api", Image: RegistryHost + "/contact-center-be/api",
			Secrets: []string{"npm_token"}},
		{Repo: "contact-center-hub", Dir: ".", Dockerfile: "Dockerfile", Image: RegistryHost + "/contact-center-hub",
			Secrets: []string{"npm_token", "npm_taskforcesh_token"}},
		{Repo: "contact-center-web", Dir: ".", Dockerfile: "Dockerfile", Image: RegistryHost + "/contact-center-web",
			Args:    map[string]string{"BUILD_MODE": "preview", "PREVIEW_SLUG": slug, "PREVIEW_DOMAIN": domain, "REGISTRY": RegistryHost, "BASE_TAG": BaseSpaTag},
			EnvArgs: []string{"PREVIEW_SLUG", "PREVIEW_DOMAIN", "REGISTRY"},
			Secrets: []string{"npm_token"}},
		{Repo: "zenify-data-seed", Dir: ".", Dockerfile: "Dockerfile", Image: RegistryHost + "/zenify-data-seed"},
	}
}

// BuildResult reports one target of BuildFromBase.
type BuildResult struct {
	Image, Tag string
	Built      bool
	Duration   time.Duration
}

// BuildFromBase builds t from its repo's base ref (refs[repo] overrides the
// baseRef of the repo's .claude/worktree.json) in a temporary worktree that is
// removed afterwards, also on error.
func BuildFromBase(ctx context.Context, r BuildRunner, ws string, t BuildTarget, refs map[string]string) (BuildResult, error) {
	start := time.Now()
	dir, sha, cleanup, err := BaseSource(ctx, r, ws, t.Repo, refs[t.Repo])
	if err != nil {
		return BuildResult{}, err
	}
	tag := t.Tag
	if tag == "" {
		tag = VariantTag(ImageTag(sha, nil), t)
	}
	t.Dir = filepath.Join(dir, t.Dir)
	built, err := Build(ctx, r, ws, t, tag)
	if cerr := cleanup(); cerr != nil {
		err = errors.Join(err, cerr)
	}
	return BuildResult{Image: t.Image, Tag: tag, Built: built, Duration: time.Since(start)}, err
}

// BaseSource fetches repo and checks out ref (empty = the repo's baseRef) as a
// detached temporary worktree under ~/.zenify/infra/src. cleanup removes that
// worktree only; the user's checkouts and worktrees are never touched.
func BaseSource(ctx context.Context, r Runner, ws, repo, ref string) (dir, sha string, cleanup func() error, err error) {
	root := filepath.Join(ws, "repos", repo)
	if ref == "" {
		cfg, err := wt.Load(root)
		if err != nil {
			return "", "", nil, fmt.Errorf("base source %s: %w", repo, err)
		}
		ref = cfg.BaseRef
	}
	if strings.HasPrefix(ref, "-") {
		return "", "", nil, fmt.Errorf("base source %s: ref %q must not start with '-'", repo, ref)
	}
	if _, err := r.Run(ctx, nil, "git", "-C", root, "fetch", "origin"); err != nil {
		return "", "", nil, fmt.Errorf("base source %s: fetch: %w", repo, err)
	}
	out, err := r.Run(ctx, nil, "git", "-C", root, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", "", nil, fmt.Errorf("base source %s: resolve %s: %w", repo, ref, err)
	}
	sha = strings.TrimSpace(string(out))
	if len(sha) < 7 || strings.Trim(sha, "0123456789abcdef") != "" {
		return "", "", nil, fmt.Errorf("base source %s: %s resolved to %q", repo, ref, sha)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", nil, fmt.Errorf("base source %s: %w", repo, err)
	}
	dir = filepath.Join(infraDir(home), "src", repo+"-"+sha[:7])
	remove := func() error {
		// cleanup must run even when ctx was cancelled by the failure it cleans up after
		if _, err := r.Run(context.WithoutCancel(ctx), nil, "git", "-C", root, "worktree", "remove", "--force", dir); err != nil {
			return fmt.Errorf("base source %s: remove temp worktree %s: %w", repo, dir, err)
		}
		return nil
	}
	// drop admin entries of temp worktrees whose dir is gone, so add below does not trip on one
	if _, err := r.Run(ctx, nil, "git", "-C", root, "worktree", "prune"); err != nil {
		return "", "", nil, fmt.Errorf("base source %s: worktree prune: %w", repo, err)
	}
	if _, err := os.Stat(dir); err == nil { //nolint:gosec // G703 -- dir is the kit's src dir + a repo literal + rev-parse hex (checked above); left by an interrupted run
		if err := remove(); err != nil {
			return "", "", nil, err
		}
	}
	if _, err := r.Run(ctx, nil, "git", "-C", root, "worktree", "add", "--detach", dir, sha); err != nil {
		return "", "", nil, fmt.Errorf("base source %s: add temp worktree %s: %w", repo, dir, err)
	}
	return dir, sha, remove, nil
}

// registryURL is the registry as the host reaches it: its ClusterIP directly.
var registryURL = "http://" + RegistryClusterIP + ":5000"

// waitPort waits until addr accepts TCP connections.
var waitPort = func(ctx context.Context, addr string) error {
	deadline := time.Now().Add(30 * time.Second)
	for {
		c, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "tcp", addr)
		if err == nil {
			return c.Close()
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("port-forward %s not ready: %w", addr, err)
		}
		if err := sleepCtx(ctx, 200*time.Millisecond); err != nil {
			return err
		}
	}
}

// tagExists asks the registry for image:tag's manifest.
func tagExists(ctx context.Context, image, tag string) (bool, error) {
	repo := strings.TrimPrefix(image, RegistryHost+"/")
	u := registryURL + "/v2/" + repo + "/manifests/" + tag
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", strings.Join([]string{
		"application/vnd.oci.image.index.v1+json",
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}, ", "))
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return false, fmt.Errorf("registry %s: %w", u, err)
	}
	defer func() { _ = resp.Body.Close() }() // only the status matters
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	}
	return false, fmt.Errorf("registry %s: unexpected status %s", u, resp.Status)
}

// secretEnv resolves each secret id to KEY=value for buildctl's env: the
// process env first, then the kit env store; npm_token falls back to
// `gh auth token`. A missing secret is an error naming its key.
func secretEnv(ctx context.Context, r Runner, ws string, ids []string) (env, values []string, err error) {
	if len(ids) == 0 {
		return nil, nil, nil
	}
	store := devenv.Path(ws)
	f, err := devenv.Load(store)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", store, err)
	}
	lookup := devenv.Lookup(os.Getenv, f)
	for _, id := range ids {
		key := strings.ToUpper(id)
		v := lookup(key)
		if v == "" && id == "npm_token" {
			out, err := r.Run(ctx, nil, "gh", "auth", "token")
			if err != nil {
				return nil, nil, fmt.Errorf("NPM_TOKEN unset and gh auth token failed: %w", err)
			}
			v = strings.TrimSpace(string(out))
		}
		if v == "" {
			return nil, nil, fmt.Errorf("%s is not set (env or %s)", key, store)
		}
		env, values = append(env, key+"="+v), append(values, v)
	}
	return env, values, nil
}

// freePort returns a local TCP port that was free a moment ago.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	return port, l.Close()
}

// redact replaces every secret value in err's message.
func redact(err error, values []string) error {
	msg := err.Error()
	for _, v := range values {
		msg = strings.ReplaceAll(msg, v, "[redacted]")
	}
	return errors.New(msg)
}

// Build pushes t.Image:tag to the local registry through the in-cluster
// buildkitd, reached by a port-forward (its NetworkPolicy admits only its own
// namespace). An existing tag is skipped (built=false). t.Dir is absolute.
// Secrets reach buildctl through its env only, never argv, logs or errors.
func Build(ctx context.Context, r BuildRunner, ws string, t BuildTarget, tag string) (bool, error) {
	ref := t.Image + ":" + tag
	exists, err := tagExists(ctx, t.Image, tag)
	if err != nil {
		return false, fmt.Errorf("build %s: %w", ref, err)
	}
	if exists {
		return false, nil
	}
	env, secrets, err := secretEnv(ctx, r, ws, t.Secrets)
	if err != nil {
		return false, fmt.Errorf("build %s: %w", ref, redact(err, secrets))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false, fmt.Errorf("build %s: %w", ref, err)
	}
	buildctl, err := EnsureBuildctl(ctx, r, home)
	if err != nil {
		return false, fmt.Errorf("build %s: %w", ref, err)
	}
	port, err := freePort()
	if err != nil {
		return false, fmt.Errorf("build %s: free port: %w", ref, err)
	}
	stop, err := r.Start(ctx, "kubectl", kubectlArgs("port-forward", "-n", PlatformNamespace, "svc/buildkitd", strconv.Itoa(port)+":1234")...)
	if err != nil {
		return false, fmt.Errorf("build %s: %w", ref, err)
	}
	err = buildctlRun(ctx, r, buildctl, port, t, ref, env)
	if serr := stop(); serr != nil {
		err = errors.Join(err, serr)
	}
	if err != nil {
		return false, fmt.Errorf("build %s: %w", ref, redact(err, secrets))
	}
	return true, nil
}

func buildctlRun(ctx context.Context, r BuildRunner, buildctl string, port int, t BuildTarget, ref string, env []string) error {
	addr := "127.0.0.1:" + strconv.Itoa(port)
	if err := waitPort(ctx, addr); err != nil {
		return err
	}
	args := []string{"--addr", "tcp://" + addr, "build", "--progress", "plain",
		"--frontend", "dockerfile.v0",
		"--local", "context=" + t.Dir, "--local", "dockerfile=" + t.Dir,
		"--opt", "filename=" + t.Dockerfile}
	if t.Target != "" {
		args = append(args, "--opt", "target="+t.Target)
	}
	keys := make([]string, 0, len(t.Args))
	for k := range t.Args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--opt", "build-arg:"+k+"="+t.Args[k])
	}
	for _, id := range t.Secrets {
		args = append(args, "--secret", "id="+id+",env="+strings.ToUpper(id))
	}
	args = append(args, "--output", "type=image,name="+ref+",push=true")
	if _, err := r.RunEnv(ctx, env, buildctl, args...); err != nil {
		return fmt.Errorf("buildctl: %w", err)
	}
	return nil
}
