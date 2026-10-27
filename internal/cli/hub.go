package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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

// runHubLauncher opens (or re-attaches to) the zenify tmux session on the
// hub's own tmux server (tmuxx.Socket): the agent window on the right, the hub
// sidebar pane on the left. It rewrites the server's tmux.conf every launch.
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
	if t := os.Getenv("TMUX"); t != "" {
		if filepath.Base(hub.TmuxSocket(t)) == tmuxx.Socket() {
			fmt.Fprintln(cmd.OutOrStdout(), "zenify hub is already open in this tmux — Alt+n opens a new agent")
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "already inside another tmux — run \"zenify hub\" from a plain terminal, or: env -u TMUX tmux -L %s attach -t %s\n", tmuxx.Socket(), tmuxx.Target)
		}
		return nil
	}
	home, err := env.home()
	if err != nil {
		return fmt.Errorf("resolve home: %w", err)
	}
	self, err := env.self()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	conf, err := hub.WriteTmuxConf(home, self)
	if err != nil {
		return err
	}
	if !tmuxx.HasSession(env.runner) {
		if err := buildHubSession(env.runner, root, self, conf, resumable(env, time.Now().Unix())); err != nil {
			return fmt.Errorf("setting up tmux session failed: %w", err)
		}
	} else if _, err := env.runner.Run("source-file", conf); err != nil {
		// The session still works with the config it started with.
		hub.LogError(home, fmt.Errorf("hub: reload tmux.conf: %w", err), time.Now().Unix())
	}
	if err := env.attach("-L", tmuxx.Socket(), "attach", "-t", tmuxx.Target); err != nil {
		return fmt.Errorf("tmux attach: %w", err)
	}
	return nil
}

// resumeMaxAge and resumeMax bound the agents a new hub session resumes.
const (
	resumeMaxAge = 7 * 24 * 3600 // seconds
	resumeMax    = 8
)

// resumable loads the agent files and returns the closed agents a new hub
// session resumes (FR-11.3), skipping agents a sidebar is resuming (fresh
// agents/<id>.resuming lock). Errors go to hub.log (stderr when home is
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
	for id := range agents { // a sidebar resuming it right now would start it twice
		locked, err := hub.ResumeLocked(home, id, time.Unix(now, 0))
		if err != nil {
			hub.LogError(home, fmt.Errorf("hub: resume lock, agent %s not resumed: %w", id, err), now)
		}
		if locked || err != nil {
			delete(agents, id)
		}
	}
	out, err := resumeCandidates(agents, now)
	if err != nil {
		hub.LogError(home, fmt.Errorf("hub: resume: %w", err), now)
	}
	return out
}

// resumeCandidates keeps the closed agents, except those of another live
// tmux server (hub.Agent.OtherTmux: they may still run there), updated
// within resumeMaxAge whose
// session id is safe for a command line, newest first, at most resumeMax.
// Unsafe ids are skipped and returned as errors.
func resumeCandidates(agents map[string]hub.Agent, now int64) ([]hub.Agent, error) {
	var out []hub.Agent
	var errs []error
	for _, a := range agents {
		if a.Open || a.OtherTmux() || now-a.UpdatedAt > resumeMaxAge {
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

// buildHubSession creates the detached session, starting the hub server with
// config file conf when none runs: the first window runs a new
// claude, or resumes resume[0], and every further agent in resume gets its own
// window; each window has the sidebar pane on its left. When new-session fails
// (another run may have created the session meanwhile) it kills nothing; when
// a later step fails it kills the session it created, since a half-built one
// would make the next run attach without a sidebar.
func buildHubSession(r tmuxx.Runner, root, self, conf string, resume []hub.Agent) error {
	name, command := "agent", []string{"claude"}
	if len(resume) > 0 {
		name, command = hub.WindowName(resume[0].Slug), []string{"claude", "--resume", resume[0].SessionID}
	}
	out, err := r.Run(append([]string{"-f", conf, "new-session", "-d", "-s", tmuxx.Session, "-P", "-F", "#{window_id}", "-c", root, "-n", name}, command...)...)
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

// sidebarPane splits the hub sidebar to the left of window target.
func sidebarPane(r tmuxx.Runner, root, self, target string) error {
	_, err := r.Run("split-window", "-d", "-h", "-b", "-l", "30%", "-t", target, "-c", root, hub.ShellQuote(self)+" hub sidebar")
	return err
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
	_, err = tea.NewProgram(hub.NewModel(home, root, tm, poller), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
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

// hubNewAgent opens a new agent window in the workspace that holds cwd, or,
// when cwd is outside one, in the workspace of the zenify session's start
// directory.
func hubNewAgent(r tmuxx.Runner, cwd, self string) error {
	root, ok := wt.FindWorkspaceRoot(cwd)
	if !ok {
		out, err := r.Run("display-message", "-p", "-t", tmuxx.Target, "#{session_path}")
		if err != nil {
			return fmt.Errorf("hub new-agent: session path: %w", err)
		}
		dir := strings.TrimSpace(string(out))
		if root, ok = wt.FindWorkspaceRoot(dir); !ok {
			return fmt.Errorf("hub new-agent: %s is not inside a zenify workspace", dir)
		}
	}
	return hub.NewAgentWindow(r, root, self)
}

// runHubNewAgent is `zenify hub new-agent`, run by tmux run-shell from Alt+n
// or a click on [+ agent]. It always exits 0, since run-shell would show any
// output over the pane: errors go to hub.log (stderr when home is unknown).
func runHubNewAgent(env hubEnv) {
	err := func() error {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("hub new-agent: resolve cwd: %w", err)
		}
		self, err := env.self()
		if err != nil {
			return fmt.Errorf("hub new-agent: resolve executable: %w", err)
		}
		return hubNewAgent(env.runner, cwd, self)
	}()
	if err == nil {
		return
	}
	home, herr := env.home()
	if herr != nil {
		fmt.Fprintf(os.Stderr, "zenify hub: resolve home: %v (original error: %v)\n", herr, err)
		return
	}
	hub.LogError(home, err, time.Now().Unix())
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
		Use:    "new-agent",
		Short:  "Open a new agent window (bound to Alt+n and [+ agent] in the hub tmux server)",
		Hidden: true,
		Args:   cobra.NoArgs,
		Run:    func(*cobra.Command, []string) { runHubNewAgent(hubLauncher) },
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
