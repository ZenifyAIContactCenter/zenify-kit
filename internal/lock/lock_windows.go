//go:build windows

package lock

import (
	"os"
	"syscall"
	"unsafe"
)

// Windows advisory locking via kernel32 LockFileEx/UnlockFileEx. Kept on raw
// syscall (LazyDLL) so no new module dependency is introduced. This path is not
// exercised on the developer's macOS host — a teammate on Windows must verify
// it (see spec SC-001 and the b2a-lock-unwired memory).
var (
	kernel32       = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx = kernel32.NewProc("LockFileEx")
	procUnlockFile = kernel32.NewProc("UnlockFileEx")
)

const (
	lockfileExclusiveLock   = 0x00000002
	lockfileFailImmediately = 0x00000001
)

// a non-nil OVERLAPPED is required by LockFileEx.
type overlapped struct {
	Internal     uintptr
	InternalHigh uintptr
	Offset       uint32
	OffsetHigh   uint32
	HEvent       syscall.Handle
}

func flockExclusive(f *os.File) (bool, error) {
	var ol overlapped
	r, _, err := procLockFileEx.Call(
		uintptr(f.Fd()),
		uintptr(lockfileExclusiveLock|lockfileFailImmediately),
		0,
		uintptr(^uint32(0)), // lock the whole file (low dword)
		uintptr(^uint32(0)), // high dword
		uintptr(unsafe.Pointer(&ol)),
	)
	if r != 0 {
		return true, nil
	}
	// ERROR_LOCK_VIOLATION (33) means another process holds it.
	if errno, ok := err.(syscall.Errno); ok && errno == 33 {
		return false, nil
	}
	return false, err
}

func flockUnlock(f *os.File) error {
	var ol overlapped
	r, _, err := procUnlockFile.Call(
		uintptr(f.Fd()),
		0,
		uintptr(^uint32(0)),
		uintptr(^uint32(0)),
		uintptr(unsafe.Pointer(&ol)),
	)
	if r != 0 {
		return nil
	}
	return err
}
