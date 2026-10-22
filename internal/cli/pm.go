package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

// findPackageRoot returns the nearest ancestor of dir (inclusive) holding a package.json.
func findPackageRoot(dir string) (string, bool) {
	for d := dir; ; d = filepath.Dir(d) {
		if st, err := os.Stat(filepath.Join(d, "package.json")); err == nil && !st.IsDir() {
			return d, true
		}
		if filepath.Dir(d) == d {
			return "", false
		}
	}
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// pmNames are the only managers a packageManager field may name.
var pmNames = []string{"pnpm", "yarn", "npm", "bun"}

// detectPM picks the JS package manager for dir: the packageManager field of the nearest
// package.json first, then the lockfile present (pnpm, yarn, npm, bun). A monorepo package
// with neither takes the signal of the nearest ancestor that has one, up to the git
// toplevel; the manager still runs at the package's root. No signal is an error.
func detectPM(dir string, lookPath func(string) (string, error)) (mgr, root, reason string, err error) {
	root, ok := findPackageRoot(dir)
	if !ok {
		return "", "", "", fmt.Errorf("pm: no package.json in %s or any parent", dir)
	}
	for d := root; ; d = filepath.Dir(d) {
		if mgr, reason, err = pmSignal(d); err != nil {
			return "", root, "", err
		}
		if mgr != "" {
			if d != root {
				reason += " (" + d + ")"
			}
			break
		}
		if _, serr := os.Stat(filepath.Join(d, ".git")); serr == nil || filepath.Dir(d) == d {
			break
		}
	}
	if mgr == "" {
		return "", root, "", fmt.Errorf("pm: %s and its parents up to the git toplevel have no packageManager field and no lockfile, so the manager is unknown; run the intended manager once by hand so a lockfile exists", root) //znf:allow-lang
	}
	if _, lerr := lookPath(mgr); lerr != nil {
		return "", root, reason, fmt.Errorf("pm: detected %s (from %s) but it is not installed", mgr, reason)
	}
	return mgr, root, reason, nil
}

// pmSignal is dir's own signal: the packageManager field of its package.json,
// else its lockfile; "" when it has neither.
func pmSignal(dir string) (mgr, reason string, err error) {
	if b, rerr := os.ReadFile(filepath.Join(dir, "package.json")); rerr == nil { //nolint:gosec // G304 -- dir is the package root or one of its ancestors
		var pj struct {
			PackageManager string `json:"packageManager"`
		}
		if json.Unmarshal(b, &pj) == nil && pj.PackageManager != "" {
			name := strings.SplitN(pj.PackageManager, "@", 2)[0]
			if !slices.Contains(pmNames, name) {
				return "", "", fmt.Errorf("pm: packageManager %q in %s is not one of %s", pj.PackageManager, filepath.Join(dir, "package.json"), strings.Join(pmNames, ", "))
			}
			return name, "packageManager field in package.json", nil
		}
	}
	switch {
	case fileExists(filepath.Join(dir, "pnpm-lock.yaml")):
		return "pnpm", "pnpm-lock.yaml", nil
	case fileExists(filepath.Join(dir, "yarn.lock")):
		return "yarn", "yarn.lock", nil
	case fileExists(filepath.Join(dir, "package-lock.json")):
		return "npm", "package-lock.json", nil
	case fileExists(filepath.Join(dir, "bun.lock")), fileExists(filepath.Join(dir, "bun.lockb")):
		return "bun", "bun lockfile", nil
	}
	return "", "", nil
}

// pmRunner starts the manager at path (argv[0] is the manager name) inside root and returns
// the exit code. On Unix it replaces the process (syscall.Exec) and only returns on failure.
type pmRunner func(path string, argv []string, root string, stdout, stderr io.Writer) int

// runPM implements `pm [args…]` and returns the process exit code.
func runPM(args []string, dir string, stdout, stderr io.Writer, lookPath func(string) (string, error), run pmRunner) int {
	mgr, root, reason, err := detectPM(dir, lookPath)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	if len(args) == 0 {
		fmt.Fprintln(stdout, mgr)
		fmt.Fprintf(stderr, "root: %s\nreason: %s\n", root, reason)
		return 0
	}
	path, err := lookPath(mgr)
	if err != nil {
		fmt.Fprintf(stderr, "pm: detected %s (from %s) but it is not installed\n", mgr, reason)
		return 1
	}
	return run(path, append([]string{mgr}, args...), root, stdout, stderr)
}

func newPmCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "pm [args…]",
		Short:              "Chạy package manager JS đúng của project (trường packageManager, rồi lockfile)", //znf:allow-lang
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			if code := runPM(args, dir, cmd.OutOrStdout(), cmd.ErrOrStderr(), exec.LookPath, execPM); code != 0 {
				os.Exit(code)
			}
			return nil
		},
	}
}
