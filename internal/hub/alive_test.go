//go:build !windows

package hub

import (
	"errors"
	"os"
	"testing"
)

func TestProcessAlive(t *testing.T) {
	old := readComm
	t.Cleanup(func() { readComm = old })
	readComm = func(int) (string, error) { return "tmux: server", nil }
	if !processAlive(os.Getpid()) {
		t.Error("own process reported dead")
	}
	if processAlive(1 << 30) { // above every pid_max
		t.Error("impossible pid reported alive")
	}
}

func TestProcessAlive_ChecksTmuxName(t *testing.T) {
	old := readComm
	t.Cleanup(func() { readComm = old })
	for _, c := range []struct {
		comm string
		err  error
		want bool
	}{
		{"bash\n", nil, false},
		{"tmux: server\n", nil, true},
		{"tmux\n", nil, true},
		{"", errors.New("no /proc"), true},
	} {
		readComm = func(int) (string, error) { return c.comm, c.err }
		if got := processAlive(os.Getpid()); got != c.want {
			t.Errorf("comm %q err %v: alive = %v, want %v", c.comm, c.err, got, c.want)
		}
	}
}
