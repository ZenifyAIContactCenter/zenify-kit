package infra

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path"
	"strings"
)

// Runner runs one host command. stdin may be nil. Secrets go through stdin only,
// never args: args can surface in process listings and error messages.
type Runner interface {
	Run(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error)
}

type execRunner struct{}

// ExecRunner runs commands on the real host.
func ExecRunner() Runner { return execRunner{} }

// Run returns stdout; a failure wraps the exec error (so errors.As finds
// *exec.ExitError) and appends the command's trimmed stderr.
func (execRunner) Run(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204 -- callers pass fixed command names; secrets go via stdin
	cmd.Stdin = stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return stdout.Bytes(), fmt.Errorf("%w: %s", err, msg)
		}
		return stdout.Bytes(), err
	}
	return stdout.Bytes(), nil
}

// kubectl targets the local k3s cluster explicitly: the user's default context
// may point at a shared remote cluster.
func kubectl(ctx context.Context, r Runner, stdin io.Reader, args ...string) ([]byte, error) {
	return r.Run(ctx, stdin, "kubectl", append([]string{"--kubeconfig", K3sKubeconfig}, args...)...)
}

// exitCode returns the process exit code carried by err, or -1.
func exitCode(err error) int {
	var ec interface{ ExitCode() int }
	if errors.As(err, &ec) {
		return ec.ExitCode()
	}
	return -1
}

// readRootFile reads a root-owned file; exists is false when it is absent.
func readRootFile(ctx context.Context, r Runner, p string) (data []byte, exists bool, err error) {
	if _, err := r.Run(ctx, nil, "sudo", "test", "-f", p); err != nil {
		if exitCode(err) == 1 {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("check %s: %w", p, err)
	}
	out, err := r.Run(ctx, nil, "sudo", "cat", p)
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", p, err)
	}
	return out, true, nil
}

// writeRootFile writes content to p via a temp file + rename.
func writeRootFile(ctx context.Context, r Runner, p string, content []byte) error {
	if _, err := r.Run(ctx, nil, "sudo", "install", "-d", "-m", "0755", path.Dir(p)); err != nil {
		return fmt.Errorf("mkdir %s: %w", path.Dir(p), err)
	}
	tmp := p + ".zenify.tmp"
	if _, err := r.Run(ctx, bytes.NewReader(content), "sudo", "tee", tmp); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if _, err := r.Run(ctx, nil, "sudo", "mv", "-f", tmp, p); err != nil {
		return fmt.Errorf("rename %s to %s: %w", tmp, p, err)
	}
	return nil
}
