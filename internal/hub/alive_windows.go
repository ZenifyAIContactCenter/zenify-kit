//go:build windows

package hub

// processAlive is always false on windows, where the hub's tmux does not
// run natively: every agent counts as one of the hub.
func processAlive(int) bool { return false }
