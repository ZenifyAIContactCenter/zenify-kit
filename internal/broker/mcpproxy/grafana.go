package mcpproxy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// StartGrafana starts mcp-grafana read-only on addr (FR-4.1). The child's env
// is PATH and HOME plus its two Grafana variables only: the broker's env holds
// every prod secret. extraArgs (from config) follow the fixed argv. Child
// output goes to the broker's stderr. The child is killed when ctx ends.
// onExit is called with a cause on every child exit, including that kill, so
// the caller can cancel the server context and exit non-zero. The caller must
// not Wait on the returned Cmd: a goroutine here already does.
func StartGrafana(ctx context.Context, onExit context.CancelCauseFunc, bin, grafanaURL, token, addr string, extraArgs []string) (*exec.Cmd, error) {
	args := append([]string{"-t", "streamable-http", "--address", addr, "--disable-write"}, extraArgs...)
	c := exec.CommandContext(ctx, bin, args...) //nolint:gosec // G204 -- bin, addr and extraArgs come from the broker's config
	for _, k := range []string{"PATH", "HOME"} {
		if v := os.Getenv(k); v != "" {
			c.Env = append(c.Env, k+"="+v)
		}
	}
	c.Env = append(c.Env, "GRAFANA_URL="+grafanaURL, "GRAFANA_SERVICE_ACCOUNT_TOKEN="+token)
	c.Stdout, c.Stderr = os.Stderr, os.Stderr
	if err := c.Start(); err != nil {
		return nil, fmt.Errorf("start mcp-grafana: %w", err)
	}
	go func() {
		err := c.Wait()
		if err == nil {
			err = fmt.Errorf("exited with status 0")
		}
		onExit(fmt.Errorf("mcp-grafana: %w", err))
	}()
	return c, nil
}
