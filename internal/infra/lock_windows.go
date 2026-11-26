//go:build windows

package infra

import "errors"

// LockHost is unsupported off Linux: `zenify infra` runs on a Linux devbox only.
func LockHost(string) (func() error, error) {
	return nil, errors.New("infra lock: zenify infra runs on linux only")
}
