//go:build windows

package apply

import (
	"errors"
	"syscall"
)

// errorNotSameDevice is ERROR_NOT_SAME_DEVICE (17): MoveFileEx refuses to move
// a directory across volumes. syscall.EXDEV on windows is an invented errno
// that never comes back from the OS, so match the real Win32 code instead.
const errorNotSameDevice = syscall.Errno(17)

func isCrossDevice(err error) bool { return errors.Is(err, errorNotSameDevice) }
