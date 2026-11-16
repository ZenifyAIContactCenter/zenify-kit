package wt

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// cacheDeps is the deps dir the standalone cache commands handle. Only npm
// trees are cached, so unlike ApplyDeps it is not configurable.
const cacheDeps = "node_modules"

// cacheTempPrefix names CacheGet's staging dir inside the target dir.
const cacheTempPrefix = ".wt-cache-"

// cacheTempStaleAge is how old a staging dir must be before a later get
// removes it; a younger one may belong to a concurrent get still copying.
const cacheTempStaleAge = time.Hour

// cacheNameRe is the one path segment a cache repo name may be, so a name can
// never point outside <cache>/ (no "/", no leading "." or "-").
var cacheNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func checkCacheName(repoName string) error {
	if !cacheNameRe.MatchString(repoName) {
		return fmt.Errorf("wt cache: invalid repo name %q (want [A-Za-z0-9][A-Za-z0-9._-]*)", repoName)
	}
	return nil
}

// CachePut stores dir's installed node_modules in the deps cache, in the same
// slot layout `wt new` uses (<cache>/<repoName>/<lockHash>/{node_modules,
// package-lock.json}), so a later `wt new` or CacheGet for a matching lock hits
// it. repoName names the cache directory (not filepath.Base(dir)), so a sandbox
// that checks the repo out under another name still shares the slot.
//
// It refuses (returns an error) to write when the cache is off, the build env
// cannot be fingerprinted, npm's ignore-scripts is on or unknown, or the tree
// does not provably match its package-lock.json — the same gates ApplyDeps
// applies before it populates. An existing slot is left alone (nil). After a
// write the repo's slots are pruned LRU to cacheSlotCap; a prune failure only
// warns on stderr. A symlinked node_modules is refused. The tree's content is
// trusted: run put only on a tree you installed.
func CachePut(dir, repoName string, lookupEnv func(string) (string, bool)) error {
	if err := checkCacheName(repoName); err != nil {
		return err
	}
	cache, enabled := resolveCacheDir(lookupEnv)
	if !enabled {
		return errors.New("wt cache: deps cache is off (ZENIFY_DEPS_CACHE)")
	}
	tag, ok := probeEnvTag(dir)
	if !ok {
		return errors.New("wt cache: cannot read the Node build env (is node on PATH?)")
	}
	nm := filepath.Join(dir, cacheDeps)
	if fi, err := os.Lstat(nm); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("wt cache: %s is a symlink, refusing to cache it", nm)
	}
	if _, current := cloneIsCurrent(nm, nm, "npm ci"); !current {
		return fmt.Errorf("wt cache: %s does not match its package-lock.json", nm)
	}
	ignored, ok := probeIgnoreScripts(dir)
	if !ok {
		return errors.New("wt cache: cannot read npm's ignore-scripts")
	}
	if ignored {
		return errors.New("wt cache: npm ignore-scripts is on, so the tree has unbuilt deps")
	}
	if err := populateCache(cache, repoName, dir, cacheDeps, tag); err != nil {
		return fmt.Errorf("wt cache: put: %w", err)
	}
	if err := pruneCache(cache, repoName, cacheSlotCap); err != nil {
		fmt.Fprintf(os.Stderr, "wt cache: prune skipped: %v\n", err)
	}
	return nil
}

// CacheGet copies the cached node_modules matching dir's package-lock.json and
// the current build env into dir/node_modules, replacing any existing one. A
// miss (no slot, cache off, no readable lock) returns false, nil and leaves dir
// untouched. The cache is only read, never written. The copy lands in a temp
// dir beside dst first, so a failed copy leaves the existing node_modules as is;
// a staging dir over an hour old (a killed get) is removed on the next hit.
func CacheGet(dir, repoName string, lookupEnv func(string) (string, bool)) (hit bool, err error) {
	src, found, err := cacheLookup(dir, repoName, lookupEnv)
	if err != nil || !found {
		return false, err
	}
	dst := filepath.Join(dir, cacheDeps)
	// A get killed mid-copy leaves its temp dir behind; clear old ones (best
	// effort) so the repo dir does not stay dirty.
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), cacheTempPrefix) {
				continue
			}
			if fi, err := e.Info(); err == nil && time.Since(fi.ModTime()) > cacheTempStaleAge {
				_ = os.RemoveAll(filepath.Join(dir, e.Name()))
			}
		}
	}
	temp, err := os.MkdirTemp(dir, cacheTempPrefix)
	if err != nil {
		return false, fmt.Errorf("wt cache: temp dir in %s: %w", dir, err)
	}
	defer func() { _ = os.RemoveAll(temp) }()
	staged := filepath.Join(temp, cacheDeps)
	if err := copyTree(src, staged); err != nil {
		return false, fmt.Errorf("wt cache: copy %s: %w", src, err)
	}
	// cp keeps a read-only source's modes, and moving a directory to a new
	// parent needs write permission on it (its ".." entry changes).
	if fi, err := os.Stat(staged); err == nil {
		_ = os.Chmod(staged, fi.Mode().Perm()|0o700) //nolint:gosec // G302 -- the copied node_modules root, owner rwx only added
	}
	if err := os.RemoveAll(dst); err != nil {
		return false, fmt.Errorf("wt cache: clear %s: %w", dst, err)
	}
	if err := os.Rename(staged, dst); err != nil {
		return false, fmt.Errorf("wt cache: move into %s: %w", dst, err)
	}
	return true, nil
}

// CacheHas reports whether CacheGet would hit, without copying anything. A
// lookup that cannot run (bad name, Node env unreadable) is an error, not a miss.
func CacheHas(dir, repoName string, lookupEnv func(string) (string, bool)) (bool, error) {
	_, found, err := cacheLookup(dir, repoName, lookupEnv)
	if err != nil {
		return false, err
	}
	return found, nil
}

func cacheLookup(dir, repoName string, lookupEnv func(string) (string, bool)) (src string, found bool, err error) {
	if err := checkCacheName(repoName); err != nil {
		return "", false, err
	}
	if _, enabled := resolveCacheDir(lookupEnv); !enabled {
		return "", false, nil
	}
	tag, ok := probeEnvTag(dir)
	if !ok {
		return "", false, errors.New("wt cache: cannot read the Node build env (is node on PATH?)")
	}
	src, found = depsSource(repoName, "", dir, cacheDeps, tag, lookupEnv)
	return src, found, nil
}
