package wt

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
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

// testEnvTag is the fixed build-env fingerprint the probe overrides return, so
// slot hashes are deterministic and never depend on node being installed.
const testEnvTag = "testabi"

// stubProbes pins probeEnvTag → ("testabi", true) and probeIgnoreScripts →
// (false, true) for the test, restoring both afterwards. Call it in every test
// that drives ApplyDeps in clone mode: without it the real probes shell out to
// node/npm, and with stubNpm on PATH the ignore-scripts probe silently runs the
// stub (which prints nothing → parses as "not ignored, ok"), so a forgotten
// override looks green.
func stubProbes(t *testing.T) {
	t.Helper()
	origTag, origIgnore := probeEnvTag, probeIgnoreScripts
	probeEnvTag = func() (string, bool) { return testEnvTag, true }
	probeIgnoreScripts = func(string) (bool, bool) { return false, true }
	t.Cleanup(func() { probeEnvTag, probeIgnoreScripts = origTag, origIgnore })
}

// cacheFixture builds the hash-keyed cache slot
// <tmp>/cache/repo/<lockHash(cacheLock, testEnvTag)>/{node_modules/x,package-lock.json}
// and a worktree whose package-lock.json is wtLock. Returns repoRoot (named
// "repo", without node_modules), the worktree path, the cache dir and the slot's
// hash. The slot hash uses testEnvTag so it matches what a stubProbes'd ApplyDeps
// computes.
func cacheFixture(t *testing.T, cacheLock, wtLock string) (repo, wtp, cache, hash string) {
	t.Helper()
	tmp := t.TempDir()
	cache = filepath.Join(tmp, "cache")
	repo = filepath.Join(tmp, "repo")
	wtp = filepath.Join(tmp, "wt")
	lockSrc := filepath.Join(tmp, "cache-lock-src.json")
	if err := os.WriteFile(lockSrc, []byte(cacheLock), 0o600); err != nil {
		t.Fatal(err)
	}
	hash, err := lockHash(lockSrc, testEnvTag)
	if err != nil {
		t.Fatal(err)
	}
	slot := filepath.Join(cache, "repo", hash)
	for _, d := range []string{filepath.Join(slot, "node_modules"), repo, wtp} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(slot, "node_modules", "x"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(slot, "package-lock.json"), []byte(cacheLock), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtp, "package-lock.json"), []byte(wtLock), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZENIFY_DEPS_CACHE", cache)
	return repo, wtp, cache, hash
}

// cacheEntries lists the entry names under dir; a missing dir yields none.
func cacheEntries(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	names := make([]string, 0, len(es))
	for _, e := range es {
		names = append(names, e.Name())
	}
	return names
}

// SC-10: symlink mode never reads the cache, even when a matching slot exists.
func TestApplyDepsSymlinkIgnoresMatchingCache(t *testing.T) {
	repo, wtp, cache, _ := cacheFixture(t, `{"lockfileVersion":3}`, `{"lockfileVersion":3}`)
	own := filepath.Join(repo, "node_modules")
	if err := os.MkdirAll(own, 0o750); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(cache, filepath.Base(repo))
	before := cacheEntries(t, parent)
	if err := ApplyDeps(repo, wtp, "symlink", "node_modules", ""); err != nil {
		t.Fatal(err)
	}
	if after := cacheEntries(t, parent); !reflect.DeepEqual(before, after) {
		t.Fatalf("symlink mode changed the cache: before %v, after %v", before, after)
	}
	got, err := os.Readlink(filepath.Join(wtp, "node_modules"))
	if err != nil {
		t.Fatalf("node_modules not a symlink: %v", err)
	}
	if got != own {
		t.Fatalf("symlink target = %q, want own %q", got, own)
	}
}

func TestApplyDepsSymlinkIgnoresMismatchedCache(t *testing.T) {
	repo, wtp, cache, _ := cacheFixture(t, `{"a":1}`, `{"a":2}`)
	own := filepath.Join(repo, "node_modules")
	if err := os.MkdirAll(own, 0o750); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(cache, filepath.Base(repo))
	before := cacheEntries(t, parent)
	if err := ApplyDeps(repo, wtp, "symlink", "node_modules", ""); err != nil {
		t.Fatal(err)
	}
	if after := cacheEntries(t, parent); !reflect.DeepEqual(before, after) {
		t.Fatalf("symlink mode changed the cache: before %v, after %v", before, after)
	}
	got, err := os.Readlink(filepath.Join(wtp, "node_modules"))
	if err != nil {
		t.Fatalf("node_modules not a symlink: %v", err)
	}
	if got != own {
		t.Fatalf("symlink target = %q, want own %q", got, own)
	}
}

func TestDepsSourceHashLookup(t *testing.T) {
	const lock = `{"lockfileVersion":3}`
	env := func(cache string) func(string) (string, bool) {
		return func(k string) (string, bool) {
			if k == "ZENIFY_DEPS_CACHE" {
				return cache, true
			}
			return "", false
		}
	}
	t.Run("hit", func(t *testing.T) {
		repo, wtp, cache, hash := cacheFixture(t, lock, lock)
		slot := filepath.Join(cache, "repo", hash)
		old := time.Now().Add(-48 * time.Hour)
		if err := os.Chtimes(slot, old, old); err != nil {
			t.Fatal(err)
		}
		got, from := depsSource(repo, wtp, "node_modules", testEnvTag, env(cache))
		if want := filepath.Join(slot, "node_modules"); got != want || !from {
			t.Fatalf("got (%q, %v), want (%q, true)", got, from, want)
		}
		fi, err := os.Stat(slot)
		if err != nil {
			t.Fatal(err)
		}
		if time.Since(fi.ModTime()) > time.Hour {
			t.Fatalf("slot mtime not touched on hit: %v", fi.ModTime())
		}
	})
	t.Run("miss", func(t *testing.T) {
		repo, wtp, cache, _ := cacheFixture(t, lock, `{"other":true}`)
		got, from := depsSource(repo, wtp, "node_modules", testEnvTag, env(cache))
		if want := filepath.Join(repo, "node_modules"); got != want || from {
			t.Fatalf("got (%q, %v), want (%q, false)", got, from, want)
		}
	})
	t.Run("collision", func(t *testing.T) {
		repo, wtp, cache, hash := cacheFixture(t, lock, lock)
		// slot exists at the worktree's hash but holds a different lock's bytes
		if err := os.WriteFile(filepath.Join(cache, "repo", hash, "package-lock.json"), []byte(`{"collided":1}`), 0o600); err != nil {
			t.Fatal(err)
		}
		got, from := depsSource(repo, wtp, "node_modules", testEnvTag, env(cache))
		if want := filepath.Join(repo, "node_modules"); got != want || from {
			t.Fatalf("got (%q, %v), want (%q, false)", got, from, want)
		}
	})
	t.Run("disabled", func(t *testing.T) {
		repo, wtp, cache, _ := cacheFixture(t, lock, lock)
		_ = cache
		got, from := depsSource(repo, wtp, "node_modules", testEnvTag, env("off"))
		if want := filepath.Join(repo, "node_modules"); got != want || from {
			t.Fatalf("got (%q, %v), want (%q, false)", got, from, want)
		}
	})
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
		if strings.Join(scripts, ",") != "postinstall,prepare" {
			t.Fatalf("lifecycle scripts = %v, want [postinstall prepare]", scripts)
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
		"hook npm ci never runs":     {cloneCase{pkg: `{"name":"p","dependencies":{"x":"^1.0.0"},"scripts":{"postinstall":"echo","prepostinstall":"exit 1"}}`}, "npm ci"},
		"preprepare without prepare": {cloneCase{pkg: `{"name":"p","dependencies":{"x":"^1.0.0"},"scripts":{"preprepare":"echo"}}`}, "npm ci"},
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
	stubProbes(t)
	repo, wtp := cloneFixture(t, cloneCase{})
	if err := ApplyDeps(repo, wtp, "clone", "node_modules", "npm ci"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(wtp, "node_modules", "x")); err != nil {
		t.Fatalf("clone missing: %v", err)
	}
}

func TestApplyDepsCloneCustomInstallAlwaysRuns(t *testing.T) {
	stubProbes(t)
	repo, wtp := cloneFixture(t, cloneCase{})
	if err := ApplyDeps(repo, wtp, "clone", "node_modules", installMarker); err != nil {
		t.Fatal(err)
	}
	if !installed(t, wtp) {
		t.Fatal("custom install command skipped")
	}
}

func TestApplyDepsCloneFromCacheSkipsWhenCurrent(t *testing.T) {
	stubProbes(t)
	repo, wtp, cache, hash := cacheFixture(t, lockJSON, lockJSON)
	slotNM := filepath.Join(cache, "repo", hash, "node_modules")
	for p, v := range map[string]string{
		filepath.Join(slotNM, ".package-lock.json"): hiddenJSON,
		filepath.Join(wtp, "package.json"):          pkgJSON,
	} {
		if err := os.WriteFile(p, []byte(v), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := cloneIsCurrent(slotNM, filepath.Join(wtp, "node_modules"), "npm ci"); !ok {
		t.Fatal("cache entry matching the lock should count as current")
	}
	// the repo has its OWN node_modules and a DIFFERENT lock: a regression that
	// cloned from the main checkout would carry repo_only_marker over.
	repoNM := filepath.Join(repo, "node_modules")
	if err := os.MkdirAll(repoNM, 0o750); err != nil {
		t.Fatal(err)
	}
	for p, v := range map[string]string{
		filepath.Join(repoNM, "repo_only_marker"): "repo",
		filepath.Join(repo, "package-lock.json"):  `{"lockfileVersion":3,"repo":"different"}`,
	} {
		if err := os.WriteFile(p, []byte(v), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := ApplyDeps(repo, wtp, "clone", "node_modules", "npm ci"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(wtp, "node_modules", "repo_only_marker")); !os.IsNotExist(err) {
		t.Fatalf("worktree seeded from the repo's own node_modules, not the cache slot (stat err: %v)", err)
	}
	if _, err := os.Stat(filepath.Join(wtp, "node_modules", "x")); err != nil {
		t.Fatalf("clone not seeded from cache slot: %v", err)
	}
}

func TestResolveCacheDir(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		wantDir string
		wantOn  bool
	}{
		{"unset, XDG set", map[string]string{"XDG_CACHE_HOME": "/x", "HOME": "/h"}, "/x/zenify/wt-deps", true},
		{"unset, XDG unset, HOME set", map[string]string{"HOME": "/h"}, "/h/.cache/zenify/wt-deps", true},
		{"empty disables", map[string]string{"ZENIFY_DEPS_CACHE": "", "HOME": "/h"}, "", false},
		{"off disables", map[string]string{"ZENIFY_DEPS_CACHE": "off", "HOME": "/h"}, "", false},
		{"OFF disables", map[string]string{"ZENIFY_DEPS_CACHE": "OFF", "HOME": "/h"}, "", false},
		{"custom path", map[string]string{"ZENIFY_DEPS_CACHE": "/custom", "HOME": "/h"}, "/custom", true},
		{"unset, no XDG, no HOME", map[string]string{}, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lookup := func(k string) (string, bool) { v, ok := c.env[k]; return v, ok }
			dir, on := resolveCacheDir(lookup)
			if dir != c.wantDir || on != c.wantOn {
				t.Fatalf("got (%q, %v), want (%q, %v)", dir, on, c.wantDir, c.wantOn)
			}
		})
	}
}

func TestLockHash(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	a := write("a.json", `{"lockfileVersion":3}`)
	a2 := write("a2.json", `{"lockfileVersion":3}`)
	b := write("b.json", `{"lockfileVersion":2}`)

	// hash = sha256(lock ‖ 0x00 ‖ tag)[:16]. Goldens from
	//   printf '{"lockfileVersion":3}\000testabi' | shasum -a 256 | cut -c1-16
	//   printf '{"lockfileVersion":2}\000testabi' | shasum -a 256 | cut -c1-16
	for p, want := range map[string]string{a: "439a58b15fcf9104", a2: "439a58b15fcf9104", b: "033ed51c43753ff8"} {
		got, err := lockHash(p, testEnvTag)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("lockHash(%s) = %q, want %q", p, got, want)
		}
	}
	// the env tag changes the slot key: same lock, different tag → different hash.
	hA, err := lockHash(a, "tagA")
	if err != nil {
		t.Fatal(err)
	}
	hB, err := lockHash(a, "tagB")
	if err != nil {
		t.Fatal(err)
	}
	if hA == hB {
		t.Fatalf("different env tags must yield different hashes, both %q", hA)
	}
	if _, err := lockHash(filepath.Join(dir, "missing.json"), testEnvTag); err == nil {
		t.Fatal("missing file: want error")
	}
}

// wtLockHash is the lockHash of the worktree's package-lock.json under testEnvTag.
func wtLockHash(t *testing.T, wtp string) string {
	t.Helper()
	h, err := lockHash(filepath.Join(wtp, "package-lock.json"), testEnvTag)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestPopulateFreshSlot(t *testing.T) { // SC-2 — branch (b): real install seeds the slot
	stubProbes(t)
	repo, wtp, cache := realInstallFixture(t)
	marker := stubNpm(t, hiddenJSON)
	if err := ApplyDeps(repo, wtp, "clone", "node_modules", "npm ci"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("stub npm did not run (fixture took the skip branch): %v", err)
	}
	slot := filepath.Join(cache, "repo", wtLockHash(t, wtp))
	if _, err := os.Stat(filepath.Join(slot, "node_modules", "x")); err != nil {
		t.Fatalf("slot node_modules missing: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(slot, "package-lock.json"))
	if err != nil || string(got) != lockJSON {
		t.Fatalf("slot lock = %q, %v; want worktree lock", got, err)
	}
	ents, err := os.ReadDir(filepath.Join(cache, "repo"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Fatalf("temp dir left behind: %s", e.Name())
		}
	}
}

func TestPopulateIdempotent(t *testing.T) { // SC-3 — branch (b): existing slot left alone
	stubProbes(t)
	repo, wtp, cache := realInstallFixture(t)
	marker := stubNpm(t, hiddenJSON)
	slot := filepath.Join(cache, "repo", wtLockHash(t, wtp))
	if err := os.MkdirAll(filepath.Join(slot, "node_modules"), 0o750); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(slot, "node_modules", "sentinel")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ApplyDeps(repo, wtp, "clone", "node_modules", "npm ci"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("stub npm did not run (fixture took the skip branch): %v", err)
	}
	if b, err := os.ReadFile(sentinel); err != nil || string(b) != "keep" {
		t.Fatalf("existing slot was replaced: %q, %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(slot, "node_modules", "x")); err == nil {
		t.Fatal("existing slot was overwritten with new content")
	}
}

func TestPopulateLoseRace(t *testing.T) { // SC-7
	_, wtp := cloneFixture(t, cloneCase{})
	if err := os.MkdirAll(filepath.Join(wtp, "node_modules"), 0o750); err != nil {
		t.Fatal(err)
	}
	cache := t.TempDir()
	parent := filepath.Join(cache, "repo")
	// target slot exists without a deps dir, so populate proceeds to rename and loses
	slot := filepath.Join(parent, wtLockHash(t, wtp))
	if err := os.MkdirAll(slot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(slot, "occupied"), []byte("1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := populateCache(cache, filepath.Join(t.TempDir(), "repo"), wtp, "node_modules", testEnvTag); err != nil {
		t.Fatalf("lost race must not be an error: %v", err)
	}
	ents, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Fatalf("temp dir left behind: %s", e.Name())
		}
	}
}

func TestPopulateCustomPath(t *testing.T) { // SC-8 — branch (b) writes under the custom dir
	stubProbes(t)
	repo, wtp := cloneFixture(t, cloneCase{hidden: "-"})
	marker := stubNpm(t, hiddenJSON)
	custom := filepath.Join(t.TempDir(), "mine")
	t.Setenv("ZENIFY_DEPS_CACHE", custom)
	if err := ApplyDeps(repo, wtp, "clone", "node_modules", "npm ci"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("stub npm did not run (fixture took the skip branch): %v", err)
	}
	if _, err := os.Stat(filepath.Join(custom, "repo", wtLockHash(t, wtp), "node_modules", "x")); err != nil {
		t.Fatalf("slot not under custom dir: %v", err)
	}
}

func TestPopulateOff(t *testing.T) { // SC-4 — branch (b) reaches the populate gate, cache off blocks it
	stubProbes(t)
	repo, wtp := cloneFixture(t, cloneCase{hidden: "-"})
	marker := stubNpm(t, hiddenJSON)
	cache := filepath.Join(t.TempDir(), "never")
	t.Setenv("ZENIFY_DEPS_CACHE", "off")
	t.Setenv("XDG_CACHE_HOME", cache)
	if err := ApplyDeps(repo, wtp, "clone", "node_modules", "npm ci"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("stub npm did not run (fixture took the skip branch): %v", err)
	}
	if _, err := os.Stat(cache); err == nil {
		t.Fatal("cache dir created while cache is off")
	}
}

func TestPopulateNoLock(t *testing.T) { // SC-6
	repo, wtp := cloneFixture(t, cloneCase{})
	cache := t.TempDir()
	if err := os.Remove(filepath.Join(wtp, "package-lock.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(wtp, "node_modules"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := populateCache(cache, repo, wtp, "node_modules", testEnvTag); err != nil {
		t.Fatalf("no lock must be skipped silently, got %v", err)
	}
	if ents, _ := os.ReadDir(cache); len(ents) != 0 {
		t.Fatalf("cache written without a lock file: %v", ents)
	}
}

func TestPopulateEmptyInstallNoPoison(t *testing.T) { // SC-9, FR-2.1
	stubProbes(t)
	repo, wtp := cloneFixture(t, cloneCase{hidden: "-"})
	cache := t.TempDir()
	t.Setenv("ZENIFY_DEPS_CACHE", cache)
	if err := ApplyDeps(repo, wtp, "clone", "node_modules", ""); err != nil {
		t.Fatal(err)
	}
	if ents, _ := os.ReadDir(cache); len(ents) != 0 {
		t.Fatalf("stale clone poisoned the cache: %v", ents)
	}
}

func TestPopulateCustomInstallNoPoison(t *testing.T) { // SC-9, FR-2.1
	stubProbes(t)
	repo, wtp := cloneFixture(t, cloneCase{hidden: "-"})
	cache := t.TempDir()
	t.Setenv("ZENIFY_DEPS_CACHE", cache)
	if err := ApplyDeps(repo, wtp, "clone", "node_modules", installMarker); err != nil {
		t.Fatal(err)
	}
	if !installed(t, wtp) {
		t.Fatal("custom install did not run")
	}
	if ents, _ := os.ReadDir(cache); len(ents) != 0 {
		t.Fatalf("custom install poisoned the cache: %v", ents)
	}
}

func TestIsPlainNpmInstall(t *testing.T) {
	for cmd, want := range map[string]bool{
		"npm ci": true, " npm  install ": true, "npm i": true,
		"pnpm i": false, "": false, "npm ci && npm run build": false,
	} {
		if got := isPlainNpmInstall(cmd); got != want {
			t.Errorf("isPlainNpmInstall(%q) = %v, want %v", cmd, got, want)
		}
	}
}

func TestIsSlotName(t *testing.T) {
	cases := map[string]bool{
		"0123456789abcdef":  true,
		"ffffffffffffffff":  true,
		"0123456789abcde":   false, // 15
		"0123456789abcdef0": false, // 17
		"0123456789ABCDEF":  false, // uppercase
		"0123456789abcdeg":  false, // non-hex
		"node_modules":      false,
		".tmp-x":            false,
		"":                  false,
	}
	for name, want := range cases {
		if got := isSlotName(name); got != want {
			t.Errorf("isSlotName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestPruneCacheCapLRU(t *testing.T) { // SC-5
	cache := t.TempDir()
	repo := filepath.Join(t.TempDir(), "myrepo")
	parent := filepath.Join(cache, "myrepo")
	names := []string{"0000000000000001", "0000000000000002", "0000000000000003", "0000000000000004", "0000000000000005", "0000000000000006"}
	base := time.Now().Add(-time.Hour)
	for i, n := range names {
		p := filepath.Join(parent, n)
		if err := os.MkdirAll(p, 0o750); err != nil {
			t.Fatal(err)
		}
		mt := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
	}
	if err := pruneCache(cache, repo, 5); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(parent, names[0])); !os.IsNotExist(err) {
		t.Errorf("oldest slot should be removed, stat err=%v", err)
	}
	for _, n := range names[1:] {
		if _, err := os.Stat(filepath.Join(parent, n)); err != nil {
			t.Errorf("slot %s should remain: %v", n, err)
		}
	}
}

func TestPruneSkipsNonSlotDirs(t *testing.T) {
	cache := t.TempDir()
	repo := filepath.Join(t.TempDir(), "myrepo")
	parent := filepath.Join(cache, "myrepo")
	for _, n := range []string{"node_modules", ".tmp-abc", "aaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbb"} {
		if err := os.MkdirAll(filepath.Join(parent, n), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * time.Hour)
	for _, n := range []string{"node_modules", ".tmp-abc", "aaaaaaaaaaaaaaaa"} {
		if err := os.Chtimes(filepath.Join(parent, n), old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := pruneCache(cache, repo, 1); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"node_modules", ".tmp-abc"} {
		if _, err := os.Stat(filepath.Join(parent, n)); err != nil {
			t.Errorf("%s must survive: %v", n, err)
		}
	}
	if _, err := os.Stat(filepath.Join(parent, "aaaaaaaaaaaaaaaa")); !os.IsNotExist(err) {
		t.Errorf("older slot should be pruned, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(parent, "bbbbbbbbbbbbbbbb")); err != nil {
		t.Errorf("newer slot should remain: %v", err)
	}
}

func TestPruneCacheMissingParent(t *testing.T) {
	if err := pruneCache(t.TempDir(), "/x/nope", 5); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestApplyDepsPopulatePrunes(t *testing.T) { // SC-5 wiring: populate + prune fire from ApplyDeps (branch (b))
	stubProbes(t)
	repo, wtp := cloneFixture(t, cloneCase{hidden: "-"})
	marker := stubNpm(t, hiddenJSON)
	cache := t.TempDir()
	t.Setenv("ZENIFY_DEPS_CACHE", cache)
	parent := filepath.Join(cache, "repo")
	seeded := []string{"0000000000000001", "0000000000000002", "0000000000000003", "0000000000000004", "0000000000000005"}
	base := time.Now().Add(-2 * time.Hour)
	for i, n := range seeded {
		p := filepath.Join(parent, n)
		if err := os.MkdirAll(p, 0o750); err != nil {
			t.Fatal(err)
		}
		mt := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
	}
	if err := ApplyDeps(repo, wtp, "clone", "node_modules", "npm ci"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("stub npm did not run (fixture took the skip branch): %v", err)
	}
	fresh := wtLockHash(t, wtp)
	if _, err := os.Stat(filepath.Join(parent, fresh, "node_modules", "x")); err != nil {
		t.Fatalf("new slot missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(parent, seeded[0])); !os.IsNotExist(err) {
		t.Errorf("oldest seeded slot should be pruned, err=%v", err)
	}
	n := 0
	ents, _ := os.ReadDir(parent)
	for _, e := range ents {
		if isSlotName(e.Name()) {
			n++
		}
	}
	if n != cacheSlotCap {
		t.Fatalf("want %d slots, got %d", cacheSlotCap, n)
	}
}

// stubNpm puts a fake `npm` first on PATH. When run it touches a marker (so a
// test can prove the real-install branch ran, not the skip branch) and writes
// hidden as <cwd>/node_modules/.package-lock.json, like npm ci would.
func stubNpm(t *testing.T, hidden string) (marker string) {
	t.Helper()
	dir := t.TempDir()
	marker = filepath.Join(dir, "ran")
	hiddenSrc := filepath.Join(dir, "hidden.json")
	if err := os.WriteFile(hiddenSrc, []byte(hidden), 0o600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ntouch '" + marker + "'\nmkdir -p node_modules\ncp '" + hiddenSrc + "' node_modules/.package-lock.json\n"
	if err := os.WriteFile(filepath.Join(dir, "npm"), []byte(script), 0o755); err != nil { //nolint:gosec // G306 -- test stub must be executable
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return marker
}

// realInstallFixture is a clone fixture that is NOT current (no hidden lockfile
// in the source), so ApplyDeps runs the install and takes the populate branch.
func realInstallFixture(t *testing.T) (repo, wtp, cache string) {
	t.Helper()
	repo, wtp = cloneFixture(t, cloneCase{hidden: "-"})
	cache = filepath.Join(t.TempDir(), "cache")
	t.Setenv("ZENIFY_DEPS_CACHE", cache)
	return repo, wtp, cache
}

// After a real npm ci whose installed tree matches the lock, the tree is cached.
func TestApplyDepsPopulateAfterRealInstall(t *testing.T) {
	stubProbes(t)
	repo, wtp, cache := realInstallFixture(t)
	marker := stubNpm(t, hiddenJSON)
	if err := ApplyDeps(repo, wtp, "clone", "node_modules", "npm ci"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("stub npm did not run (fixture took the skip branch): %v", err)
	}
	hash, err := lockHash(filepath.Join(wtp, "package-lock.json"), testEnvTag)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cache, "repo", hash, "node_modules")); err != nil {
		t.Fatalf("complete install not cached: %v", err)
	}
}

// npm ci can exit 0 with a partial tree (omit=dev, NODE_ENV=production): the
// populate gate checks installed content, so nothing is cached.
func TestApplyDepsNoPopulateWhenInstallIncomplete(t *testing.T) {
	stubProbes(t)
	repo, wtp, cache := realInstallFixture(t)
	marker := stubNpm(t, `{"packages":{}}`) // lock's non-optional node_modules/x missing
	if err := ApplyDeps(repo, wtp, "clone", "node_modules", "npm ci"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("stub npm did not run (fixture took the skip branch): %v", err)
	}
	if got := cacheEntries(t, filepath.Join(cache, "repo")); len(got) != 0 {
		t.Fatalf("incomplete install was cached: %v", got)
	}
}

// A slot written under one env tag is not reused under another: switching Node
// ABI produces a cache miss, not a wrong-ABI tree (zenie #1).
func TestDepsCacheTagSeparatesSlots(t *testing.T) {
	repo, wtp := cloneFixture(t, cloneCase{})
	if err := os.MkdirAll(filepath.Join(wtp, "node_modules"), 0o750); err != nil {
		t.Fatal(err)
	}
	cache := t.TempDir()
	if err := populateCache(cache, repo, wtp, "node_modules", "tagA"); err != nil {
		t.Fatal(err)
	}
	env := func(k string) (string, bool) {
		if k == "ZENIFY_DEPS_CACHE" {
			return cache, true
		}
		return "", false
	}
	if got, from := depsSource(repo, wtp, "node_modules", "tagB", env); from {
		t.Fatalf("tag B must miss the tag-A slot, got (%q, %v)", got, from)
	}
	if _, from := depsSource(repo, wtp, "node_modules", "tagA", env); !from {
		t.Fatal("tag A must hit the slot it wrote")
	}
}

// A tree installed with ignore-scripts=true has unbuilt deps yet passes
// cloneIsCurrent; it must never seed the cache (zenie #2). Serving a full tree TO
// such a worktree stays allowed, so only the write is blocked.
func TestApplyDepsNoPopulateWhenIgnoreScripts(t *testing.T) {
	origTag, origIgnore := probeEnvTag, probeIgnoreScripts
	probeEnvTag = func() (string, bool) { return testEnvTag, true }
	probeIgnoreScripts = func(string) (bool, bool) { return true, true }
	t.Cleanup(func() { probeEnvTag, probeIgnoreScripts = origTag, origIgnore })

	repo, wtp, cache := realInstallFixture(t)
	marker := stubNpm(t, hiddenJSON)
	if err := ApplyDeps(repo, wtp, "clone", "node_modules", "npm ci"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("stub npm did not run (fixture took the skip branch): %v", err)
	}
	if got := cacheEntries(t, filepath.Join(cache, "repo")); len(got) != 0 {
		t.Fatalf("ignore-scripts tree was cached: %v", got)
	}
}

// When the env probe fails (no node), the cache is disabled for the run: the
// source is the main checkout, no slot is read, and no slot is written — never
// collapsed onto an empty tag (zenie #1).
func TestApplyDepsProbeEnvFailDisablesCache(t *testing.T) {
	origTag, origIgnore := probeEnvTag, probeIgnoreScripts
	probeEnvTag = func() (string, bool) { return "", false }
	probeIgnoreScripts = func(string) (bool, bool) { return false, true }
	t.Cleanup(func() { probeEnvTag, probeIgnoreScripts = origTag, origIgnore })

	repo, wtp, cache := realInstallFixture(t)
	marker := stubNpm(t, hiddenJSON)
	// main checkout carries a marker, so a clone from it is recognisable.
	if err := os.WriteFile(filepath.Join(repo, "node_modules", "main_marker"), []byte("main"), 0o600); err != nil {
		t.Fatal(err)
	}
	// a slot a buggy run WOULD read if it collapsed onto an empty tag.
	wtLock, err := os.ReadFile(filepath.Join(wtp, "package-lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	emptyTagHash, err := lockHash(filepath.Join(wtp, "package-lock.json"), "")
	if err != nil {
		t.Fatal(err)
	}
	plantedNM := filepath.Join(cache, "repo", emptyTagHash, "node_modules")
	if err := os.MkdirAll(plantedNM, 0o750); err != nil {
		t.Fatal(err)
	}
	for p, v := range map[string][]byte{
		filepath.Join(plantedNM, "cache_marker"):                        []byte("cache"),
		filepath.Join(cache, "repo", emptyTagHash, "package-lock.json"): wtLock,
	} {
		if err := os.WriteFile(p, v, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	before := cacheEntries(t, filepath.Join(cache, "repo"))

	if err := ApplyDeps(repo, wtp, "clone", "node_modules", "npm ci"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("stub npm did not run (fixture took the skip branch): %v", err)
	}
	// source was the main checkout, not the planted slot.
	if _, err := os.Stat(filepath.Join(wtp, "node_modules", "main_marker")); err != nil {
		t.Fatalf("worktree not seeded from the main checkout: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wtp, "node_modules", "cache_marker")); !os.IsNotExist(err) {
		t.Fatalf("cache was read despite a failed env probe (stat err: %v)", err)
	}
	// no new slot written.
	if after := cacheEntries(t, filepath.Join(cache, "repo")); !reflect.DeepEqual(before, after) {
		t.Fatalf("cache was written despite a failed env probe: before %v, after %v", before, after)
	}
}

// A clone that is already current was copied from the main checkout (or an
// existing slot), whose build env is unknown, so branch (a) must never seed a
// slot (zenie #3 regression).
func TestApplyDepsCurrentCloneFromMainNoPopulate(t *testing.T) {
	stubProbes(t)
	repo, wtp := cloneFixture(t, cloneCase{})
	cache := t.TempDir()
	t.Setenv("ZENIFY_DEPS_CACHE", cache)
	if err := ApplyDeps(repo, wtp, "clone", "node_modules", "npm ci"); err != nil {
		t.Fatal(err)
	}
	if got := cacheEntries(t, filepath.Join(cache, "repo")); len(got) != 0 {
		t.Fatalf("branch (a) seeded the cache: %v", got)
	}
}
