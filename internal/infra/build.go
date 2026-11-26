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
	OptSecrets []string          // secret ids passed only when their env key is set; never required
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

// VariantTag appends -v<8 hex of sha256 over everything in infra.json that
// shapes the image besides the commit: context, dockerfile, target and every
// build arg (per-env ones resolved). Any change to them, or another env's
// args, gives another tag, so an existing image is never reused for a
// different build. A plain build (context ".", dockerfile "Dockerfile", no
// target, no args) keeps tag.
func VariantTag(tag string, t BuildTarget) string {
	if t.Dir == "." && t.Dockerfile == "Dockerfile" && t.Target == "" && len(t.Args) == 0 {
		return tag
	}
	parts := []string{"context=" + t.Dir, "dockerfile=" + t.Dockerfile, "target=" + t.Target}
	for _, k := range sortedKeys(t.Args) {
		parts = append(parts, "arg:"+k+"="+t.Args[k])
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return tag + "-v" + hex.EncodeToString(sum[:])[:8]
}

// CoreTargets returns the images of env slug in the workspace's build order
// (a base image before the images built FROM it), with its per-env build args
// resolved. Every repo must be checked out under <ws>/repos.
func CoreTargets(w Workspace, ws, domain, slug string) ([]BuildTarget, error) {
	if domain == "" || slug == "" {
		return nil, fmt.Errorf("build targets: domain %q and slug %q are both required", domain, slug)
	}
	ts := make([]BuildTarget, 0, len(w.Builds))
	for _, b := range w.Builds {
		t := BuildTarget{Repo: b.Repo, Dir: b.Context, Dockerfile: b.Dockerfile, Target: b.Target,
			Image: RegistryHost + "/" + b.Image, Tag: b.Tag, Args: map[string]string{},
			Secrets: b.Secrets, OptSecrets: b.OptionalSecrets}
		for k, v := range b.Args {
			t.Args[k] = v
		}
		for _, k := range sortedKeys(b.EnvArgs) {
			t.Args[k] = envArgValue(b.EnvArgs[k], slug, domain)
		}
		if _, err := os.Stat(filepath.Join(ws, "repos", t.Repo)); err != nil { //nolint:gosec // G703 -- t.Repo comes from the workspace's own infra.json, ws is the kit's workspace root
			return nil, fmt.Errorf("build targets: repo %s: %w", t.Repo, err)
		}
		ts = append(ts, t)
	}
	return ts, nil
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
// detached temporary worktree under ~/.zenify/infra/src. Only that kit-owned
// path is ever removed (no global prune), so the user's checkouts and
// worktrees, and their admin entries, are never touched.
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
	// a run interrupted before cleanup leaves this dir and/or its admin entry
	list, err := r.Run(ctx, nil, "git", "-C", root, "worktree", "list", "--porcelain")
	if err != nil {
		return "", "", nil, fmt.Errorf("base source %s: worktree list: %w", repo, err)
	}
	listed := worktreeListed(list, dir)
	_, statErr := os.Stat(dir) //nolint:gosec // G703 -- dir is the kit's src dir + a repo literal + rev-parse hex (checked above)
	exists := statErr == nil
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return "", "", nil, fmt.Errorf("base source %s: stat %s: %w", repo, dir, statErr)
	}
	switch {
	case listed && exists:
		if err := remove(); err != nil {
			return "", "", nil, err
		}
	case exists: // a kit-owned dir git no longer knows
		if err := os.RemoveAll(dir); err != nil { //nolint:gosec // G703 -- dir is the kit src dir + a repo literal + rev-parse hex (checked above)
			return "", "", nil, fmt.Errorf("base source %s: remove stale %s: %w", repo, dir, err)
		}
	}
	add := []string{"-C", root, "worktree", "add", "--detach", dir, sha}
	if listed && !exists {
		// registered but deleted: --force reuses this one entry instead of a global prune
		add = []string{"-C", root, "worktree", "add", "--force", "--detach", dir, sha}
	}
	if _, err := r.Run(ctx, nil, "git", add...); err != nil {
		return "", "", nil, fmt.Errorf("base source %s: add temp worktree %s: %w", repo, dir, err)
	}
	return dir, sha, remove, nil
}

// worktreeListed reports whether `git worktree list --porcelain` output names dir.
func worktreeListed(porcelain []byte, dir string) bool {
	want := map[string]bool{dir: true}
	if real, err := filepath.EvalSymlinks(filepath.Dir(dir)); err == nil {
		want[filepath.Join(real, filepath.Base(dir))] = true
	}
	for _, line := range strings.Split(string(porcelain), "\n") {
		if p, ok := strings.CutPrefix(line, "worktree "); ok && want[p] {
			return true
		}
	}
	return false
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
// `gh auth token`. A missing required secret is an error naming its key; an
// optional one that is unset is left out. used lists the ids to pass.
func secretEnv(ctx context.Context, r Runner, ws string, ids, optional []string) (used, env, values []string, err error) {
	if len(ids) == 0 && len(optional) == 0 {
		return nil, nil, nil, nil
	}
	store := devenv.Path(ws)
	var f *devenv.File
	// the process env wins; the store is read only for a key the env lacks, so
	// an unreadable store never blocks secrets the env already provides
	lookup := func(key string) (string, error) {
		if v := os.Getenv(key); v != "" {
			return v, nil
		}
		if f == nil {
			loaded, err := devenv.Load(store)
			if err != nil {
				return "", fmt.Errorf("read %s: %w", store, err)
			}
			f = loaded
		}
		return devenv.Lookup(os.Getenv, f)(key), nil
	}
	for _, id := range optional {
		key := strings.ToUpper(id)
		v, err := lookup(key)
		if err != nil {
			return nil, nil, nil, err
		}
		if v != "" {
			used, env, values = append(used, id), append(env, key+"="+v), append(values, v)
		}
	}
	for _, id := range ids {
		key := strings.ToUpper(id)
		v, err := lookup(key)
		if err != nil {
			return nil, nil, values, err
		}
		if v == "" && id == "npm_token" {
			out, err := r.Run(ctx, nil, "gh", "auth", "token")
			if err != nil {
				return nil, nil, values, fmt.Errorf("NPM_TOKEN unset and gh auth token failed: %w", err)
			}
			v = strings.TrimSpace(string(out))
		}
		if v == "" {
			return nil, nil, values, fmt.Errorf("%s is not set (env or %s)", key, store)
		}
		used, env, values = append(used, id), append(env, key+"="+v), append(values, v)
	}
	return used, env, values, nil
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
	used, env, secrets, err := secretEnv(ctx, r, ws, t.Secrets, t.OptSecrets)
	if err != nil {
		return false, fmt.Errorf("build %s: %w", ref, redact(err, secrets))
	}
	t.Secrets = used // buildctlRun passes --secret for exactly these
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
