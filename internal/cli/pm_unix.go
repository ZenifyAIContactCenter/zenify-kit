//go:build !windows

package cli

import (
	"fmt"
	"io"
	"os"
	"syscall"
)

// execPM replaces this process with the manager (same semantics as the shell `exec`):
// exit code, signals and Ctrl-C belong to the manager itself.
func execPM(path string, argv []string, root string, _, stderr io.Writer) int {
	if err := os.Chdir(root); err != nil {
		fmt.Fprintf(stderr, "pm: %v\n", err)
		return 1
	}
	err := syscall.Exec(path, argv, os.Environ()) //nolint:gosec // G204 -- path is the manager detectPM resolved via LookPath; argv is the user's own command line
	fmt.Fprintf(stderr, "pm: %v\n", err)
	return 1
}
