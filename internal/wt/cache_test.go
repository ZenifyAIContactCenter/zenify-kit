package wt

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const cacheDepsDir = "node_modules"

// cacheEnv pins ZENIFY_DEPS_CACHE to dir and leaves every other var unset.
func cacheEnv(dir string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		if k == "ZENIFY_DEPS_CACHE" {
			return dir, true
		}
		return "", false
	}
}

// installedTree builds <tmp>/<name> as a freshly `npm ci`'d project whose tree
// provably matches lock: package.json, package-lock.json, node_modules/{x/package.json,
// .package-lock.json}.
func installedTree(t *testing.T, name, lock string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(filepath.Join(dir, cacheDepsDir, "x"), 0o750); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"package.json":                    pkgJSON,
		"package-lock.json":               lock,
		"node_modules/.package-lock.json": hiddenJSON,
		"node_modules/x/package.json":     `{"name":"x","version":"1.0.0"}`,
	}
	for p, v := range files {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(p)), []byte(v), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// bareDir is a project dir with only package-lock.json (a consumer of the cache).
func bareDir(t *testing.T, lock string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "consumer")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(lock), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCachePutWritesSlotThenGetHits(t *testing.T) {
	stubProbes(t)
	cache := t.TempDir()
	src := installedTree(t, "src", lockJSON)
	if err := CachePut(src, "myrepo", cacheEnv(cache)); err != nil {
		t.Fatal(err)
	}
	dst := bareDir(t, lockJSON)
	hit, err := CacheGet(dst, "myrepo", cacheEnv(cache))
	if err != nil || !hit {
		t.Fatalf("CacheGet = (%v, %v), want (true, nil)", hit, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "node_modules", "x", "package.json")); err != nil {
		t.Fatalf("node_modules/x/package.json missing at dst: %v", err)
	}
	if has, err := CacheHas(dst, "myrepo", cacheEnv(cache)); err != nil || !has {
		t.Fatalf("CacheHas = (%v, %v), want (true, nil)", has, err)
	}
}

func TestCachePutUsesNameNotDirBase(t *testing.T) {
	stubProbes(t)
	cache := t.TempDir()
	src := installedTree(t, "src", lockJSON)
	if err := CachePut(src, "contact-center-hub", cacheEnv(cache)); err != nil {
		t.Fatal(err)
	}
	slots, err := filepath.Glob(filepath.Join(cache, "contact-center-hub", "????????????????"))
	if err != nil || len(slots) != 1 {
		t.Fatalf("slots under contact-center-hub = %v (err %v), want exactly 1", slots, err)
	}
	if got := cacheEntries(t, filepath.Join(cache, "src")); len(got) != 0 {
		t.Fatalf("slot written under the dir base name: %v", got)
	}
}

func TestCachePutRefusesStaleTree(t *testing.T) {
	stubProbes(t)
	cache := t.TempDir()
	src := installedTree(t, "src", lockJSON)
	// lock edited after install: tree no longer matches it
	stale := strings.Replace(lockJSON, `"1.0.0"`, `"1.0.1"`, 1)
	if err := os.WriteFile(filepath.Join(src, "package-lock.json"), []byte(stale), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CachePut(src, "myrepo", cacheEnv(cache)); err == nil {
		t.Fatal("want error for a tree that does not match its lock")
	}
	if got := cacheEntries(t, cache); len(got) != 0 {
		t.Fatalf("cache written for a stale tree: %v", got)
	}
}

func TestCachePutRefusesIgnoreScripts(t *testing.T) {
	stubProbes(t)
	probeIgnoreScripts = func(string) (bool, bool) { return true, true }
	cache := t.TempDir()
	src := installedTree(t, "src", lockJSON)
	if err := CachePut(src, "myrepo", cacheEnv(cache)); err == nil {
		t.Fatal("want error when ignore-scripts is on")
	}
	if got := cacheEntries(t, cache); len(got) != 0 {
		t.Fatalf("cache written under ignore-scripts: %v", got)
	}
}

func TestCachePutRefusesWhenProbesFailOrCacheOff(t *testing.T) {
	cache := t.TempDir()
	src := installedTree(t, "src", lockJSON)
	t.Run("env tag unknown", func(t *testing.T) {
		stubProbes(t)
		probeEnvTag = func(string) (string, bool) { return "", false }
		if err := CachePut(src, "myrepo", cacheEnv(cache)); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("ignore-scripts unknown", func(t *testing.T) {
		stubProbes(t)
		probeIgnoreScripts = func(string) (bool, bool) { return false, false }
		if err := CachePut(src, "myrepo", cacheEnv(cache)); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("cache off", func(t *testing.T) {
		stubProbes(t)
		if err := CachePut(src, "myrepo", cacheEnv("off")); err == nil {
			t.Fatal("want error")
		}
	})
	if got := cacheEntries(t, cache); len(got) != 0 {
		t.Fatalf("cache written despite refusal: %v", got)
	}
}

func TestCacheGetMissLeavesDirUntouched(t *testing.T) {
	stubProbes(t)
	cache := t.TempDir()
	if err := CachePut(installedTree(t, "src", lockJSON), "myrepo", cacheEnv(cache)); err != nil {
		t.Fatal(err)
	}
	dst := bareDir(t, strings.Replace(lockJSON, `"p"`, `"q"`, 1))
	hit, err := CacheGet(dst, "myrepo", cacheEnv(cache))
	if err != nil || hit {
		t.Fatalf("CacheGet = (%v, %v), want (false, nil)", hit, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("node_modules created on a miss: %v", err)
	}
	if has, err := CacheHas(dst, "myrepo", cacheEnv(cache)); err != nil || has {
		t.Fatalf("CacheHas = (%v, %v) on a miss, want (false, nil)", has, err)
	}
}

func TestCacheGetReplacesExistingNodeModules(t *testing.T) {
	stubProbes(t)
	cache := t.TempDir()
	if err := CachePut(installedTree(t, "src", lockJSON), "myrepo", cacheEnv(cache)); err != nil {
		t.Fatal(err)
	}
	dst := bareDir(t, lockJSON)
	if err := os.MkdirAll(filepath.Join(dst, "node_modules", "old"), 0o750); err != nil {
		t.Fatal(err)
	}
	if hit, err := CacheGet(dst, "myrepo", cacheEnv(cache)); err != nil || !hit {
		t.Fatalf("CacheGet = (%v, %v)", hit, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "node_modules", "old")); !os.IsNotExist(err) {
		t.Fatalf("old node_modules content survived: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "node_modules", "x", "package.json")); err != nil {
		t.Fatal(err)
	}
}

// writable restores u+w on root's whole tree so t.TempDir removal can succeed.
func writable(t *testing.T, root string) {
	t.Helper()
	t.Cleanup(func() {
		_ = filepath.Walk(root, func(p string, _ os.FileInfo, err error) error {
			if err == nil {
				_ = os.Chmod(p, 0o750)
			}
			return nil
		})
	})
}

func TestCacheGetReadOnlyCache(t *testing.T) {
	stubProbes(t)
	cache := t.TempDir()
	if err := CachePut(installedTree(t, "src", lockJSON), "myrepo", cacheEnv(cache)); err != nil {
		t.Fatal(err)
	}
	writable(t, cache)
	dst := bareDir(t, lockJSON)
	writable(t, dst)
	if err := filepath.Walk(cache, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chmod(p, fi.Mode().Perm()&^0o222)
	}); err != nil {
		t.Fatal(err)
	}
	hit, err := CacheGet(dst, "myrepo", cacheEnv(cache))
	if err != nil || !hit {
		t.Fatalf("CacheGet on read-only cache = (%v, %v), want (true, nil)", hit, err)
	}
}

func TestCachePutIdempotent(t *testing.T) {
	stubProbes(t)
	cache := t.TempDir()
	src := installedTree(t, "src", lockJSON)
	if err := CachePut(src, "myrepo", cacheEnv(cache)); err != nil {
		t.Fatal(err)
	}
	slots, _ := filepath.Glob(filepath.Join(cache, "myrepo", "????????????????"))
	if len(slots) != 1 {
		t.Fatalf("slots = %v, want 1", slots)
	}
	marker := filepath.Join(slots[0], "node_modules", "x", "package.json")
	if err := os.WriteFile(marker, []byte("sentinel"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CachePut(src, "myrepo", cacheEnv(cache)); err != nil {
		t.Fatalf("second put = %v, want nil", err)
	}
	got, err := os.ReadFile(marker)
	if err != nil || string(got) != "sentinel" {
		t.Fatalf("second put rewrote the slot: %q, %v", got, err)
	}
}

// F3: a cache repo name is one path segment, so put/get/has never leave the cache root.
func TestCacheRejectsUnsafeRepoName(t *testing.T) {
	stubProbes(t)
	root := t.TempDir()
	cache := filepath.Join(root, "cache")
	src := installedTree(t, "src", lockJSON)
	dst := bareDir(t, lockJSON)
	for _, name := range []string{"../x", "..", ".", "a/b", "", "-x", ".hidden"} {
		if err := CachePut(src, name, cacheEnv(cache)); err == nil {
			t.Errorf("CachePut(%q) = nil, want error", name)
		}
		if _, err := CacheGet(dst, name, cacheEnv(cache)); err == nil {
			t.Errorf("CacheGet(%q) err = nil, want error", name)
		}
		if _, err := CacheHas(dst, name, cacheEnv(cache)); err == nil {
			t.Errorf("CacheHas(%q) err = nil, want error", name)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "x")); !os.IsNotExist(err) {
		t.Fatalf("put escaped the cache root: %v", err)
	}
	if err := CachePut(src, "contact-center-hub.v2_x", cacheEnv(cache)); err != nil {
		t.Fatalf("safe name refused: %v", err)
	}
}

// F5: a symlinked node_modules root is never cached.
func TestCachePutRefusesSymlinkedNodeModules(t *testing.T) {
	stubProbes(t)
	cache := t.TempDir()
	real := installedTree(t, "real", lockJSON)
	src := installedTree(t, "src", lockJSON)
	if err := os.RemoveAll(filepath.Join(src, "node_modules")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(real, "node_modules"), filepath.Join(src, "node_modules")); err != nil {
		t.Fatal(err)
	}
	if err := CachePut(src, "myrepo", cacheEnv(cache)); err == nil {
		t.Fatal("CachePut on a symlinked node_modules = nil, want error")
	}
	if got := cacheEntries(t, cache); len(got) != 0 {
		t.Fatalf("cache written from a symlink: %v", got)
	}
}

// F5: a slot whose node_modules is a symlink is a miss, not a hit.
func TestCacheSymlinkSlotIsMiss(t *testing.T) {
	stubProbes(t)
	cache := t.TempDir()
	if err := CachePut(installedTree(t, "src", lockJSON), "myrepo", cacheEnv(cache)); err != nil {
		t.Fatal(err)
	}
	slots, _ := filepath.Glob(filepath.Join(cache, "myrepo", "????????????????"))
	if len(slots) != 1 {
		t.Fatalf("slots = %v, want 1", slots)
	}
	nm := filepath.Join(slots[0], "node_modules")
	elsewhere := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.Rename(nm, elsewhere); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, nm); err != nil {
		t.Fatal(err)
	}
	dst := bareDir(t, lockJSON)
	if has, err := CacheHas(dst, "myrepo", cacheEnv(cache)); err != nil || has {
		t.Fatalf("CacheHas = (%v, %v), want (false, nil)", has, err)
	}
	if hit, err := CacheGet(dst, "myrepo", cacheEnv(cache)); err != nil || hit {
		t.Fatalf("CacheGet = (%v, %v), want (false, nil)", hit, err)
	}
}

// F10: a copy that fails partway leaves the existing node_modules intact.
func TestCacheGetFailedCopyKeepsExistingNodeModules(t *testing.T) {
	stubProbes(t)
	cache := t.TempDir()
	if err := CachePut(installedTree(t, "src", lockJSON), "myrepo", cacheEnv(cache)); err != nil {
		t.Fatal(err)
	}
	slots, _ := filepath.Glob(filepath.Join(cache, "myrepo", "????????????????"))
	if len(slots) != 1 {
		t.Fatalf("slots = %v, want 1", slots)
	}
	writable(t, cache)
	unreadable := filepath.Join(slots[0], "node_modules", "x", "package.json")
	if err := os.Chmod(unreadable, 0); err != nil {
		t.Fatal(err)
	}
	dst := bareDir(t, lockJSON)
	keep := filepath.Join(dst, "node_modules", "old", "keep.txt")
	if err := os.MkdirAll(filepath.Dir(keep), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keep, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if hit, err := CacheGet(dst, "myrepo", cacheEnv(cache)); err == nil || hit {
		t.Fatalf("CacheGet = (%v, %v), want a copy error", hit, err)
	}
	if got, err := os.ReadFile(keep); err != nil || string(got) != "keep" {
		t.Fatalf("existing node_modules lost after a failed copy: %q, %v", got, err)
	}
	entries, _ := os.ReadDir(dst)
	for _, e := range entries {
		if e.Name() != "node_modules" && e.Name() != "package-lock.json" {
			t.Errorf("leftover temp entry %q in dst", e.Name())
		}
	}
}

// F11: a prune failure after a successful put only warns; put still succeeds.
func TestCachePutPruneErrorOnlyWarns(t *testing.T) {
	stubProbes(t)
	cache := t.TempDir()
	parent := filepath.Join(cache, "myrepo")
	if err := os.MkdirAll(parent, 0o750); err != nil {
		t.Fatal(err)
	}
	// write+search but no read: populate can create its slot, prune cannot list it.
	if err := os.Chmod(parent, 0o300); err != nil { //nolint:gosec // G302 -- test fixture
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o750) }) //nolint:gosec // G302 -- test fixture
	if err := CachePut(installedTree(t, "src", lockJSON), "myrepo", cacheEnv(cache)); err != nil {
		t.Fatalf("CachePut with a failing prune = %v, want nil", err)
	}
}

// A get killed mid-copy leaves <dir>/.wt-cache-*; the next get removes it so the
// repo dir does not stay dirty. Only that exact prefix is touched.
func TestCacheGetRemovesStaleTempDirs(t *testing.T) {
	stubProbes(t)
	cache := t.TempDir()
	if err := CachePut(installedTree(t, "src", lockJSON), "myrepo", cacheEnv(cache)); err != nil {
		t.Fatal(err)
	}
	dst := bareDir(t, lockJSON)
	for _, d := range []string{".wt-cache-stale/node_modules/x", ".wt-cache-live/node_modules", ".wt-cachex", "wt-cache-keep"} {
		if err := os.MkdirAll(filepath.Join(dst, d), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	// Only a staging dir older than the sweep age is stale; a fresh one may be
	// a concurrent get still copying.
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(dst, ".wt-cache-stale"), old, old); err != nil {
		t.Fatal(err)
	}
	if hit, err := CacheGet(dst, "myrepo", cacheEnv(cache)); err != nil || !hit {
		t.Fatalf("CacheGet = (%v, %v)", hit, err)
	}
	if _, err := os.Stat(filepath.Join(dst, ".wt-cache-stale")); !os.IsNotExist(err) {
		t.Fatalf(".wt-cache-stale survived a get: %v", err)
	}
	for _, keep := range []string{".wt-cache-live", ".wt-cachex", "wt-cache-keep"} {
		if _, err := os.Stat(filepath.Join(dst, keep)); err != nil {
			t.Errorf("%s removed: %v", keep, err)
		}
	}
}

// gitRepoNamed makes <tmp>/<name> a git repo; a non-empty remote becomes origin.
func gitRepoNamed(t *testing.T, name, remote string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	args := [][]string{{"init", "-q"}}
	if remote != "" {
		args = append(args, []string{"remote", "add", "origin", remote})
	}
	for _, a := range args {
		if out, err := exec.Command("git", append([]string{"-C", dir}, a...)...).CombinedOutput(); err != nil { //nolint:gosec // G204 -- fixed git subcommands in a test
			t.Fatalf("git %v: %v\n%s", a, err, out)
		}
	}
	return dir
}

func TestCacheRepoName(t *testing.T) {
	const dirName = "ZenifyAIContactCenter-contact-center-web"
	cases := []struct{ name, remote, want string }{
		{"https with .git", "https://github.com/ZenifyAIContactCenter/contact-center-web.git", "contact-center-web"},
		{"ssh", "git@github.com:ZenifyAIContactCenter/contact-center-web.git", "contact-center-web"},
		{"trailing slash", "https://github.com/ZenifyAIContactCenter/contact-center-web/", "contact-center-web"},
		{"no remote", "", dirName},
		{"invalid name", "https://x/.hidden.git", dirName},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := gitRepoNamed(t, dirName, c.remote)
			if got := CacheRepoName(dir); got != c.want {
				t.Fatalf("CacheRepoName = %q, want %q", got, c.want)
			}
		})
	}
}

func TestCacheRepoNameIgnoresAmbientGitDir(t *testing.T) {
	other := gitRepoNamed(t, "other", "https://x/other-repo.git")
	dir := gitRepoNamed(t, "ZenifyAIContactCenter-contact-center-web", "https://github.com/ZenifyAIContactCenter/contact-center-web.git")
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	if got := CacheRepoName(dir); got != "contact-center-web" {
		t.Fatalf("CacheRepoName = %q, want contact-center-web", got)
	}
}
