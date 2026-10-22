//go:build windows

package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// execPM runs the manager as a child (Windows has no exec) and returns its exit code.
func execPM(path string, argv []string, root string, stdout, stderr io.Writer) int {
	c := exec.Command(path, argv[1:]...)
	c.Dir = root
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, stdout, stderr
	if err := c.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		fmt.Fprintf(stderr, "pm: %v\n", err)
		return 1
	}
	return 0
}
