package e2e

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

const (
	pinnedPlaywright = `"version": "1.55.0"`
	staleLockAge     = 10 * time.Minute
	defaultLockWait  = 120 * time.Second
	lockPoll         = 500 * time.Millisecond
)

// LoginOptions configures Login. Credentials are read only through Getenv by the
// spawned process (inherited env); they never pass through Run's args or extraEnv.
type LoginOptions struct {
	Home     string
	BaseURL  string
	Getenv   func(string) string
	Run      func(dir, name string, args []string, extraEnv []string) error
	Stdout   io.Writer
	LockWait time.Duration // 0 = 120s
}

func playwrightDir(home string) string { return filepath.Join(home, ".zenify", "playwright") }

// StatePath is where the Playwright storage state is written.
func StatePath(home string) string { return filepath.Join(playwrightDir(home), "state.json") }

// acquireLock takes an O_EXCL lock file, breaking locks older than staleLockAge.
func acquireLock(path string, wait time.Duration) error {
	deadline := time.Now().Add(wait)
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // G304 -- path built from home
		if err == nil {
			return f.Close()
		}
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		if st, serr := os.Stat(path); serr == nil && time.Since(st.ModTime()) > staleLockAge {
			_ = os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return errors.New("harness locked by another login")
		}
		time.Sleep(lockPoll)
	}
}

// prepareHarness writes the harness and installs deps + chromium, under the lock.
func prepareHarness(o LoginOptions, root, harness string) error {
	wait := o.LockWait
	if wait == 0 {
		wait = defaultLockWait
	}
	lock := filepath.Join(root, "harness.lock")
	if err := acquireLock(lock, wait); err != nil {
		return err
	}
	defer func() { _ = os.Remove(lock) }()
	if err := WriteHarness(harness); err != nil {
		return err
	}
	pkg := filepath.Join(harness, "node_modules", "@playwright", "test", "package.json")
	if b, err := os.ReadFile(pkg); err != nil || !strings.Contains(string(b), pinnedPlaywright) { //nolint:gosec // G304 -- path built from home
		if err := o.Run(harness, "npm", []string{"install", "--no-audit", "--no-fund", "--silent"}, nil); err != nil {
			return fmt.Errorf("npm install: %w", err)
		}
	}
	if err := o.Run(harness, "npx", []string{"playwright", "install", "chromium"}, nil); err != nil {
		return fmt.Errorf("playwright install: %w", err)
	}
	return nil
}

// Login runs the harness setup project on the host and atomically writes the
// storage state. It returns the state path.
func Login(o LoginOptions) (string, error) {
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
	if err := prepareHarness(o, root, harness); err != nil {
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
	dst := StatePath(o.Home)
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
