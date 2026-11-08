package wt

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SeedCopyFiles copies each still-present copy-array target from repoRoot into
// worktreePath, creating parent dirs. A declared target that does not exist in
// the repo is NOT an error (matching bash wt) — it is returned as a warning
// string. Uses `cp -c -R` (copy-on-write where the FS allows) with a plain
// `cp -R` fallback, so large trees stay cheap.
func SeedCopyFiles(repoRoot, worktreePath string, copyList []string) ([]string, error) {
	var warns []string
	for _, f := range copyList {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		src := filepath.Join(repoRoot, f)
		if _, err := os.Stat(src); err != nil {
			warns = append(warns, fmt.Sprintf("declared copy target %q does not exist in the repo root", f))
			continue
		}
		dst := filepath.Join(worktreePath, f)
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			return warns, fmt.Errorf("wt: parent dir for copy target %q: %w", f, err)
		}
		if err := copyTree(src, dst); err != nil {
			return warns, fmt.Errorf("wt: seed copy target %q: %w", f, err)
		}
	}
	return warns, nil
}

// copyTree copies src→dst preferring APFS copy-on-write (`cp -c -R`), falling
// back to a plain recursive copy. Shelling to cp keeps CoW that a Go copy loses.
//
// The first attempt can fail partway through a large tree (ENOSPC, a file
// vanishing mid-walk, a permission on one entry) and leave dst half-populated.
// Because `cp -R src dst` copies INTO dst when dst already exists — producing a
// nested dst/<basename(src)> instead of populating dst — the fallback MUST start
// from the same clean precondition as the first attempt, so remove any partial
// dst first. cp's own stderr is captured into the returned error: without it a
// real failure (disk full, cross-device) surfaces only as "exit status 1".
func copyTree(src, dst string) error {
	if err := exec.Command("cp", "-c", "-R", src, dst).Run(); err == nil { //nolint:gosec // G204 -- fixed trusted binary, args are internally-computed subcommands, not attacker-controlled shell input
		return nil
	}
	// Clear whatever the failed CoW attempt may have left, so the fallback does
	// not nest into an existing dst.
	if err := os.RemoveAll(dst); err != nil {
		return fmt.Errorf("clear partial copy at %s: %w", dst, err)
	}
	var stderr strings.Builder
	cmd := exec.Command("cp", "-R", src, dst) //nolint:gosec // G204 -- fixed trusted binary, args are internally-computed subcommands, not attacker-controlled shell input
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}

// upsertEnvVar sets key=value in envPath: replacing an existing line for that key,
// or appending one (guaranteeing a newline before the appended line). A missing
// file is created with just the single line. This is the replace-or-append core
// WritePortEnv previously inlined.
func upsertEnvVar(envPath, key, value string) error {
	line := key + "=" + value
	b, err := os.ReadFile(envPath) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if os.IsNotExist(err) {
		return os.WriteFile(envPath, []byte(line+"\n"), 0o600)
	}
	if err != nil {
		return err
	}
	content := string(b)
	lines := strings.Split(content, "\n")
	replaced := false
	for i, l := range lines {
		if strings.HasPrefix(l, key+"=") {
			lines[i] = line
			replaced = true
			break
		}
	}
	if replaced {
		return os.WriteFile(envPath, []byte(strings.Join(lines, "\n")), 0o600) //nolint:gosec // G703 -- envPath is the worktree's own env file, computed internally from the repo's configured worktree path, not externally-tainted input
	}
	// append, ensuring exactly one separating newline
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += line + "\n"
	return os.WriteFile(envPath, []byte(content), 0o600) //nolint:gosec // G703 -- envPath is the worktree's own env file, computed internally from the repo's configured worktree path, not externally-tainted input
}

// WritePortEnv sets <portEnv>=<port> in envPath: replacing an existing line for
// that key, or appending one (guaranteeing a newline before the appended line).
// A missing file is created with just the single line.
func WritePortEnv(envPath, portEnv string, port int) error {
	return upsertEnvVar(envPath, portEnv, strconv.Itoa(port))
}

// SeedIdentityEnv writes the FR-038 identity variables into the worktree env file
// so parallel worktrees of one repo do not collide on the shared, stateful infra
// that a per-worktree port cannot isolate (Docker Compose project, Redis/BullMQ
// key space): WT_SLUG, COMPOSE_PROJECT_NAME=<abbrev>-<slug>, and — only when the
// config names the env var to hold it — a Redis key-prefix <abbrev>-<slug>:.
func SeedIdentityEnv(envPath string, cfg *Config, slug string) error {
	compose := slug
	if cfg.Abbrev != "" {
		compose = cfg.Abbrev + "-" + slug
	}
	if err := upsertEnvVar(envPath, "WT_SLUG", slug); err != nil {
		return err
	}
	if err := upsertEnvVar(envPath, "COMPOSE_PROJECT_NAME", compose); err != nil {
		return err
	}
	if cfg.RedisPrefixEnv != "" {
		if err := upsertEnvVar(envPath, cfg.RedisPrefixEnv, compose+":"); err != nil {
			return err
		}
	}
	return nil
}

// ApplyDeps sets up the dependency dir (e.g. node_modules) in the worktree per
// mode: "symlink" links to the main checkout's dir (never touches the deps
// cache); "clone" CoW-copies it then runs install, unless cloneIsCurrent says
// the copy already matches the worktree's package-lock.json (then only the root
// lifecycle scripts run); "install" just runs install. symlink/clone require the
// source dir to exist in repoRoot (or a cache slot, for clone). installCmd empty
// → install step is skipped.
//
// Clone mode only: the source is the default-on deps cache
// (<cache>/<repo>/<hash>/) when a slot matches the worktree's package-lock.json
// AND the current build env (slots are keyed by lock bytes + Node ABI/platform/
// arch — see depsSource and probeEnvTag). The env tag is probed once here; if it
// cannot be read (no node) the cache is disabled for this run (neither read nor
// written), never collapsed onto an empty tag. A slot is written only after a
// real npm-ci install whose tree is provably current (branch (b)) — never from
// branch (a), whose source env is unknown — and only when npm's ignore-scripts
// is provably off. On a write the repo's slots are pruned LRU to cacheSlotCap.
func ApplyDeps(repoRoot, worktreePath, mode, depsDirName, installCmd string) error {
	dst := filepath.Join(worktreePath, depsDirName)
	src := filepath.Join(repoRoot, depsDirName)
	switch mode {
	case "symlink":
		if _, err := os.Stat(src); err != nil {
			return fmt.Errorf("wt: deps=symlink but %s does not exist — install in the main checkout first", src)
		}
		return os.Symlink(src, dst)
	case "clone":
		// The env tag joins the slot key; probe it once. Without it (no node) the
		// cache is disabled for this run — src stays the main checkout, nothing is
		// read or written — rather than collapsing every env onto an empty tag.
		tag, tagOK := probeEnvTag()
		if tagOK {
			src, _ = depsSource(repoRoot, worktreePath, depsDirName, tag, os.LookupEnv)
		}
		if _, err := os.Stat(src); err != nil {
			return fmt.Errorf("wt: deps=clone but %s does not exist — install in the main checkout first", src)
		}
		if err := copyTree(src, dst); err != nil {
			return fmt.Errorf("wt: clone %s: %w", depsDirName, err)
		}
		// current: the just-installed worktree tree is provably what a plain
		// install of its lock yields. Only such a tree, built in THIS env, may
		// seed the cache (FR-2.1). A clone that was already current (branch (a))
		// was copied from the main checkout or an existing slot, whose build env
		// is unknown, so it must never seed a slot (zenie #1/#3).
		current := false
		if scripts, ok := cloneIsCurrent(src, dst, installCmd); ok {
			if err := runLifecycleOnly(worktreePath, src, installCmd, scripts); err != nil {
				return err
			}
			// branch (a): do NOT populate — src env unknown.
		} else {
			if err := runInstall(worktreePath, installCmd); err != nil {
				return err
			}
			// npm ci can exit 0 with a partial tree (omit=dev, NODE_ENV=production):
			// judge the installed tree's content, not the command name.
			_, current = cloneIsCurrent(dst, dst, installCmd)
		}
		// Populate only from branch (b), only when the env tag is known, and only
		// when npm's ignore-scripts is provably off (an ignore-scripts tree has
		// unbuilt deps yet passes cloneIsCurrent — zenie #2). Serving a full tree
		// TO an ignore-scripts worktree is harmless, so only the WRITE is guarded.
		if current && tagOK {
			if ignored, ok := probeIgnoreScripts(worktreePath); ok && !ignored {
				if dir, enabled := resolveCacheDir(os.LookupEnv); enabled {
					if err := populateCache(dir, repoRoot, worktreePath, depsDirName, tag); err != nil {
						fmt.Fprintf(os.Stderr, "wt: deps cache populate skipped: %v\n", err)
					} else if err := pruneCache(dir, repoRoot, cacheSlotCap); err != nil {
						fmt.Fprintf(os.Stderr, "wt: deps cache prune skipped: %v\n", err)
					}
				}
			}
		}
		return nil
	case "install":
		return runInstall(worktreePath, installCmd)
	case "none":
		// Repo uses a global dependency cache (e.g. Maven ~/.m2): there is no
		// per-project deps dir to clone/symlink, and no per-worktree install step.
		return nil
	default:
		return fmt.Errorf("wt: unknown deps mode: %s", mode)
	}
}

// cacheSlotCap is the number of lock-hash slots kept per repo in the deps cache.
const cacheSlotCap = 5

// probeEnvTag fingerprints the build env: node ABI + platform + arch. A cached
// tree built under one Node ABI is wrong for another (native modules compiled
// against a different ABI crash), so the tag joins the slot key. A package-level
// var so tests can override it. ok=false when node cannot be run — the caller
// then disables the cache for that run rather than keying everything under one
// tag.
var probeEnvTag = func() (string, bool) {
	out, err := exec.Command("node", "-p", "process.versions.modules+'-'+process.platform+'-'+process.arch").Output() //nolint:gosec // G204 -- fixed trusted binary, no external input
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

// probeIgnoreScripts reports npm's effective ignore-scripts (env or .npmrc) for
// the worktree. A tree installed with ignore-scripts=true has unbuilt deps yet
// passes cloneIsCurrent, so it must never seed the cache; ok=false when npm
// cannot report it. A package-level var so tests can override it.
var probeIgnoreScripts = func(worktreePath string) (ignored bool, ok bool) {
	cmd := exec.Command("npm", "config", "get", "ignore-scripts") //nolint:gosec // G204 -- fixed trusted binary, no external input
	cmd.Dir = worktreePath
	out, err := cmd.Output()
	if err != nil {
		return false, false
	}
	return strings.TrimSpace(string(out)) == "true", true
}

// resolveCacheDir applies the ZENIFY_DEPS_CACHE policy. Unset: the cache is on at
// $XDG_CACHE_HOME/zenify/wt-deps, else $HOME/.cache/zenify/wt-deps (no HOME: off).
// Set to "" or "off" (case-insensitive): off. Any other value is the cache dir.
// lookupEnv is injected so callers/tests need not touch the real environment.
func resolveCacheDir(lookupEnv func(string) (string, bool)) (dir string, enabled bool) {
	if v, ok := lookupEnv("ZENIFY_DEPS_CACHE"); ok {
		if v == "" || strings.EqualFold(strings.TrimSpace(v), "off") {
			return "", false
		}
		return v, true
	}
	if xdg, ok := lookupEnv("XDG_CACHE_HOME"); ok && xdg != "" {
		return filepath.Join(xdg, "zenify", "wt-deps"), true
	}
	if home, ok := lookupEnv("HOME"); ok && home != "" {
		return filepath.Join(home, ".cache", "zenify", "wt-deps"), true
	}
	return "", false
}

// lockHash returns the first 16 hex chars of the sha256 of the lock file's bytes
// joined (0x00 separator) with the env tag, so a tree built under one Node
// ABI/platform/arch keys a different slot than one built under another (see
// probeEnvTag). The 16-hex shape is unchanged, so isSlotName and prune still
// apply.
func lockHash(lockPath, tag string) (string, error) {
	b, err := os.ReadFile(lockPath) //nolint:gosec // G304 -- path is a lock file inside a worktree/cache dir this tool manages
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write(b)
	h.Write([]byte{0})
	h.Write([]byte(tag))
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// isPlainNpmInstall reports whether installCmd is exactly "npm ci" / "npm install"
// / "npm i" (whitespace-normalised): a custom command may do more than install.
func isPlainNpmInstall(installCmd string) bool {
	switch strings.Join(strings.Fields(installCmd), " ") {
	case "npm ci", "npm install", "npm i":
		return true
	}
	return false
}

// populateCache copies the worktree's deps dir and package-lock.json into the
// slot <cacheDir>/<repo>/<lockHash(lock, tag)> (tag = the build-env fingerprint,
// so a tree keys a slot distinct from one built under another Node ABI). It is
// idempotent (an existing slot is left alone), atomic (copy into a temp dir, then
// rename, so a reader never sees a partial slot) and loses a populate race
// silently. A worktree with no readable lock cannot be keyed and is skipped. The
// sole caller gates it on a content check of the just-installed worktree tree
// (cloneIsCurrent(dst, dst, ...)) on branch (b) only, with ignore-scripts off,
// so neither a stale, a partial, nor an unbuilt tree ever enters.
func populateCache(cacheDir, repoRoot, worktreePath, depsDirName, tag string) error {
	wtLock := filepath.Join(worktreePath, "package-lock.json")
	hash, err := lockHash(wtLock, tag)
	if err != nil {
		return nil
	}
	parent := filepath.Join(cacheDir, filepath.Base(repoRoot))
	slot := filepath.Join(parent, hash)
	if fi, err := os.Stat(filepath.Join(slot, depsDirName)); err == nil && fi.IsDir() {
		return nil
	}
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return err
	}
	temp, err := os.MkdirTemp(parent, ".tmp-")
	if err != nil {
		return err
	}
	if err := copyTree(filepath.Join(worktreePath, depsDirName), filepath.Join(temp, depsDirName)); err != nil {
		_ = os.RemoveAll(temp)
		return err
	}
	lock, err := os.ReadFile(wtLock) //nolint:gosec // G304 -- path is the worktree this tool created
	if err == nil {
		err = os.WriteFile(filepath.Join(temp, "package-lock.json"), lock, 0o600) //nolint:gosec // G703 -- temp is a MkdirTemp dir inside the cache dir this tool manages, path computed internally, not externally-tainted
	}
	if err != nil {
		_ = os.RemoveAll(temp)
		return err
	}
	if err := os.Rename(temp, slot); err != nil {
		// the slot appeared meanwhile: another wt won the race, which is fine
		_ = os.RemoveAll(temp)
	}
	return nil
}

// isSlotName reports whether name is a cache slot: exactly 16 lowercase hex chars
// (a lockHash). It rejects the legacy flat "node_modules", ".tmp-*" and uppercase.
func isSlotName(name string) bool {
	if len(name) != 16 {
		return false
	}
	for _, c := range name {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// pruneCache keeps at most max slots under <cacheDir>/<repo>, removing the
// least recently used (oldest mtime) first. Only genuine slot dirs are touched;
// an entry whose Info() fails is skipped (conservative: never deleted).
func pruneCache(cacheDir, repoRoot string, max int) error {
	parent := filepath.Join(cacheDir, filepath.Base(repoRoot))
	entries, err := os.ReadDir(parent)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	type slot struct {
		name string
		mod  time.Time
	}
	var slots []slot
	for _, e := range entries {
		if !e.IsDir() || !isSlotName(e.Name()) {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		slots = append(slots, slot{e.Name(), fi.ModTime()})
	}
	if len(slots) <= max {
		return nil
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i].mod.Before(slots[j].mod) })
	for _, s := range slots[:len(slots)-max] {
		if err := os.RemoveAll(filepath.Join(parent, s.name)); err != nil {
			return err
		}
	}
	return nil
}

// depsSource picks where clone reads the deps dir from (symlink never uses the
// cache). The cache is on by default at $XDG_CACHE_HOME/zenify/wt-deps (else
// ~/.cache/zenify/wt-deps); ZENIFY_DEPS_CACHE=<path> moves it, =off or empty
// disables it. The slot key is lockHash(worktree package-lock.json, tag) — the
// lock bytes AND the build-env fingerprint tag, so a slot built under one Node
// ABI never matches a lookup under another. With the cache off, no such slot
// <cache>/<repo>/<hash>/<depsDirName>, or a slot whose package-lock.json is not
// byte-equal to the worktree's (hash collision), it is the repo root's dir
// (fromCache=false) — silently, a miss is normal. On a hit it touches the slot's
// mtime (LRU) and returns the slot's dir (fromCache=true).
func depsSource(repoRoot, worktreePath, depsDirName, tag string, lookupEnv func(string) (string, bool)) (src string, fromCache bool) {
	own := filepath.Join(repoRoot, depsDirName)
	dir, enabled := resolveCacheDir(lookupEnv)
	if !enabled {
		return own, false
	}
	wtLock := filepath.Join(worktreePath, "package-lock.json")
	hash, err := lockHash(wtLock, tag)
	if err != nil {
		return own, false
	}
	slot := filepath.Join(dir, filepath.Base(repoRoot), hash)
	cached := filepath.Join(slot, depsDirName)
	if fi, err := os.Stat(cached); err != nil || !fi.IsDir() {
		return own, false
	}
	a, errA := os.ReadFile(filepath.Join(slot, "package-lock.json")) //nolint:gosec // G304 -- path built from operator-set cache dir and repo basename
	b, errB := os.ReadFile(wtLock)                                   //nolint:gosec // G304 -- path is the worktree this tool created
	if errA != nil || errB != nil || !bytes.Equal(a, b) {
		return own, false
	}
	now := time.Now()
	_ = os.Chtimes(slot, now, now)
	return cached, true
}

// cloneIsCurrent reports whether the cloned deps dir (src copied to dst)
// already is what installCmd would produce, so the dependency install can be
// skipped. It returns the root lifecycle scripts npm would still run (in
// order), which the caller runs instead: e.g. "prepare": "husky" sets up git
// hooks in the worktree. Every condition must hold, else ok=false:
//   - installCmd is plain "npm ci" / "npm install" / "npm i" (a custom command
//     may do more than install) and package.json has no "install" script;
//   - no npm-shrinkwrap.json (npm would prefer it over package-lock.json);
//   - the worktree's package-lock.json is byte-equal to the one beside src, and
//     its root entry lists the same dependencies as the worktree's package.json
//     (npm ci would reject a lock out of sync with package.json);
//   - npm's hidden lockfile src/.package-lock.json (what was actually installed)
//     agrees with the lock: every installed package is in the lock with the
//     same version, resolved and integrity, and is present on disk; every lock
//     package not installed is optional and excluded on this platform by its
//     os/cpu/libc; no linked (workspace) package.
//
// Content, not mtime, so a cache filled by copy is judged the same way.
func cloneIsCurrent(src, dst, installCmd string) (scripts []string, ok bool) {
	if !isPlainNpmInstall(installCmd) {
		return nil, false
	}
	srcRoot, wtRoot := filepath.Dir(src), filepath.Dir(dst)
	if _, err := os.Stat(filepath.Join(wtRoot, "npm-shrinkwrap.json")); err == nil {
		return nil, false
	}
	a, errA := os.ReadFile(filepath.Join(srcRoot, "package-lock.json")) //nolint:gosec // G304 -- path built from the repo root or operator-set cache dir
	b, errB := os.ReadFile(filepath.Join(wtRoot, "package-lock.json"))  //nolint:gosec // G304 -- path is the worktree this tool created
	pj, errP := os.ReadFile(filepath.Join(wtRoot, "package.json"))      //nolint:gosec // G304 -- path is the worktree this tool created
	h, errH := os.ReadFile(filepath.Join(src, ".package-lock.json"))    //nolint:gosec // G304 -- path built from the repo root or operator-set cache dir
	if errA != nil || errB != nil || errP != nil || errH != nil || !bytes.Equal(a, b) {
		return nil, false
	}
	var manifest struct {
		Scripts map[string]string `json:"scripts"`
		depFields
	}
	var lock, hidden struct {
		Packages map[string]lockPkg `json:"packages"`
	}
	if json.Unmarshal(pj, &manifest) != nil || json.Unmarshal(a, &lock) != nil || json.Unmarshal(h, &hidden) != nil {
		return nil, false
	}
	root, hasRoot := lock.Packages[""]
	if !hasRoot || !manifest.equal(root.depFields) {
		return nil, false
	}
	// install/prepublish are run by npm ci but not replayed here; the other
	// four are hooks `npm run` would add that npm ci never calls. preprepare/
	// postprepare without prepare cannot be reached through `npm run prepare`.
	_, hasPrepare := manifest.Scripts["prepare"]
	for _, sc := range []string{"install", "prepublish", "prepreinstall", "postpreinstall", "prepostinstall", "postpostinstall", "preprepare", "postprepare"} {
		if _, has := manifest.Scripts[sc]; has && (!strings.HasSuffix(sc, "prepare") || !hasPrepare) {
			return nil, false
		}
	}
	for k, hp := range hidden.Packages {
		lp, inLock := lock.Packages[k]
		if k == "" {
			continue
		}
		if hp.Link || !inLock || lp.Version != hp.Version || lp.Resolved != hp.Resolved || lp.Integrity != hp.Integrity {
			return nil, false
		}
		if _, err := os.Stat(filepath.Join(srcRoot, filepath.FromSlash(k))); err != nil {
			return nil, false
		}
	}
	for k, lp := range lock.Packages {
		if k == "" {
			continue
		}
		if lp.Link {
			return nil, false
		}
		if _, installed := hidden.Packages[k]; !installed && (!lp.Optional || platformAllows(lp)) {
			return nil, false
		}
	}
	for _, sc := range lifecycleScripts {
		if _, has := manifest.Scripts[sc]; has {
			scripts = append(scripts, sc)
		}
	}
	return scripts, true
}

// lifecycleScripts are the root scripts npm ci runs after installing, in its
// order. `npm run prepare` also runs preprepare/postprepare, as npm ci does.
var lifecycleScripts = []string{"preinstall", "postinstall", "prepare"}

// runLifecycleOnly replaces installCmd for a clone cloneIsCurrent accepted: it
// runs only the root lifecycle scripts npm ci would have run, each through a
// plain `npm run <name>` so npm commands nested inside a script keep the user's
// npm config (cloneIsCurrent already refused hooks npm ci would not run). When
// npm's effective ignore-scripts (env or .npmrc) is true, npm ci would run none,
// so none run here; if npm cannot report it, the full installCmd runs instead.
func runLifecycleOnly(worktreePath, src, installCmd string, scripts []string) error {
	fmt.Fprintf(os.Stderr, "wt: package-lock.json unchanged and %s matches it; skipping %q\n", src, installCmd)
	if len(scripts) == 0 {
		return nil
	}
	cmd := exec.Command("npm", "config", "get", "ignore-scripts")
	cmd.Dir = worktreePath
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "wt: npm config get ignore-scripts failed (%v); running %q\n", err, installCmd)
		return runInstall(worktreePath, installCmd)
	}
	if strings.TrimSpace(string(out)) == "true" {
		return nil
	}
	for _, sc := range scripts {
		if err := runInstall(worktreePath, "npm run "+sc); err != nil {
			return fmt.Errorf("wt: npm run %s: %w", sc, err)
		}
	}
	return nil
}

// depFields are the package.json dependency maps npm copies into the lock's
// root entry; npm ci refuses to install when they disagree.
type depFields struct {
	Dependencies         map[string]string `json:"dependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	PeerDependencies     map[string]string `json:"peerDependencies"`
}

func (d depFields) equal(o depFields) bool {
	eq := func(x, y map[string]string) bool {
		if len(x) != len(y) {
			return false
		}
		for k, v := range x {
			if w, ok := y[k]; !ok || w != v {
				return false
			}
		}
		return true
	}
	return eq(d.Dependencies, o.Dependencies) && eq(d.DevDependencies, o.DevDependencies) &&
		eq(d.OptionalDependencies, o.OptionalDependencies) && eq(d.PeerDependencies, o.PeerDependencies)
}

// lockPkg is one entry of a package-lock.json / hidden lockfile "packages" map.
type lockPkg struct {
	Version   string   `json:"version"`
	Resolved  string   `json:"resolved"`
	Integrity string   `json:"integrity"`
	Optional  bool     `json:"optional"`
	Link      bool     `json:"link"`
	OS        []string `json:"os"`
	CPU       []string `json:"cpu"`
	Libc      []string `json:"libc"`
	depFields
}

// platformAllows reports whether npm would install p on this machine, judged by
// its os/cpu/libc lists (npm names: darwin/linux/win32, x64/arm64/ia32). A
// package with none of them is allowed everywhere.
func platformAllows(p lockPkg) bool {
	goos := runtime.GOOS
	if goos == "windows" {
		goos = "win32"
	}
	arch := map[string]string{"amd64": "x64", "386": "ia32"}[runtime.GOARCH]
	if arch == "" {
		arch = runtime.GOARCH
	}
	libc := ""
	if runtime.GOOS == "linux" {
		libc = "glibc"
		if m, _ := filepath.Glob("/lib/ld-musl-*"); len(m) > 0 {
			libc = "musl"
		}
	}
	return npmListAllows(p.OS, goos) && npmListAllows(p.CPU, arch) && (libc == "" || npmListAllows(p.Libc, libc))
}

// npmListAllows applies npm's os/cpu/libc list rule: "!x" excludes x; any
// positive entry means v must be listed; an empty list allows everything.
func npmListAllows(list []string, v string) bool {
	positive := false
	for _, e := range list {
		if strings.HasPrefix(e, "!") {
			if e[1:] == v {
				return false
			}
			continue
		}
		positive = true
	}
	if !positive {
		return true
	}
	for _, e := range list {
		if e == v {
			return true
		}
	}
	return false
}

// runInstall runs installCmd inside dir via the shell. An empty command is a
// no-op (matching bash wt's run_install, which skips when none is declared).
func runInstall(dir, installCmd string) error {
	if strings.TrimSpace(installCmd) == "" {
		return nil
	}
	cmd := exec.Command("sh", "-c", installCmd) //nolint:gosec // G204 -- fixed trusted binary, args are internally-computed subcommands, not attacker-controlled shell input
	cmd.Dir = dir
	cmd.Stdout = os.Stderr // install chatter goes to stderr, never stdout (path contract)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
