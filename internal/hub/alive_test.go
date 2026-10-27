//go:build !windows

package hub

import (
	"os"
	"testing"
)

func TestProcessAlive(t *testing.T) {
	if !processAlive(os.Getpid()) {
		t.Error("own process reported dead")
	}
	if processAlive(1 << 30) { // above every pid_max
		t.Error("impossible pid reported alive")
	}
}
