package e2e

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

const (
	pinnedPlaywrightVersion = "1.64.1"
	defaultLockWait         = 120 * time.Second
	lockPoll                = 500 * time.Millisecond
	// staleLockAge is how long an empty lock file — the O_EXCL lock of older zenify
	// binaries, which hold no OS lock — counts as held; older ones are abandoned.
	staleLockAge = 10 * time.Minute
)

// LoginOptions configures Login. Credentials are read only through Getenv by the
// spawned process (inherited env); they never pass through Run's args or extraEnv.
type LoginOptions struct {
	Home     string
	BaseURL  string
	Role     string // "" = state.json; else state-<role>.json (broker E2E role)
	Getenv   func(string) string
	Run      func(dir, name string, args []string, extraEnv []string) error
	Stdout   io.Writer
	LockWait time.Duration // 0 = 120s
}

func playwrightDir(home string) string { return filepath.Join(home, ".zenify", "playwright") }

// StatePath is where the Playwright storage state is written.
func StatePath(home string) string { return StatePathRole(home, "") }

// StatePathRole is the storage state of one E2E role: state.json for "", else
// state-<role>.json.
func StatePathRole(home, role string) string {
	name := "state.json"
	if role != "" {
		name = "state-" + role + ".json"
	}
	return filepath.Join(playwrightDir(home), name)
}

// Locks this process holds, for InheritLock.
var (
	heldMu    sync.Mutex
	heldLocks []*os.File
)

// acquireLock takes an OS lock (flock; LockFileEx on windows) on path and returns the
// open file holding it. The OS releases it when every process sharing that open file
// is gone, so a killed run never leaves a held lock behind. The file holds our PID.
// Older binaries lock by an empty O_EXCL file: an existing empty file younger than
// staleLockAge is theirs, so we wait as they would.
func acquireLock(path string, wait time.Duration) (*os.File, error) {
	deadline := time.Now().Add(wait)
	for {
		f, err := takeLockFile(path)
		if err != nil || f != nil {
			return f, err
		}
		if time.Now().After(deadline) {
			return nil, errors.New("harness locked by another e2e login or run")
		}
		time.Sleep(lockPoll)
	}
}

// afterLockFileOpen is a test seam (nil in production): it runs between opening the
// lock file and locking it, where an unlink by the previous holder can land.
var afterLockFileOpen func()

// takeLockFile makes one attempt: (file, nil) holding the lock, (nil, nil) busy.
func takeLockFile(path string) (*os.File, error) {
	created := true
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600) //nolint:gosec // G304 -- path built from home
	if errors.Is(err, os.ErrExist) {
		created = false
		f, err = os.OpenFile(path, os.O_RDWR, 0o600) //nolint:gosec // G304 -- path built from home
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil // removed between the two opens: retry
		}
	}
	if err != nil {
		return nil, err
	}
	if afterLockFileOpen != nil {
		afterLockFileOpen()
	}
	if created {
		// Never leave our own file empty: an empty file reads as an older binary's lock.
		_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())), 0)
	}
	busy, err := tryLockFile(f)
	if err != nil || busy {
		_ = f.Close()
		return nil, err
	}
	if !created && legacyHeld(f) || !samePath(f, path) {
		// An older binary holds it, or the file we locked was unlinked by the
		// holder before us (unlock removes it): retry on the path's current file.
		_ = unlockFile(f)
		_ = f.Close()
		return nil, nil
	}
	if err := f.Truncate(0); err == nil {
		_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())), 0)
	}
	return f, nil
}

// legacyHeld: an empty lock file younger than staleLockAge (older binaries' lock).
func legacyHeld(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Size() == 0 && time.Since(st.ModTime()) < staleLockAge
}

// samePath reports whether path still names the open file f.
func samePath(f *os.File, path string) bool {
	fs, err := f.Stat()
	if err != nil {
		return false
	}
	ps, err := os.Stat(path)
	return err == nil && os.SameFile(fs, ps)
}

// PrepareHarness writes the harness and installs deps + chromium, under the lock.
// The chromium install is skipped when PLAYWRIGHT_BROWSERS_PATH already holds the
// revision Playwright expects (an offline worker image baked it at build time).
func PrepareHarness(o LoginOptions, root, harness string) error {
	unlock, err := lockHarness(root, o.LockWait)
	if err != nil {
		return err
	}
	defer unlock()
	return prepareHarness(o, harness)
}

// lockHarness takes root/harness.lock (wait 0 = 120s) and returns its release.
func lockHarness(root string, wait time.Duration) (func(), error) {
	if wait == 0 {
		wait = defaultLockWait
	}
	path := filepath.Join(root, "harness.lock")
	f, err := acquireLock(path, wait)
	if err != nil {
		return nil, err
	}
	heldMu.Lock()
	heldLocks = append(heldLocks, f)
	heldMu.Unlock()
	return func() {
		heldMu.Lock()
		heldLocks = slices.DeleteFunc(heldLocks, func(h *os.File) bool { return h == f })
		heldMu.Unlock()
		// Remove first, then unlock: a waiter that gets the old file sees it unlinked
		// and retries. Older binaries wait while the file exists.
		if removeOnUnlock && samePath(f, path) {
			_ = os.Remove(path)
		}
		_ = unlockFile(f)
		_ = f.Close()
	}, nil
}

// prepareHarness is PrepareHarness without the lock; the caller holds it.
func prepareHarness(o LoginOptions, harness string) error {
	if err := WriteHarness(harness); err != nil {
		return err
	}
	pkg := filepath.Join(harness, "node_modules", "@playwright", "test", "package.json")
	if !pinnedInstalled(pkg) {
		if err := o.Run(harness, "npm", []string{"install", "--no-audit", "--no-fund", "--silent"}, nil); err != nil {
			return fmt.Errorf("npm install: %w", err)
		}
	}
	if hasChromium(os.Getenv("PLAYWRIGHT_BROWSERS_PATH"), harness) {
		return nil
	}
	if err := o.Run(harness, "npx", []string{"playwright", "install", "chromium"}, nil); err != nil {
		return fmt.Errorf("playwright install: %w", err)
	}
	return nil
}

// pinnedInstalled reports whether the package.json at pkg is the pinned Playwright.
func pinnedInstalled(pkg string) bool {
	b, err := os.ReadFile(pkg) //nolint:gosec // G304 -- path built from home
	if err != nil {
		return false
	}
	var v struct {
		Version string `json:"version"`
	}
	return json.Unmarshal(b, &v) == nil && v.Version == pinnedPlaywrightVersion
}

// hasChromium reports whether dir (a PLAYWRIGHT_BROWSERS_PATH) holds the chromium and
// chromium-headless-shell revisions the harness's Playwright expects, fully installed. Unreadable browsers.json = false.
func hasChromium(dir, harness string) bool {
	if dir == "" {
		return false
	}
	b, err := os.ReadFile(filepath.Join(harness, "node_modules", "playwright-core", "browsers.json")) //nolint:gosec // G304 -- path built from home
	if err != nil {
		return false
	}
	var v struct {
		Browsers []struct {
			Name     string `json:"name"`
			Revision string `json:"revision"`
		} `json:"browsers"`
	}
	if json.Unmarshal(b, &v) != nil {
		return false
	}
	// Headless runs launch chromium-headless-shell, which lives in its own dir.
	need := map[string]string{"chromium": "chromium-", "chromium-headless-shell": "chromium_headless_shell-"}
	found := 0
	for _, br := range v.Browsers {
		prefix, ok := need[br.Name]
		if !ok || br.Revision == "" {
			continue
		}
		// The installer writes this marker last; a dir without it is a half-extracted download.
		if _, err := os.Stat(filepath.Join(dir, prefix+br.Revision, "INSTALLATION_COMPLETE")); err != nil { //nolint:gosec // G703 -- dir is the browsers path we chose, revision comes from the harness's own browsers.json
			return false
		}
		found++
	}
	return found == len(need)
}

// CheckLoopbackURL refuses any origin that is not a local app: the harness types
// real credentials into whatever page it is pointed at.
func CheckLoopbackURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return exitcode.New(exitcode.BadArgs, fmt.Errorf("login needs an http(s) origin of a local app; got %q", raw))
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return exitcode.New(exitcode.BadArgs, fmt.Errorf("login only targets a local app (localhost/loopback); got host %s", host))
}

// Login runs the harness setup project on the host and atomically writes the
// storage state. It returns the state path.
func Login(o LoginOptions) (string, error) {
	if o.Role != "" && !slices.Contains(api.E2ERoles, o.Role) {
		return "", exitcode.New(exitcode.BadArgs, fmt.Errorf("unknown role %q", o.Role))
	}
	if err := CheckLoopbackURL(o.BaseURL); err != nil {
		return "", err
	}
	for _, k := range []string{"E2E_DOMAIN", "E2E_EMAIL", "E2E_PASSWORD"} {
		if o.Getenv(k) == "" {
			return "", exitcode.New(exitcode.BadArgs, fmt.Errorf("missing env: %s", k))
		}
	}
	root := playwrightDir(o.Home)
	harness := filepath.Join(root, "harness")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", exitcode.New(exitcode.Fail, err)
	}
	// Hold harness.lock through the setup run too: `e2e run --local` rewrites the harness.
	unlock, err := lockHarness(root, o.LockWait)
	if err != nil {
		return "", exitcode.New(exitcode.Fail, err)
	}
	defer unlock()
	if err := prepareHarness(o, harness); err != nil {
		return "", exitcode.New(exitcode.Fail, err)
	}
	f, err := os.CreateTemp(root, "state-*.json.tmp")
	if err != nil {
		return "", exitcode.New(exitcode.Fail, err)
	}
	tmp := f.Name()
	_ = f.Close()
	if err := o.Run(harness, "npx", []string{"playwright", "test", "--project=setup"},
		[]string{"BASE_URL=" + o.BaseURL, "STORAGE_STATE=" + tmp}); err != nil {
		_ = os.Remove(tmp)
		return "", exitcode.New(exitcode.Fail, fmt.Errorf("login failed: %w", err))
	}
	dst := StatePathRole(o.Home, o.Role)
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return "", exitcode.New(exitcode.Fail, err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return "", exitcode.New(exitcode.Fail, err)
	}
	_, _ = fmt.Fprintf(o.Stdout, "state: %s (origin %s)\n", dst, o.BaseURL)
	return dst, nil
}
