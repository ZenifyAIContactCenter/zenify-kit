package gitx

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"
)

func TestTimeoutRunner_KillsHungGit(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not available")
	}
	old := execGit
	execGit = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sleep", "5")
	}
	t.Cleanup(func() { execGit = old })
	r := TimeoutRunner(200 * time.Millisecond)
	start := time.Now()
	_, err := r.Run("/", "fetch")
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want DeadlineExceeded, got %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("took %v; the deadline did not fire", time.Since(start))
	}
}

func TestTimeoutRunner_PassesThroughOnSuccess(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	out, err := TimeoutRunner(5*time.Second).Run(t.TempDir(), "--version")
	if err != nil || len(out) == 0 {
		t.Fatalf("git --version via TimeoutRunner: %v %q", err, out)
	}
}

// A grandchild that keeps the pipes open after git is killed must not hold Run
// past the deadline (WaitDelay closes the pipes).
func TestTimeoutRunner_ReturnsWhenChildHoldsPipes(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	old := execGit
	execGit = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "sleep 8 & sleep 8")
	}
	t.Cleanup(func() { execGit = old })
	start := time.Now()
	_, err := TimeoutRunner(200*time.Millisecond).Run("/", "fetch")
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want DeadlineExceeded, got %v", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("took %v; a child holding the pipes blocked Run", d)
	}
}
