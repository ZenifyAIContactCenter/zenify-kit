//go:build !windows

package apply

import (
	"errors"
	"syscall"
)

// isCrossDevice reports whether a rename failed because source and destination
// sit on different filesystems (EXDEV). *os.LinkError unwraps to the errno.
func isCrossDevice(err error) bool { return errors.Is(err, syscall.EXDEV) }
