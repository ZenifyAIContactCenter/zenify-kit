package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ghx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/hub"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/tmuxx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/wt"
)

// hubEnv holds the host boundaries of the launcher, injectable for tests.
type hubEnv struct {
	lookPath func(string) (string, error)
	runner   tmuxx.Runner
	attach   func(args ...string) error
	self     func() (string, error)
}

var hubLauncher = hubEnv{
	lookPath: exec.LookPath,
	runner:   tmuxx.ExecRunner(),
	attach:   attachTmux,
	self:     os.Executable,
}

// attachTmux runs an interactive tmux client on the current terminal. It is
// not syscall.Exec so the package still builds for windows.
func attachTmux(args ...string) error {
	cmd := exec.Command("tmux", args...) //nolint:gosec // G204 -- fixed binary, internally computed args
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runHubLauncher opens (or re-attaches to) the zenify tmux session: the agent
// window on the right, the hub sidebar pane on the left.
func runHubLauncher(cmd *cobra.Command, env hubEnv) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve cwd: %w", err)
	}
	root, ok := wt.FindWorkspaceRoot(cwd)
	if !ok {
		return errors.New("not inside a zenify workspace")
	}
	if _, ok, detail := tmuxx.Available(env.lookPath, env.runner); !ok {
		return errors.New(detail)
	}
	if os.Getenv("TMUX") != "" {
		fmt.Fprintln(cmd.OutOrStdout(), `already inside tmux — run "zenify hub sidebar" in a pane`)
		return nil
	}
	if !tmuxx.HasSession(env.runner) {
		self, err := env.self()
		if err != nil {
			return fmt.Errorf("resolve executable: %w", err)
		}
		if err := buildHubSession(env.runner, root, self); err != nil {
			// A half-built session would make the next run attach without a sidebar.
			_, kerr := env.runner.Run("kill-session", "-t", tmuxx.Session)
			return errors.Join(fmt.Errorf("setting up tmux session failed: %w", err), kerr)
		}
	}
	if err := env.attach("attach", "-t", tmuxx.Session); err != nil {
		return fmt.Errorf("tmux attach: %w", err)
	}
	return nil
}

// shellQuote single-quotes s for sh, escaping embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// buildHubSession creates the detached session with the agent window and the
// sidebar pane. The caller cleans up the session when this fails.
func buildHubSession(r tmuxx.Runner, root, self string) error {
	target := tmuxx.Session + ":agent"
	if _, err := r.Run("new-session", "-d", "-s", tmuxx.Session, "-c", root, "-n", "agent", "claude"); err != nil {
		return err
	}
	if _, err := r.Run("split-window", "-d", "-h", "-b", "-l", "30%", "-t", target, "-c", root, shellQuote(self)+" hub sidebar"); err != nil {
		return err
	}
	// Best effort: OSC 52 copy needs it, tmux < 3.3 rejects the option.
	if _, err := r.Run("set-option", "-w", "-t", target, "allow-passthrough", "on"); err != nil {
		perr := fmt.Errorf("hub: allow-passthrough: %w", err)
		if home, herr := os.UserHomeDir(); herr == nil {
			hub.LogError(home, perr, time.Now().Unix())
		} else {
			fmt.Fprintln(os.Stderr, perr)
		}
	}
	return nil
}

func runHubSidebar() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home: %w", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve cwd: %w", err)
	}
	root, ok := wt.FindWorkspaceRoot(cwd)
	if !ok {
		return errors.New("not inside a zenify workspace")
	}
	tm := tmuxx.ExecRunner()
	poller := &hub.Poller{
		Home: home, Root: root,
		Git: gitx.ExecRunner(), GH: ghx.ExecRunner(), Tmux: tm,
		Now: func() int64 { return time.Now().Unix() },
	}
	_, err = tea.NewProgram(hub.NewModel(home, root, tm, poller), tea.WithAltScreen()).Run()
	return err
}

// runHubReport applies one hook event to the hub agent files. It is fail-open:
// any error is appended to hub.log and never surfaces to the harness.
func runHubReport(stdin io.Reader, pane string, now int64) {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "zenify hub: resolve home: %v\n", err)
		return
	}
	if err := hub.Report(home, stdin, pane, now); err != nil {
		hub.LogError(home, err, now)
	}
}

func newHubCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hub",
		Short: "Mở sidebar tmux theo dõi task, agent và PR của workspace", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runHubLauncher(cmd, hubLauncher)
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:    "sidebar",
		Short:  "Run the hub sidebar TUI (started by zenify hub)",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE:   func(*cobra.Command, []string) error { return runHubSidebar() },
	})
	cmd.AddCommand(&cobra.Command{
		Use:    "report",
		Short:  "Record one Claude Code hook event (reads the hook JSON on stdin)",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			runHubReport(cmd.InOrStdin(), os.Getenv("TMUX_PANE"), time.Now().Unix())
			return nil
		},
	})
	return cmd
}
