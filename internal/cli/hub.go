package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
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
	home     func() (string, error)
	lookPath func(string) (string, error)
	runner   tmuxx.Runner
	attach   func(args ...string) error
	self     func() (string, error)
}

var hubLauncher = hubEnv{
	home:     os.UserHomeDir,
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
		if err := buildHubSession(env.runner, root, self, resumable(env, time.Now().Unix())); err != nil {
			return fmt.Errorf("setting up tmux session failed: %w", err)
		}
	}
	if err := env.attach("attach", "-t", tmuxx.Target); err != nil {
		return fmt.Errorf("tmux attach: %w", err)
	}
	return nil
}

// shellQuote single-quotes s for sh, escaping embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// resumeMaxAge and resumeMax bound the agents a new hub session resumes.
const (
	resumeMaxAge = 7 * 24 * 3600 // seconds
	resumeMax    = 8
)

// resumable loads the agent files and returns the closed agents a new hub
// session resumes (FR-11.3). Errors go to hub.log (stderr when home is
// unknown); an unreadable tmux server resumes nothing, since an agent still
// running there must not be started twice.
func resumable(env hubEnv, now int64) []hub.Agent {
	home, err := env.home()
	if err != nil {
		fmt.Fprintf(os.Stderr, "zenify hub: resolve home, no agent resumed: %v\n", err)
		return nil
	}
	agents, err := hub.LoadAgents(home)
	if err != nil {
		hub.LogError(home, fmt.Errorf("hub: load agents to resume: %w", err), now)
	}
	if len(agents) == 0 {
		return nil
	}
	if agents, err = hub.MarkOpen(env.runner, agents); err != nil {
		hub.LogError(home, fmt.Errorf("hub: tmux liveness, no agent resumed: %w", err), now)
		return nil
	}
	out, err := resumeCandidates(agents, now)
	if err != nil {
		hub.LogError(home, fmt.Errorf("hub: resume: %w", err), now)
	}
	return out
}

// resumeCandidates keeps the closed agents updated within resumeMaxAge whose
// session id is safe for a command line, newest first, at most resumeMax.
// Unsafe ids are skipped and returned as errors.
func resumeCandidates(agents map[string]hub.Agent, now int64) ([]hub.Agent, error) {
	var out []hub.Agent
	var errs []error
	for _, a := range agents {
		if a.Open || now-a.UpdatedAt > resumeMaxAge {
			continue
		}
		if !hub.ValidSessionID(a.SessionID) {
			errs = append(errs, fmt.Errorf("skip agent: invalid session id %q", a.SessionID))
			continue
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt != out[j].UpdatedAt {
			return out[i].UpdatedAt > out[j].UpdatedAt
		}
		return out[i].SessionID < out[j].SessionID
	})
	if len(out) > resumeMax {
		out = out[:resumeMax]
	}
	return out, errors.Join(errs...)
}

// buildHubSession creates the detached session: the first window runs a new
// claude, or resumes resume[0], and every further agent in resume gets its own
// window; each window has the sidebar pane on its left. When new-session fails
// (another run may have created the session meanwhile) it kills nothing; when
// a later step fails it kills the session it created, since a half-built one
// would make the next run attach without a sidebar.
func buildHubSession(r tmuxx.Runner, root, self string, resume []hub.Agent) error {
	name, command := "agent", []string{"claude"}
	if len(resume) > 0 {
		name, command = hub.WindowName(resume[0].Slug), []string{"claude", "--resume", resume[0].SessionID}
	}
	out, err := r.Run(append([]string{"new-session", "-d", "-s", tmuxx.Session, "-P", "-F", "#{window_id}", "-c", root, "-n", name}, command...)...)
	if err != nil {
		return err
	}
	win := strings.TrimSpace(string(out))
	if win == "" {
		err = errors.New("tmux new-session printed no window id")
	} else {
		err = sidebarPane(r, root, self, win)
	}
	if err != nil {
		_, kerr := r.Run("kill-session", "-t", tmuxx.Target)
		return errors.Join(err, kerr)
	}
	for _, a := range resume[min(1, len(resume)):] {
		out, err := r.Run("new-window", "-d", "-t", tmuxx.Target+":", "-P", "-F", "#{window_id}", "-n", hub.WindowName(a.Slug), "-c", root, "claude", "--resume", a.SessionID)
		win := strings.TrimSpace(string(out))
		if err == nil && win == "" {
			err = errors.New("tmux new-window printed no window id")
		}
		if err == nil {
			err = sidebarPane(r, root, self, win)
		}
		if err != nil {
			_, kerr := r.Run("kill-session", "-t", tmuxx.Target)
			return errors.Join(err, kerr)
		}
	}
	return nil
}

// sidebarPane splits the hub sidebar to the left of window target, then
// enables allow-passthrough there. Only the split can fail: passthrough is
// best effort (OSC 52 copy needs it, tmux < 3.3 rejects the option) and its
// error goes to hub.log.
func sidebarPane(r tmuxx.Runner, root, self, target string) error {
	if _, err := r.Run("split-window", "-d", "-h", "-b", "-l", "30%", "-t", target, "-c", root, shellQuote(self)+" hub sidebar"); err != nil {
		return err
	}
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
	if err := hub.Report(home, stdin, pane, os.Getenv("TMUX"), now); err != nil {
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
