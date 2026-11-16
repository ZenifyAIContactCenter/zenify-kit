package wt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if !CacheHas(dst, "myrepo", cacheEnv(cache)) {
		t.Fatal("CacheHas = false, want true")
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
	if CacheHas(dst, "myrepo", cacheEnv(cache)) {
		t.Fatal("CacheHas = true on a miss")
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
