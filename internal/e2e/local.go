package e2e

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

// LocalOptions configures RunLocal: journeys run on the host Playwright harness
// (the one `e2e login` uses) instead of Docker.
type LocalOptions struct {
	RepoDir  string // repo whose .znf/e2e holds the journeys
	BaseURL  string // loopback origin of the app under test
	Env      []string
	Run      func(dir, name string, args []string, env []string) error
	Home     string
	LockWait time.Duration
}

func (o LocalOptions) paths() (root, harness string) {
	root = playwrightDir(o.Home)
	return root, filepath.Join(root, "harness")
}

// PrepareLocal writes the harness and installs its deps; it runs no test.
func PrepareLocal(o LocalOptions) error {
	root, harness := o.paths()
	if err := os.MkdirAll(root, 0o700); err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	lo := LoginOptions{Home: o.Home, Run: o.Run, LockWait: o.LockWait}
	if err := PrepareHarness(lo, root, harness); err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	return nil
}

// RunLocal prepares the harness, copies <RepoDir>/.znf/e2e into it and runs
// `npx playwright test` there. The journeys are copied, not symlinked: they import
// '../../fixtures', and a symlink may resolve outside the harness.
func RunLocal(o LocalOptions) error {
	if err := CheckLoopbackURL(o.BaseURL); err != nil {
		return err
	}
	if err := PrepareLocal(o); err != nil {
		return err
	}
	root, harness := o.paths()
	dst := filepath.Join(harness, ".znf", "e2e")
	if err := os.RemoveAll(dst); err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	if err := copyTree(e2eDirOf(o.RepoDir), dst); err != nil {
		return exitcode.New(exitcode.Fail, fmt.Errorf("copy journeys: %w", err))
	}
	f, err := os.CreateTemp(root, "state-*.json.tmp")
	if err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	tmp := f.Name()
	_ = f.Close()
	defer func() { _ = os.Remove(tmp) }()
	env := append([]string{"BASE_URL=" + o.BaseURL, "STORAGE_STATE=" + tmp}, o.Env...)
	if err := o.Run(harness, "npx", []string{"playwright", "test"}, env); err != nil {
		return exitcode.New(exitcode.Fail, fmt.Errorf("e2e run: %w", err))
	}
	return nil
}

func e2eDirOf(repo string) string { return filepath.Join(repo, ".znf", "e2e") }

// copyTree copies regular files and directories from src to dst.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		b, err := os.ReadFile(p) //nolint:gosec // G304 -- p walks the repo's own .znf/e2e
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o600) //nolint:gosec // G306 -- harness-private copy
	})
}
