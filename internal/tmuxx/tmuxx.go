// Package tmuxx is a thin adapter over the `tmux` CLI: version check and the
// few pane/message operations the hub sidebar needs.
package tmuxx

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Session is the tmux session name the hub runs in.
const Session = "zenify"

// Target is the exact-match target for Session; a bare name would let
// tmux prefix-match another session such as "zenify-old".
const Target = "=" + Session

const installHint = "install: brew install tmux (macOS) or sudo apt install tmux (Linux)"

// Runner abstracts invoking `tmux` so callers can inject a fake in tests.
type Runner interface {
	Run(args ...string) ([]byte, error)
}

// Socket is the name of the dedicated tmux server the hub runs on (`tmux -L
// <name>`): $ZENIFY_HUB_SOCKET, or "zenify". The user's default server is
// never touched.
func Socket() string {
	if s := os.Getenv("ZENIFY_HUB_SOCKET"); s != "" {
		return s
	}
	return "zenify"
}

// execArgs is args addressed to the hub server.
func execArgs(args []string) []string {
	return append([]string{"-L", Socket()}, args...)
}

type execRunner struct{}

// Run invokes tmux on the hub server; every hub tmux call goes through here.
func (execRunner) Run(args ...string) ([]byte, error) {
	var stderr bytes.Buffer
	cmd := exec.Command("tmux", execArgs(args)...) //nolint:gosec // G204 -- fixed trusted binary, args are internally-computed subcommands, not attacker-controlled shell input
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return out, fmt.Errorf("tmux %s: %w: %s", strings.Join(args, " "), err, msg)
		}
		return out, fmt.Errorf("tmux %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// ExecRunner returns the default Runner that shells to `tmux`.
func ExecRunner() Runner { return execRunner{} }

// Available checks that tmux is installed and at least version 3.1.
// detail is empty when ok.
func Available(lookPath func(string) (string, error), r Runner) (version string, ok bool, detail string) {
	if _, err := lookPath("tmux"); err != nil {
		return "", false, "tmux not found — " + installHint
	}
	out, err := r.Run("-V")
	if err != nil {
		return "", false, fmt.Sprintf("tmux -V failed: %v", err)
	}
	version = parseVersion(string(out))
	major, minor, parsed := versionNumbers(version)
	if !parsed {
		return version, false, fmt.Sprintf("cannot parse tmux version from %q", strings.TrimSpace(string(out)))
	}
	if major < 3 || (major == 3 && minor < 1) {
		return version, false, fmt.Sprintf("tmux %s is older than 3.1 — upgrade: %s", version, strings.TrimPrefix(installHint, "install: "))
	}
	return version, true, ""
}

// parseVersion extracts "3.4", "3.3a" or "3.5" from "tmux 3.4" / "tmux next-3.5".
func parseVersion(out string) string {
	f := strings.Fields(out)
	if len(f) < 2 {
		return ""
	}
	return strings.TrimPrefix(f[1], "next-")
}

// versionNumbers returns the major and minor numbers, ignoring a letter suffix.
func versionNumbers(v string) (major, minor int, ok bool) {
	maj, rest, found := strings.Cut(v, ".")
	if !found {
		return 0, 0, false
	}
	rest = strings.TrimRight(rest, "abcdefghijklmnopqrstuvwxyz")
	a, err1 := strconv.Atoi(maj)
	b, err2 := strconv.Atoi(rest)
	if err := errors.Join(err1, err2); err != nil {
		return 0, 0, false
	}
	return a, b, true
}

// HasSession reports whether the hub session exists.
func HasSession(r Runner) bool {
	_, err := r.Run("has-session", "-t", Target)
	return err == nil
}

// SelectPane focuses the window holding paneID, then the pane itself.
func SelectPane(r Runner, paneID string) error {
	if _, err := r.Run("select-window", "-t", paneID); err != nil {
		return err
	}
	_, err := r.Run("select-pane", "-t", paneID)
	return err
}

// DisplayMessage shows msg in the hub session status line for 5 seconds.
func DisplayMessage(r Runner, msg string) error {
	_, err := r.Run("display-message", "-d", "5000", "-t", Target, msg)
	return err
}

// SendKeys types text literally into paneID and presses Enter.
func SendKeys(r Runner, paneID, text string) error {
	if _, err := r.Run("send-keys", "-t", paneID, "-l", text); err != nil {
		return err
	}
	_, err := r.Run("send-keys", "-t", paneID, "Enter")
	return err
}

// ServerPID is the pid of the tmux server r talks to.
func ServerPID(r Runner) (string, error) {
	out, err := r.Run("display-message", "-p", "#{pid}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Pane is the window holding a live pane: its index and its id (@N).
type Pane struct {
	Window   int
	WindowID string
}

// LivePanes maps every pane id on the server to its window.
func LivePanes(r Runner) (map[string]Pane, error) {
	out, err := r.Run("list-panes", "-a", "-F", "#{pane_id} #{window_index} #{window_id}")
	if err != nil {
		return nil, err
	}
	panes := map[string]Pane{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 3 {
			return nil, fmt.Errorf("parse list-panes line %q: want 3 fields", line)
		}
		n, err := strconv.Atoi(f[1])
		if err != nil {
			return nil, fmt.Errorf("parse list-panes line %q: %w", line, err)
		}
		panes[f[0]] = Pane{Window: n, WindowID: f[2]}
	}
	return panes, nil
}

// NoServer reports whether err is tmux saying no server is running: none
// listens on the socket, or the socket file does not exist. Any other connect
// error (permission denied, ...) is a real error.
func NoServer(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no server running") || strings.Contains(msg, "No such file or directory")
}
