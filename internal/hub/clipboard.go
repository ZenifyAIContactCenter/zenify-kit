package hub

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// clipTimeout bounds a clipboard tool so a hung one cannot freeze the sidebar.
const clipTimeout = 2 * time.Second

// clipboard copies text to the system clipboard.
type clipboard func(text string) error

// pipeRunner runs the program at path with args, writing stdin to it.
type pipeRunner func(stdin, path string, args ...string) error

// clipEnv is what pickClipboard looks at; tests pass fakes.
type clipEnv struct {
	goos     string
	getenv   func(string) string
	lookPath func(string) (string, error)
	run      pipeRunner
	out      io.Writer     // OSC 52 fallback
	timeout  time.Duration // how long a tool may run
}

// clipTools in order of preference: a tool applies when its env var is set
// (or goos matches) and it is on PATH.
var clipTools = []struct {
	env, goos, name string
	args            []string
}{
	{env: "WAYLAND_DISPLAY", name: "wl-copy"},
	{env: "DISPLAY", name: "xclip", args: []string{"-selection", "clipboard"}},
	{env: "DISPLAY", name: "xsel", args: []string{"--clipboard", "--input"}},
	{goos: "darwin", name: "pbcopy"},
}

// pickClipboard returns the first available clipboard tool, or OSC 52 written
// to e.out when none is.
func pickClipboard(e clipEnv) clipboard {
	for _, t := range clipTools {
		if (t.env != "" && e.getenv(t.env) == "") || (t.goos != "" && t.goos != e.goos) {
			continue
		}
		path, err := e.lookPath(t.name)
		if err != nil {
			continue // not installed: try the next tool
		}
		name, args := t.name, t.args
		return func(text string) error {
			done := make(chan error, 1) // buffered: a late tool must not leak a blocked sender
			go func() { done <- e.run(text, path, args...) }()
			select {
			case err := <-done:
				if err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}
				return nil
			case <-time.After(e.timeout):
				return fmt.Errorf("%s: timed out after %s", name, e.timeout)
			}
		}
	}
	return osc52Clipboard(e.out)
}

// osc52Clipboard writes the OSC 52 sequence to out.
func osc52Clipboard(out io.Writer) clipboard {
	return func(text string) error {
		_, err := io.WriteString(out, OSC52(text))
		return err
	}
}

// newClipboard picks the clipboard for this machine; OSC 52 goes to out.
func newClipboard(out io.Writer) clipboard {
	return pickClipboard(clipEnv{goos: runtime.GOOS, getenv: os.Getenv, lookPath: exec.LookPath, run: runPipe, out: out, timeout: clipTimeout})
}

// runPipe runs path with stdin, killed after clipTimeout. Stdout and stderr
// stay unset (the null device): wl-copy and xclip leave a child holding the
// clipboard, which would keep a captured pipe open and block Run.
func runPipe(stdin, path string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), clipTimeout)
	defer cancel()
	c := exec.CommandContext(ctx, path, args...) //nolint:gosec // G204 -- path is a clipboard tool from clipTools, found by exec.LookPath
	c.Stdin = strings.NewReader(stdin)
	return c.Run()
}
