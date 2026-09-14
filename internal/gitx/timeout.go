package gitx

import (
	"context"
	"errors"
	"os/exec"
	"time"
)

// execGit builds the git command; a package-level seam so tests can swap in a
// process that blocks.
var execGit = func(ctx context.Context, dir string, args ...string) *exec.Cmd {
	full := append([]string{"-C", dir}, args...)
	return exec.CommandContext(ctx, "git", full...) //nolint:gosec // G204 -- fixed trusted binary, args are internally-computed subcommands, not attacker-controlled shell input
}

type timeoutRunner struct{ d time.Duration }

// TimeoutRunner returns a Runner that bounds EVERY invocation to d. Callers
// that must never hang a session (SessionStart hooks, `wt sweep --all`'s
// parallel fetch) use it instead of ExecRunner. On deadline the error wraps
// context.DeadlineExceeded so callers can tell a timeout from a git failure.
func TimeoutRunner(d time.Duration) Runner { return timeoutRunner{d: d} }

func (t timeoutRunner) Run(dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), t.d)
	defer cancel()
	out, err := execGit(ctx, dir, args...).Output()
	if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return out, context.DeadlineExceeded
	}
	return out, err
}
