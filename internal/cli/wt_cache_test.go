package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

// Minimal npm project whose node_modules provably matches its lock (hand-written
// literals, independent of the wt package's own fixtures).
const (
	cliLock   = `{"packages":{"":{"name":"p","dependencies":{"x":"^1.0.0"}},"node_modules/x":{"version":"1.0.0","resolved":"https://r/x-1.0.0.tgz","integrity":"sha512-x"}}}`
	cliHidden = `{"packages":{"node_modules/x":{"version":"1.0.0","resolved":"https://r/x-1.0.0.tgz","integrity":"sha512-x"}}}`
	cliPkg    = `{"name":"p","dependencies":{"x":"^1.0.0"}}`
)

func cacheWriteFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for p, v := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(v), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// cacheCLIFixture fakes node/npm on PATH (the real probes shell out to them),
// points the cache at a temp dir, and returns an installed source dir and a
// consumer dir holding only the same lock.
func cacheCLIFixture(t *testing.T) (src, dst string) {
	t.Helper()
	bin := t.TempDir()
	cacheWriteFiles(t, bin, map[string]string{
		"node": "#!/bin/sh\necho abi-test\n",
		"npm":  "#!/bin/sh\necho false\n",
	})
	for _, n := range []string{"node", "npm"} {
		if err := os.Chmod(filepath.Join(bin, n), 0o700); err != nil { //nolint:gosec // G302 -- test stub must be executable
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("ZENIFY_DEPS_CACHE", t.TempDir())
	src = filepath.Join(t.TempDir(), "src")
	cacheWriteFiles(t, src, map[string]string{
		"package.json":                    cliPkg,
		"package-lock.json":               cliLock,
		"node_modules/.package-lock.json": cliHidden,
		"node_modules/x/package.json":     `{"name":"x"}`,
	})
	dst = filepath.Join(t.TempDir(), "consumer")
	cacheWriteFiles(t, dst, map[string]string{"package-lock.json": cliLock})
	return src, dst
}

func TestWtCacheGetExitCodes(t *testing.T) {
	src, dst := cacheCLIFixture(t)
	// nothing cached yet: miss exits 1
	_, err := runWt(t, t.TempDir(), "cache", "get", dst, "--name", "myrepo")
	if err == nil || exitcode.Code(err) != 1 {
		t.Fatalf("miss: err = %v, want exit code 1", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("miss touched dst: %v", err)
	}
	if _, err := runWt(t, t.TempDir(), "cache", "put", src, "--name", "myrepo"); err != nil {
		t.Fatalf("put: %v", err)
	}
	if _, err := runWt(t, t.TempDir(), "cache", "get", dst, "--name", "myrepo"); err != nil {
		t.Fatalf("hit: %v, want exit 0", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "node_modules", "x", "package.json")); err != nil {
		t.Fatalf("hit did not restore node_modules: %v", err)
	}
}

func TestWtCacheGetCheckCopiesNothing(t *testing.T) {
	src, dst := cacheCLIFixture(t)
	if _, err := runWt(t, t.TempDir(), "cache", "put", src, "--name", "myrepo"); err != nil {
		t.Fatalf("put: %v", err)
	}
	if _, err := runWt(t, t.TempDir(), "cache", "get", dst, "--name", "myrepo", "--check"); err != nil {
		t.Fatalf("--check on a hit: %v, want exit 0", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("--check copied node_modules: %v", err)
	}
	_, err := runWt(t, t.TempDir(), "cache", "get", dst, "--name", "other", "--check")
	if exitcode.Code(err) != 1 {
		t.Fatalf("--check on a miss: err = %v, want exit code 1", err)
	}
}
