package wt

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// cacheDeps is the deps dir the standalone cache commands handle. Only npm
// trees are cached, so unlike ApplyDeps it is not configurable.
const cacheDeps = "node_modules"

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
// write the repo's slots are pruned LRU to cacheSlotCap.
func CachePut(dir, repoName string, lookupEnv func(string) (string, bool)) error {
	cache, enabled := resolveCacheDir(lookupEnv)
	if !enabled {
		return errors.New("wt cache: deps cache is off (ZENIFY_DEPS_CACHE)")
	}
	tag, ok := probeEnvTag(dir)
	if !ok {
		return errors.New("wt cache: cannot read the Node build env (is node on PATH?)")
	}
	nm := filepath.Join(dir, cacheDeps)
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
		return fmt.Errorf("wt cache: prune: %w", err)
	}
	return nil
}

// CacheGet copies the cached node_modules matching dir's package-lock.json and
// the current build env into dir/node_modules, replacing any existing one. A
// miss (no slot, cache off, no readable lock) returns false, nil and leaves dir
// untouched. The cache is only read, never written.
func CacheGet(dir, repoName string, lookupEnv func(string) (string, bool)) (hit bool, err error) {
	src, found, err := cacheLookup(dir, repoName, lookupEnv)
	if err != nil || !found {
		return false, err
	}
	dst := filepath.Join(dir, cacheDeps)
	if err := os.RemoveAll(dst); err != nil {
		return false, fmt.Errorf("wt cache: clear %s: %w", dst, err)
	}
	if err := copyTree(src, dst); err != nil {
		return false, fmt.Errorf("wt cache: copy %s: %w", src, err)
	}
	return true, nil
}

// CacheHas reports whether CacheGet would hit, without copying anything.
func CacheHas(dir, repoName string, lookupEnv func(string) (string, bool)) bool {
	_, found, err := cacheLookup(dir, repoName, lookupEnv)
	return err == nil && found
}

func cacheLookup(dir, repoName string, lookupEnv func(string) (string, bool)) (src string, found bool, err error) {
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
