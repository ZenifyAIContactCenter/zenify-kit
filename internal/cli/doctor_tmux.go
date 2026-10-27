package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/tmuxx"
	"golang.org/x/term"
)

// tmuxEnv holds the host facts the tmux check depends on, injectable for tests.
type tmuxEnv struct {
	goos     string
	lookPath func(string) (string, error)
	runner   tmuxx.Runner
	install  func(name string, args ...string) error
	// isTerminal reports whether stdin is a terminal (sudo needs one to prompt).
	isTerminal func() bool
}

var defaultTmuxEnv = tmuxEnv{
	goos:       runtime.GOOS,
	lookPath:   exec.LookPath,
	runner:     tmuxx.ExecRunner(),
	install:    runInstall,
	isTerminal: func() bool { return term.IsTerminal(int(os.Stdin.Fd())) },
}

// runInstall runs the installer with stdio attached to the terminal so sudo can prompt.
func runInstall(name string, args ...string) error {
	cmd := exec.Command(name, args...) //nolint:gosec // G204 -- name/args come from tmuxInstallCommand's fixed package-manager commands, not user input
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// tmuxInstallCommand picks the package-manager command that installs tmux on this host.
func tmuxInstallCommand(env tmuxEnv) (name string, args []string, ok bool) {
	has := func(bin string) bool {
		_, err := env.lookPath(bin)
		return err == nil
	}
	switch env.goos {
	case "darwin":
		if has("brew") {
			return "brew", []string{"install", "tmux"}, true
		}
	case "linux":
		if has("apt-get") {
			return "sudo", []string{"apt-get", "install", "-y", "tmux"}, true
		}
		if has("dnf") {
			return "sudo", []string{"dnf", "install", "-y", "tmux"}, true
		}
		if has("brew") {
			return "brew", []string{"install", "tmux"}, true
		}
	}
	return "", nil, false
}

func tmuxCheck(env tmuxEnv) Check {
	return Check{
		Name: "tmux",
		Run: func() (bool, string) {
			_, ok, detail := tmuxx.Available(env.lookPath, env.runner)
			if !ok {
				detail += " — or run: zenify doctor --fix"
			}
			return ok, detail
		},
		Fix: func() (bool, string) {
			if _, ok, _ := tmuxx.Available(env.lookPath, env.runner); ok {
				return true, "already installed"
			}
			name, args, ok := tmuxInstallCommand(env)
			if !ok {
				return false, "no supported package manager — install tmux ≥ 3.1 manually (Windows: use WSL)"
			}
			if name == "sudo" && !env.isTerminal() {
				return false, fmt.Sprintf("sudo needs a terminal to ask for your password — run `zenify doctor --fix` in your own terminal, or run: %s %s", name, strings.Join(args, " "))
			}
			if err := env.install(name, args...); err != nil {
				return false, fmt.Sprintf("install failed: %v", err)
			}
			ver, ok, detail := tmuxx.Available(env.lookPath, env.runner)
			if !ok {
				return false, detail
			}
			return true, "installed tmux " + ver
		},
	}
}

// printTmuxHint tells the user how to get tmux when it is missing; it never installs.
func printTmuxHint(w io.Writer, env tmuxEnv) {
	if _, ok, detail := tmuxx.Available(env.lookPath, env.runner); !ok {
		fmt.Fprintf(w, "hint: %s (needed by zenify hub; run: zenify doctor --fix)\n", detail)
	}
}

func init() {
	RegisterCheck(tmuxCheck(defaultTmuxEnv))
}
