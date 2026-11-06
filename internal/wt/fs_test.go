package wt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWritePortEnv_ReplaceAndAppend(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	// append into a file with no trailing newline
	if err := os.WriteFile(env, []byte("FOO=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WritePortEnv(env, "PORT", 3207); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(env) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if !strings.Contains(string(b), "FOO=1\n") || !strings.Contains(string(b), "PORT=3207") {
		t.Fatalf("append wrong: %q", b)
	}
	// replace an existing PORT line, not append a second
	if err := WritePortEnv(env, "PORT", 3210); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(env) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if strings.Count(string(b), "PORT=") != 1 || !strings.Contains(string(b), "PORT=3210") {
		t.Fatalf("replace wrong: %q", b)
	}
}

func TestWritePortEnv_CreatesFile(t *testing.T) {
	env := filepath.Join(t.TempDir(), ".env")
	if err := WritePortEnv(env, "PORT", 3300); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(env) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if strings.TrimSpace(string(b)) != "PORT=3300" {
		t.Fatalf("create wrong: %q", b)
	}
}

func TestSeedCopyFiles_CopiesPresentWarnsMissing(t *testing.T) {
	repo := t.TempDir()
	wtp := t.TempDir()
	// present: a nested file
	if err := os.MkdirAll(filepath.Join(repo, ".claude"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".claude", "settings.local.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	warns, err := SeedCopyFiles(repo, wtp, []string{".claude/settings.local.json", "CLAUDE.md"})
	if err != nil {
		t.Fatal(err)
	}
	if _, e := os.Stat(filepath.Join(wtp, ".claude", "settings.local.json")); e != nil {
		t.Fatalf("present target not seeded: %v", e)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "CLAUDE.md") {
		t.Fatalf("missing target should warn once about CLAUDE.md, got %v", warns)
	}
}

func TestSeedCopyFiles_DirectoryTargetNotNested(t *testing.T) {
	// A directory copy target must land FLAT at dst, not nested as
	// dst/<basename>/... — the cp-into-existing-dir gotcha copyTree guards.
	repo := t.TempDir()
	wtp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".claude", "sub"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".claude", "sub", "f.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SeedCopyFiles(repo, wtp, []string{".claude"}); err != nil {
		t.Fatal(err)
	}
	// Flat: the file is at wtp/.claude/sub/f.txt, NOT wtp/.claude/.claude/...
	if _, err := os.Stat(filepath.Join(wtp, ".claude", "sub", "f.txt")); err != nil {
		t.Fatalf("directory target not copied flat: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wtp, ".claude", ".claude")); err == nil {
		t.Fatal("directory target nested one level too deep")
	}
}

func TestApplyDeps_Symlink(t *testing.T) {
	repo := t.TempDir()
	wtp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "node_modules", "pkg"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := ApplyDeps(repo, wtp, "symlink", "node_modules", ""); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(filepath.Join(wtp, "node_modules"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("node_modules should be a symlink in symlink mode")
	}
}

func TestApplyDepsNone(t *testing.T) {
	repo := t.TempDir()
	wtp := t.TempDir()
	// mode none: creates no deps dir, runs no install, returns nil.
	if err := ApplyDeps(repo, wtp, "none", "node_modules", "echo SHOULD_NOT_RUN > "+filepath.Join(wtp, "ran.txt")); err != nil {
		t.Fatalf("ApplyDeps none → err %v, want nil", err)
	}
	if _, err := os.Stat(filepath.Join(wtp, "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("none must not create node_modules")
	}
	if _, err := os.Stat(filepath.Join(wtp, "ran.txt")); !os.IsNotExist(err) {
		t.Fatalf("none must not run install")
	}
}

func TestApplyDeps_SymlinkMissingSourceErrors(t *testing.T) {
	if err := ApplyDeps(t.TempDir(), t.TempDir(), "symlink", "node_modules", ""); err == nil {
		t.Fatal("symlink with no source node_modules must error")
	}
}

func TestSeedIdentityEnv_WritesSlugAndCompose(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	// pre-existing content to prove upsert appends without clobbering
	if err := os.WriteFile(env, []byte("PORT=3207\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{Abbrev: "cch", EnvFile: ".env"} // no RedisPrefixEnv → no redis line
	if err := SeedIdentityEnv(env, cfg, "my-task"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(env) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	got := string(b)
	for _, want := range []string{"PORT=3207", "WT_SLUG=my-task", "COMPOSE_PROJECT_NAME=cch-my-task"} {
		if !strings.Contains(got, want) {
			t.Errorf("env missing %q; got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "REDIS") {
		t.Errorf("no RedisPrefixEnv configured, so no redis line expected; got:\n%s", got)
	}
}

func TestSeedIdentityEnv_RedisPrefixWhenConfigured(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	cfg := &Config{Abbrev: "cch", EnvFile: ".env", RedisPrefixEnv: "REDIS_PREFIX"}
	if err := SeedIdentityEnv(env, cfg, "my-task"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(env) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if !strings.Contains(string(b), "REDIS_PREFIX=cch-my-task:") {
		t.Errorf("expected REDIS_PREFIX=cch-my-task: ; got:\n%s", string(b))
	}
}

func TestSeedIdentityEnv_EmptyAbbrevUsesSlugOnly(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	cfg := &Config{Abbrev: "", EnvFile: ".env"}
	if err := SeedIdentityEnv(env, cfg, "solo"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(env) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if !strings.Contains(string(b), "COMPOSE_PROJECT_NAME=solo\n") {
		t.Errorf("empty abbrev should yield COMPOSE_PROJECT_NAME=solo; got:\n%s", string(b))
	}
}

func TestUpsertEnvVar_ReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	if err := os.WriteFile(env, []byte("WT_SLUG=old\nOTHER=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := upsertEnvVar(env, "WT_SLUG", "new"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(env) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	got := string(b)
	if strings.Contains(got, "WT_SLUG=old") || !strings.Contains(got, "WT_SLUG=new") {
		t.Errorf("upsert should replace old→new; got:\n%s", got)
	}
	if !strings.Contains(got, "OTHER=x") {
		t.Errorf("upsert must not drop other keys; got:\n%s", got)
	}
}

// cacheFixture builds <tmp>/cache/repo/{node_modules/x,package-lock.json} and a
// worktree whose package-lock.json is wtLock. Returns repoRoot (named "repo",
// without node_modules) and the worktree path.
func cacheFixture(t *testing.T, cacheLock, wtLock string) (repo, wtp, cache string) {
	t.Helper()
	tmp := t.TempDir()
	cache = filepath.Join(tmp, "cache")
	repo = filepath.Join(tmp, "repo")
	wtp = filepath.Join(tmp, "wt")
	for _, d := range []string{filepath.Join(cache, "repo", "node_modules"), repo, wtp} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(cache, "repo", "node_modules", "x"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "repo", "package-lock.json"), []byte(cacheLock), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtp, "package-lock.json"), []byte(wtLock), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZENIFY_DEPS_CACHE", cache)
	return repo, wtp, cache
}

func TestApplyDepsUsesCacheWhenLockMatches(t *testing.T) {
	repo, wtp, cache := cacheFixture(t, `{"lockfileVersion":3}`, `{"lockfileVersion":3}`)
	if err := ApplyDeps(repo, wtp, "symlink", "node_modules", ""); err != nil {
		t.Fatal(err)
	}
	got, err := os.Readlink(filepath.Join(wtp, "node_modules"))
	if err != nil {
		t.Fatalf("node_modules not a symlink: %v", err)
	}
	if want := filepath.Join(cache, "repo", "node_modules"); got != want {
		t.Fatalf("symlink target = %q, want %q", got, want)
	}
}

func TestApplyDepsCacheLockMismatchFallsBackToOwn(t *testing.T) {
	repo, wtp, _ := cacheFixture(t, `{"a":1}`, `{"a":2}`)
	own := filepath.Join(repo, "node_modules")
	if err := os.MkdirAll(own, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := ApplyDeps(repo, wtp, "symlink", "node_modules", ""); err != nil {
		t.Fatal(err)
	}
	got, err := os.Readlink(filepath.Join(wtp, "node_modules"))
	if err != nil {
		t.Fatalf("node_modules not a symlink: %v", err)
	}
	if got != own {
		t.Fatalf("symlink target = %q, want own %q", got, own)
	}
}

func TestApplyDepsNoCacheEnvUnchanged(t *testing.T) {
	t.Setenv("ZENIFY_DEPS_CACHE", "")
	repo := t.TempDir()
	wtp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "node_modules"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := ApplyDeps(repo, wtp, "symlink", "node_modules", ""); err != nil {
		t.Fatal(err)
	}
	got, err := os.Readlink(filepath.Join(wtp, "node_modules"))
	if err != nil || got != filepath.Join(repo, "node_modules") {
		t.Fatalf("readlink = %q, %v; want repo's node_modules", got, err)
	}
}

// lockJSON is a package-lock.json with one regular package and one optional
// binary for a platform no test runs on (aix); hiddenJSON is npm's hidden
// lockfile after installing it; pkgJSON is the matching package.json.
const (
	lockJSON   = `{"packages":{"":{"name":"p","dependencies":{"x":"^1.0.0"}},"node_modules/x":{"version":"1.0.0","resolved":"https://r/x-1.0.0.tgz","integrity":"sha512-x"},"node_modules/@esbuild/aix-ppc64":{"version":"0.1.0","optional":true,"os":["aix"],"cpu":["ppc64"]}}}`
	hiddenJSON = `{"packages":{"node_modules/x":{"version":"1.0.0","resolved":"https://r/x-1.0.0.tgz","integrity":"sha512-x"}}}`
	pkgJSON    = `{"name":"p","dependencies":{"x":"^1.0.0"}}`
)

type cloneCase struct {
	lock, wtLock, hidden, pkg string            // lock: both sides; wtLock: worktree only
	extra                     map[string]string // worktree-relative path → content
	dropX                     bool              // remove repo/node_modules/x after the hidden lockfile names it
}

// cloneFixture builds repo/{package-lock.json, node_modules/{x,.package-lock.json}}
// and a worktree with package-lock.json + package.json per c. Empty fields take
// the matching defaults above; hidden "-" means no hidden lockfile.
func cloneFixture(t *testing.T, c cloneCase) (repo, wtp string) {
	t.Helper()
	t.Setenv("ZENIFY_DEPS_CACHE", "")
	def := func(v, d string) string {
		if v == "" {
			return d
		}
		return v
	}
	tmp := t.TempDir()
	repo = filepath.Join(tmp, "repo")
	wtp = filepath.Join(tmp, "wt")
	for _, d := range []string{filepath.Join(repo, "node_modules"), wtp} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(repo, "package-lock.json"): def(c.lock, lockJSON),
		filepath.Join(wtp, "package-lock.json"):  def(c.wtLock, def(c.lock, lockJSON)),
		filepath.Join(wtp, "package.json"):       def(c.pkg, pkgJSON),
	}
	if !c.dropX {
		files[filepath.Join(repo, "node_modules", "x")] = "x"
	}
	if h := def(c.hidden, hiddenJSON); h != "-" {
		files[filepath.Join(repo, "node_modules", ".package-lock.json")] = h
	}
	for p, v := range c.extra {
		files[filepath.Join(wtp, p)] = v
	}
	for p, v := range files {
		if err := os.WriteFile(p, []byte(v), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return repo, wtp
}

func TestCloneIsCurrent(t *testing.T) {
	nm := func(repo, wtp string) (string, string) {
		return filepath.Join(repo, "node_modules"), filepath.Join(wtp, "node_modules")
	}
	t.Run("current", func(t *testing.T) {
		repo, wtp := cloneFixture(t, cloneCase{pkg: `{"name":"p","dependencies":{"x":"^1.0.0"},"scripts":{"prepare":"husky","postinstall":"echo","preprepare":"echo"}}`})
		src, dst := nm(repo, wtp)
		scripts, ok := cloneIsCurrent(src, dst, " npm  ci ")
		if !ok {
			t.Fatal("want current")
		}
		if strings.Join(scripts, ",") != "postinstall,preprepare,prepare" {
			t.Fatalf("lifecycle scripts = %v, want [postinstall preprepare prepare]", scripts)
		}
	})
	notCurrent := map[string]struct {
		c   cloneCase
		cmd string
	}{
		"custom install command":     {cloneCase{}, "npm ci && npm run build"},
		"worktree lock differs":      {cloneCase{wtLock: strings.Replace(lockJSON, `"p"`, `"q"`, 1)}, "npm ci"},
		"no hidden lockfile":         {cloneCase{hidden: "-"}, "npm ci"},
		"installed version stale":    {cloneCase{hidden: strings.Replace(hiddenJSON, `"version":"1.0.0"`, `"version":"0.9.0"`, 1)}, "npm ci"},
		"same version other source":  {cloneCase{hidden: strings.Replace(hiddenJSON, "sha512-x", "sha512-old", 1)}, "npm ci"},
		"non-optional not installed": {cloneCase{hidden: `{"packages":{}}`}, "npm ci"},
		"extraneous installed":       {cloneCase{hidden: strings.Replace(hiddenJSON, `}}}`, `},"node_modules/y":{"version":"1.0.0"}}}`, 1)}, "npm ci"},
		"installed package deleted":  {cloneCase{dropX: true}, "npm ci"},
		"package.json out of sync":   {cloneCase{pkg: `{"name":"p","dependencies":{"x":"^1.1.0"}}`}, "npm ci"},
		"install script":             {cloneCase{pkg: `{"name":"p","dependencies":{"x":"^1.0.0"},"scripts":{"install":"node-gyp rebuild"}}`}, "npm ci"},
		"prepublish script":          {cloneCase{pkg: `{"name":"p","dependencies":{"x":"^1.0.0"},"scripts":{"prepublish":"tsc"}}`}, "npm ci"},
		"npm-shrinkwrap.json":        {cloneCase{extra: map[string]string{"npm-shrinkwrap.json": lockJSON}}, "npm ci"},
		"hidden lockfile garbage":    {cloneCase{hidden: "not json"}, "npm ci"},
		"optional for this platform not installed": {cloneCase{
			lock: strings.Replace(lockJSON, `"os":["aix"],"cpu":["ppc64"]`, `"os":[]`, 1),
		}, "npm ci"},
	}
	for name, tc := range notCurrent {
		t.Run(name, func(t *testing.T) {
			repo, wtp := cloneFixture(t, tc.c)
			src, dst := nm(repo, wtp)
			if _, ok := cloneIsCurrent(src, dst, tc.cmd); ok {
				t.Fatal("cloneIsCurrent = true, want false")
			}
		})
	}
}

func TestNpmListAllows(t *testing.T) {
	for _, c := range []struct {
		list []string
		v    string
		want bool
	}{
		{nil, "linux", true},
		{[]string{"linux"}, "linux", true},
		{[]string{"darwin"}, "linux", false},
		{[]string{"!win32"}, "linux", true},
		{[]string{"!linux"}, "linux", false},
	} {
		if got := npmListAllows(c.list, c.v); got != c.want {
			t.Errorf("npmListAllows(%v, %q) = %v, want %v", c.list, c.v, got, c.want)
		}
	}
}

// installMarker is an install command that leaves a file behind, so a test can
// tell whether ApplyDeps ran it.
const installMarker = "touch installed"

func installed(t *testing.T, wtp string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(wtp, "installed"))
	return err == nil
}

// With "npm ci" and a current clone, ApplyDeps returns without invoking npm
// (no lifecycle scripts in pkgJSON), so this passes on a machine without npm.
func TestApplyDepsCloneSkipsNpmCiWhenCurrent(t *testing.T) {
	repo, wtp := cloneFixture(t, cloneCase{})
	if err := ApplyDeps(repo, wtp, "clone", "node_modules", "npm ci"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(wtp, "node_modules", "x")); err != nil {
		t.Fatalf("clone missing: %v", err)
	}
}

func TestApplyDepsCloneCustomInstallAlwaysRuns(t *testing.T) {
	repo, wtp := cloneFixture(t, cloneCase{})
	if err := ApplyDeps(repo, wtp, "clone", "node_modules", installMarker); err != nil {
		t.Fatal(err)
	}
	if !installed(t, wtp) {
		t.Fatal("custom install command skipped")
	}
}

func TestApplyDepsCloneFromCacheSkipsWhenCurrent(t *testing.T) {
	repo, wtp, cache := cacheFixture(t, lockJSON, lockJSON)
	for p, v := range map[string]string{
		filepath.Join(cache, "repo", "node_modules", ".package-lock.json"): hiddenJSON,
		filepath.Join(wtp, "package.json"):                                 pkgJSON,
	} {
		if err := os.WriteFile(p, []byte(v), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := cloneIsCurrent(filepath.Join(cache, "repo", "node_modules"), filepath.Join(wtp, "node_modules"), "npm ci"); !ok {
		t.Fatal("cache entry matching the lock should count as current")
	}
	if err := ApplyDeps(repo, wtp, "clone", "node_modules", "npm ci"); err != nil {
		t.Fatal(err)
	}
}
