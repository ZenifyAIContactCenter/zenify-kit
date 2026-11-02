package mcpproxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// fixedFlags are the flags StartGrafana sets itself; extraArgs may not name them.
var fixedFlags = map[string]bool{"disable-write": true, "address": true, "t": true, "transport": true, "endpoint-path": true}

// checkGrafanaArgs refuses a non-loopback addr (mcp-grafana has no auth of its
// own) and any extraArgs entry that overrides a fixed flag. Errors name the
// flag, never its value.
func checkGrafanaArgs(addr string, extraArgs []string) error {
	host, _, err := net.SplitHostPort(addr)
	if ip := net.ParseIP(host); err != nil || (host != "localhost" && (ip == nil || !ip.IsLoopback())) {
		return fmt.Errorf("grafana.addr must be a loopback host:port (127.0.0.1, ::1 or localhost)")
	}
	for _, a := range extraArgs {
		name, _, _ := strings.Cut(a, "=")
		if strings.HasPrefix(name, "-") && fixedFlags[strings.TrimPrefix(strings.TrimPrefix(name, "-"), "-")] {
			return fmt.Errorf("grafana.args: %s is set by the broker and cannot be overridden", name)
		}
	}
	return nil
}

// StartGrafana starts mcp-grafana read-only on addr (FR-4.1), which must be
// loopback. The child's env
// is PATH and HOME plus its two Grafana variables and MCP_GRAFANA_SERVER_TOKEN
// (callerToken) only: the broker's env holds every prod secret. With the caller
// token set, mcp-grafana answers 401 to any request without
// `Authorization: Bearer <callerToken>`, so only the broker's audited proxy
// (BearerHeader) can call it. extraArgs (from config) follow the fixed argv. Child
// output goes to the broker's stderr. The child is killed when ctx ends.
// onExit is called with a cause on every child exit, including that kill, so
// the caller can cancel the server context and exit non-zero. The caller must
// not Wait on the returned Cmd: a goroutine here already does.
func StartGrafana(ctx context.Context, onExit context.CancelCauseFunc, bin, grafanaURL, token, callerToken, addr string, extraArgs []string) (*exec.Cmd, error) {
	if err := checkGrafanaArgs(addr, extraArgs); err != nil {
		return nil, err
	}
	if callerToken == "" {
		return nil, fmt.Errorf("mcp-grafana caller token is empty")
	}
	args := append([]string{"-t", "streamable-http", "--address", addr, "--disable-write"}, extraArgs...)
	c := exec.CommandContext(ctx, bin, args...) //nolint:gosec // G204 -- bin, addr and extraArgs come from the broker's config
	for _, k := range []string{"PATH", "HOME"} {
		if v := os.Getenv(k); v != "" {
			c.Env = append(c.Env, k+"="+v)
		}
	}
	c.Env = append(c.Env, "GRAFANA_URL="+grafanaURL, "GRAFANA_SERVICE_ACCOUNT_TOKEN="+token, "MCP_GRAFANA_SERVER_TOKEN="+callerToken)
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

// BearerHeader sets the caller token mcp-grafana requires (see StartGrafana).
func BearerHeader(callerToken string) func(http.Header) {
	return func(h http.Header) { h.Set("Authorization", "Bearer "+callerToken) }
}

// NewCallerToken returns a random token for one broker process's mcp-grafana.
func NewCallerToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("mcp-grafana caller token: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
