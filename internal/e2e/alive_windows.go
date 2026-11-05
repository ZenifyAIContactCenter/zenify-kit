//go:build windows

package e2e

// processAlive has no cheap probe here: unknown, so staleLockAge alone breaks a lock.
func processAlive(int) (alive, known bool) { return false, false }
