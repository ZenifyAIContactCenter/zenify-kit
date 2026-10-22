package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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

// detectPM picks the JS package manager for dir: the packageManager field of the nearest
// package.json first, then the lockfile present (pnpm, yarn, npm, bun); otherwise an error.
func detectPM(dir string, lookPath func(string) (string, error)) (mgr, root, reason string, err error) {
	root, ok := findPackageRoot(dir)
	if !ok {
		return "", "", "", fmt.Errorf("pm: no package.json in %s or any parent", dir)
	}
	if b, rerr := os.ReadFile(filepath.Join(root, "package.json")); rerr == nil { //nolint:gosec // G304 -- root is the nearest ancestor holding package.json, found by findPackageRoot
		var pj struct {
			PackageManager string `json:"packageManager"`
		}
		if json.Unmarshal(b, &pj) == nil && pj.PackageManager != "" {
			mgr = strings.SplitN(pj.PackageManager, "@", 2)[0]
			reason = "packageManager field in package.json"
		}
	}
	if mgr == "" {
		switch {
		case fileExists(filepath.Join(root, "pnpm-lock.yaml")):
			mgr, reason = "pnpm", "pnpm-lock.yaml"
		case fileExists(filepath.Join(root, "yarn.lock")):
			mgr, reason = "yarn", "yarn.lock"
		case fileExists(filepath.Join(root, "package-lock.json")):
			mgr, reason = "npm", "package-lock.json"
		case fileExists(filepath.Join(root, "bun.lock")), fileExists(filepath.Join(root, "bun.lockb")):
			mgr, reason = "bun", "bun lockfile"
		}
	}
	if mgr == "" {
		return "", root, "", fmt.Errorf("pm: %s has no packageManager field and no lockfile, so the manager is unknown; run the intended manager once by hand so a lockfile exists", root) //znf:allow-lang
	}
	if _, lerr := lookPath(mgr); lerr != nil {
		return "", root, reason, fmt.Errorf("pm: detected %s (from %s) but it is not installed", mgr, reason)
	}
	return mgr, root, reason, nil
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
