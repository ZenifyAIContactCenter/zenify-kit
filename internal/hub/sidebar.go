package hub

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ghx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/lock"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/tmuxx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/wt"
	tea "github.com/charmbracelet/bubbletea"
)

// localEvery is how often the lock holder runs a local poll round; a worktree
// added or removed triggers one at once.
const localEvery = 10 * time.Second

// rounds is the poll dependency (*Poller in production).
type rounds interface {
	Local() error
	GitHub() error
}

// tickMsg drives the 1s sidebar loop.
type tickMsg time.Time

func nextTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// polledMsg reports a finished poller round started at at. A local round
// also returns the workspace repos and their worktree fingerprint, both taken
// when it started.
type polledMsg struct {
	at          time.Time
	github      bool  // a GitHub round; otherwise a local one
	githubAt    int64 // State.GitHubAt when a GitHub round started
	ownersKnown bool  // State.Owners was non-empty when a GitHub round started
	repos       []string
	fingerprint string
	err         error
}

// openedMsg reports `gh pr view --web` for pr.
type openedMsg struct {
	pr  PR
	err error
}

// Model is the hub sidebar. Every sidebar tries to take the hub lock each
// tick; only the holder runs the poller, off the UI loop, with at most one
// local and one GitHub round in flight. All of them re-read state.json when
// it changes.
type Model struct {
	home, root string
	self       string // zenify binary, for the sidebar of a new agent window
	selfErr    error
	tm         tmuxx.Runner
	gh         ghx.Runner
	poll       rounds
	every      func() tea.Cmd // schedules the next tickMsg
	clip       clipboard
	pid        int
	host       string

	lock        *lock.Handle
	lastLocal   time.Time
	lastGitHub  time.Time
	refreshSeen time.Time
	stateMod    time.Time
	localBusy   bool // a local round is in flight
	localAgain  bool // worktrees changed while a local round was in flight
	githubBusy  bool // a GitHub round is in flight
	// githubRetried: the first-run GitHub retry already ran; reset once
	// State.GitHubAt is set.
	githubRetried bool
	repos         []string // wt-managed workspace repos, refreshed by each local round
	fingerprint   string   // worktree dirs of repos, as last seen

	state    State
	items    []Item
	cursor   int
	expanded bool            // the merged group is open
	folds    map[string]bool // repo name → open; only the user's choices, default otherwise
	width    int
	height   int

	moved bool // the user moved the cursor; until then it stays on the first row

	err       error  // last tick's lock/state error
	loggedErr string // err as last written to hub.log, so a repeated error logs once
	localErr  error  // last local round's error, nil once one succeeds
	githubErr error  // last GitHub round's error, nil once one succeeds
	notice    string // result of the last key action
	failed    bool   // notice is an error
	prompt    string // pending y/n question
	confirm   func(Model) Model
	// resuming holds when Enter last resumed each closed agent, so a second
	// Enter before it shows open does not start it twice.
	resuming map[string]time.Time
}

// NewModel builds the sidebar for the workspace root; poller.GH opens PRs.
func NewModel(home, root string, tm tmuxx.Runner, poller *Poller) Model {
	m := Model{home: home, root: root, tm: tm, gh: poller.GH, poll: poller, every: nextTick, clip: newClipboard(os.Stdout), pid: os.Getpid()}
	m.self, m.selfErr = os.Executable()
	host, err := os.Hostname()
	if err != nil {
		LogError(home, fmt.Errorf("hub sidebar: hostname: %w", err), time.Now().Unix())
	}
	m.host = host
	m.folds = map[string]bool{}
	m.resuming = map[string]time.Time{}
	m.relayout()
	return m
}

func (m Model) Init() tea.Cmd {
	return func() tea.Msg { return tickMsg(time.Now()) }
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		next, poll := m.tick(time.Time(msg))
		return next, tea.Batch(m.every(), poll)
	case polledMsg:
		var again tea.Cmd
		if msg.github {
			m.githubBusy = false
		} else {
			m.localBusy, m.repos, m.fingerprint = false, msg.repos, msg.fingerprint
			if m.localAgain {
				m.localAgain = false
				again = m.startLocal(msg.at)
			}
		}
		if msg.err != nil {
			LogError(m.home, msg.err, msg.at.Unix())
		}
		// Kept until the next round of the same kind succeeds; a reload never clears it.
		if msg.github {
			m.githubErr = msg.err
		} else {
			m.localErr = msg.err
		}
		m.err = m.reload()
		if msg.github && msg.err == nil && m.state.GitHubError == "" && m.state.GitHubAt == msg.githubAt &&
			!msg.ownersKnown && len(m.state.Owners) > 0 {
			// Deferred while owners were unknown, and they are known now: retry
			// next tick. Otherwise keep the normal interval, so owners that never
			// appear do not start a round every tick, and a failed round is
			// retried only every GitHubInterval.
			m.lastGitHub = time.Time{}
		}
		m.logErr(msg.at)
		return m, again
	case openedMsg:
		if msg.err == nil {
			msg.err = m.ack(&msg.pr)
		}
		m = m.done("opened "+msg.pr.URL, msg.err)
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		return m.key(msg.String())
	case tea.MouseMsg:
		return m.mouse(msg)
	}
	return m, nil
}

// mouse handles a press (FR-12.4): the wheel moves like j/k; a left click on
// a row selects it, and on the selected row acts like Enter. Clicks on the
// title, headers, rules and footer are ignored, and so is every event while
// a y/n prompt is up.
func (m Model) mouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress || m.confirm != nil {
		return m, nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		return m.key("k")
	case tea.MouseButtonWheelDown:
		return m.key("j")
	case tea.MouseButtonLeft:
		i := m.itemAt(msg.Y)
		if i < 0 || m.items[i].Kind == KindHeader {
			return m, nil
		}
		if i == m.cursor {
			return m.key("enter")
		}
		m.cursor, m.moved = i, true
	}
	return m, nil
}

// tick takes the lock if free, reloads state.json and, when holding the lock,
// returns the poller rounds that are due (nil when none).
func (m Model) tick(now time.Time) (Model, tea.Cmd) {
	var errs []error
	var poll tea.Cmd
	dir := Dir(m.home)
	if m.lock == nil {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			errs = append(errs, fmt.Errorf("create hub dir: %w", err))
		} else if h, err := lock.Acquire(dir, m.pid, m.host, now.Unix()); err == nil {
			m.lock = h
		} else if !errors.Is(err, lock.ErrHeld) {
			errs = append(errs, fmt.Errorf("hub lock: %w", err))
		}
	}
	if m.lock != nil {
		fp, err := worktreeFingerprint(m.repos)
		if err != nil {
			LogError(m.home, err, now.Unix())
		}
		changed := fp != m.fingerprint
		if changed {
			m.fingerprint = fp
		}
		m.retryFirstGitHub()
		if poll, err = m.pollIfDue(now, changed); err != nil {
			errs = append(errs, err)
		}
	}
	if err := m.reload(); err != nil {
		errs = append(errs, err)
	}
	m.err = errors.Join(errs...)
	m.logErr(now)
	return m, poll
}

// retryFirstGitHub makes a GitHub round due once on first run: no round has
// completed yet (GitHubAt 0), owners are known now and none is in flight, so
// the earlier round was deferred while owners were unknown. It does not depend
// on whether a tick or the local round's result loaded the owners first.
func (m *Model) retryFirstGitHub() {
	if m.state.GitHubAt != 0 {
		m.githubRetried = false
		return
	}
	if !m.githubRetried && len(m.state.Owners) > 0 && !m.githubBusy && !m.lastGitHub.IsZero() {
		m.lastGitHub, m.githubRetried = time.Time{}, true
	}
}

// logErr appends m.err to hub.log when it differs from the last one logged,
// so the footer's hub.log hint holds for it too.
func (m *Model) logErr(now time.Time) {
	var s string
	if m.err != nil {
		s = m.err.Error()
	}
	if s != "" && s != m.loggedErr {
		LogError(m.home, m.err, now.Unix())
	}
	m.loggedErr = s
}

// pollIfDue starts a local round every localEvery or at once when the
// worktrees changed, and a GitHub round when its interval passed or the
// refresh file changed, each only when none of its kind is in flight (a
// worktree change during a local round reruns it when it finishes); nil when
// nothing is due.
func (m *Model) pollIfDue(now time.Time, worktreesChanged bool) (tea.Cmd, error) {
	var cmds []tea.Cmd
	p := m.poll
	switch {
	case m.localBusy:
		m.localAgain = m.localAgain || worktreesChanged
	case worktreesChanged || m.lastLocal.IsZero() || now.Sub(m.lastLocal) >= localEvery:
		cmds = append(cmds, m.startLocal(now))
	}
	if m.githubBusy {
		return tea.Batch(cmds...), nil
	}
	refresh, err := modTime(filepath.Join(Dir(m.home), "refresh"))
	if err != nil {
		return tea.Batch(cmds...), err
	}
	if m.lastGitHub.IsZero() || now.Sub(m.lastGitHub) >= GitHubInterval(m.state) || refresh.After(m.refreshSeen) {
		m.githubBusy, m.lastGitHub, m.refreshSeen = true, now, refresh
		since, known := m.state.GitHubAt, len(m.state.Owners) > 0
		cmds = append(cmds, func() tea.Msg {
			return polledMsg{at: now, github: true, githubAt: since, ownersKnown: known, err: p.GitHub()}
		})
	}
	return tea.Batch(cmds...), nil
}

// startLocal marks a local round in flight and returns it. The round first
// re-lists the workspace repos and fingerprints their worktree dirs.
func (m *Model) startLocal(now time.Time) tea.Cmd {
	m.localBusy, m.lastLocal = true, now
	p, root := m.poll, m.root
	return func() tea.Msg {
		repos := wt.WorkspaceRepos(root)
		fp, fpErr := worktreeFingerprint(repos)
		return polledMsg{at: now, repos: repos, fingerprint: fp, err: errors.Join(fpErr, p.Local())}
	}
}

// worktreeFingerprint lists the .worktrees entries of every repo (a missing
// dir counts as empty), so adding or removing a worktree changes it.
func worktreeFingerprint(repos []string) (string, error) {
	var b strings.Builder
	var errs []error
	for _, r := range repos {
		entries, err := os.ReadDir(filepath.Join(r, ".worktrees")) // sorted by name
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, fmt.Errorf("read %s worktrees: %w", filepath.Base(r), err))
		}
		b.WriteString(r)
		for _, e := range entries {
			b.WriteString("\x00" + e.Name())
		}
		b.WriteString("\n")
	}
	return b.String(), errors.Join(errs...)
}

// modTime is path's mtime, or the zero time when it does not exist.
func modTime(p string) (time.Time, error) {
	fi, err := os.Stat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("stat %s: %w", filepath.Base(p), err)
	}
	return fi.ModTime(), nil
}

// reload re-reads state.json when its mtime changed.
func (m *Model) reload() error {
	mod, err := modTime(statePath(m.home))
	if err != nil || mod.IsZero() || mod.Equal(m.stateMod) {
		return err
	}
	s, err := ReadState(m.home)
	if err != nil {
		return err
	}
	m.state, m.stateMod = s, mod
	for id := range m.resuming {
		if s.Agents[id].Open {
			delete(m.resuming, id)
		}
	}
	m.relayout()
	return sweepResumeLocks(m.home, s.Agents, time.Now())
}

// relayout rebuilds the rows and keeps the cursor on the same row when it
// still exists, then clamps it off headers. Until the user moved the cursor
// it goes to the first selectable row instead.
func (m *Model) relayout() {
	var at string
	if !m.moved {
		m.cursor = 0
	} else if m.cursor >= 0 && m.cursor < len(m.items) {
		at = itemKey(m.items[m.cursor])
	}
	m.items = Flatten(m.state, m.expanded, m.folds)
	for i, it := range m.items {
		if at != "" && itemKey(it) == at {
			m.cursor = i
			break
		}
	}
	m.clampCursor(1)
}

// clampCursor keeps the cursor in range and off headers, stepping in dir.
func (m *Model) clampCursor(dir int) {
	n := len(m.items)
	m.cursor = min(max(m.cursor, 0), n-1)
	for _, d := range []int{dir, -dir} {
		for i := m.cursor; i >= 0 && i < n; i += d {
			if m.items[i].Kind != KindHeader {
				m.cursor = i
				return
			}
		}
	}
}

func (m Model) current() Item { return m.items[m.cursor] }

func (m Model) key(k string) (tea.Model, tea.Cmd) {
	if m.confirm != nil { // only y confirms; any other key (n, esc, q, ctrl+c) cancels
		run := m.confirm
		m.confirm, m.prompt = nil, ""
		if k == "y" {
			m = run(m)
		}
		return m, nil
	}
	it := m.current()
	switch k {
	case "q", "ctrl+c":
		// A round in flight still writes state.json: leave the lock to process
		// exit, which frees the flock.
		if !m.localBusy && !m.githubBusy {
			if err := m.lock.Release(); err != nil {
				LogError(m.home, fmt.Errorf("release hub lock: %w", err), time.Now().Unix())
			}
			m.lock = nil
		}
		return m, tea.Quit
	case "j", "down":
		m.moved = true
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
		m.clampCursor(1)
	case "k", "up":
		m.moved = true
		if m.cursor > 0 {
			m.cursor--
		}
		m.clampCursor(-1)
	case "enter", " ":
		m.moved = true
		switch {
		case it.Kind == KindMergedGroup:
			m.expanded = !m.expanded
			m.relayout()
		case it.Kind == KindRepoGroup:
			m.folds[it.Title] = !it.Expanded
			m.relayout()
		case k == " ": // Space only folds
		default:
			if a, ok := TargetAgent(m.state, it); ok && a.Open {
				m = m.jump(a, it.PR)
			} else if ok {
				m = m.resume(a)
			} else if it.Task != nil {
				m = m.ask("y/n · open new agent for "+it.Task.Slug+"?", Model.openAgent)
			} else if it.Kind == KindPR && it.PR != nil {
				return m, m.openPR(*it.PR)
			}
		}
	case "n":
		m = m.openAgent()
	case "o":
		if it.PR != nil {
			return m, m.openPR(*it.PR)
		}
	case "y":
		if it.PR != nil {
			m = m.copy(CopyURL(*it.PR))
		}
	case "m":
		if it.PR != nil {
			m = m.copy(MergeCommand(*it.PR))
		}
	case "r":
		m = m.done("refresh requested", touch(filepath.Join(Dir(m.home), "refresh"), time.Now()))
	case "f":
		m = m.fix(it)
	case "x":
		m = m.forget(it)
	}
	return m, nil
}

// fix types /znf:fix <failed job> into the task's agent, asking first when the
// agent is blocked (the text could land in a permission dialog).
func (m Model) fix(it Item) Model {
	if it.PR == nil || it.PR.Checks != "FAILURE" {
		return m
	}
	if it.PR.FailedURL == "" {
		m.notice, m.failed = "no failed job URL for this PR", false
		return m
	}
	a, ok := TargetAgent(m.state, it)
	if !ok || it.Kind == KindPR {
		m.notice, m.failed = "no agent on this task", false
		return m
	}
	if !a.Open {
		m.notice, m.failed = closedNotice, false
		return m
	}
	a, m, ok = m.freshAgent(a)
	if !ok {
		return m
	}
	pr := *it.PR
	send := func(m Model) Model {
		// The session may have ended, closed or moved while the prompt was up.
		if !m.state.Agents[a.SessionID].Open {
			m.notice, m.failed = closedNotice, false
			return m
		}
		a, m, ok := m.freshAgent(a)
		if !ok {
			return m
		}
		// A pane id is only meaningful on the server that recorded it.
		pid, err := tmuxx.ServerPID(m.tm)
		if err != nil {
			return m.done("", err)
		}
		if a.TmuxServer != pid {
			m.notice, m.failed = "agent runs on another tmux server", false
			return m
		}
		err = tmuxx.SendKeys(m.tm, a.Pane, FixPrompt(pr))
		if err == nil {
			err = m.ack(&pr)
		}
		return m.done("sent to "+a.Pane, err)
	}
	if a.State == Blocked {
		return m.ask("y/n · send to waiting agent?", send)
	}
	return send(m)
}

// freshAgent re-reads a's agent file, since the cached state may be up to a
// poll round old, and returns a with its current state, pane and server; ok is false
// (with the notice or error set on m) when the file is gone or unreadable.
func (m Model) freshAgent(a Agent) (Agent, Model, bool) {
	fresh, err := readAgent(m.home, a.SessionID)
	if err != nil {
		return a, m.done("", err), false
	}
	if fresh == nil { // SessionEnd removed the file
		return a, m.done("agent session ended", nil), false
	}
	a.State, a.TmuxServer = fresh.State, fresh.TmuxServer
	if fresh.Pane != "" {
		a.Pane = fresh.Pane
	}
	return a, m, true
}

// jump focuses a's pane, switching the client to its session first (a no-op
// in the same session), then marks it seen and acks pr's alert (pr may be nil).
func (m Model) jump(a Agent, pr *PR) Model {
	if _, err := m.tm.Run("switch-client", "-t", a.Pane); err != nil {
		return m.done("", err)
	}
	if err := tmuxx.SelectPane(m.tm, a.Pane); err != nil {
		return m.done("", err)
	}
	return m.done("", errors.Join(MarkSeen(m.home, a.SessionID), m.ack(pr)))
}

// openPR opens pr in the browser; its alert is acked once that succeeded.
func (m Model) openPR(pr PR) tea.Cmd {
	gh := m.gh
	return func() tea.Msg {
		_, err := gh.Run("pr", "view", strconv.Itoa(pr.Number), "--repo", pr.Repo, "--web")
		return openedMsg{pr: pr, err: err}
	}
}

func (m Model) openAgent() Model {
	if m.selfErr != nil {
		return m.done("", fmt.Errorf("locate zenify binary: %w", m.selfErr))
	}
	return m.done("opened a new agent window", NewAgentWindow(m.tm, m.root, m.self))
}

// closedNotice answers f on an agent whose pane is gone: typing into it could
// land in whatever reuses the pane id.
const closedNotice = "agent is closed — press Enter to resume"

// resumeGuard is how long a resumed agent may take to show open before Enter
// may resume it again.
const resumeGuard = 15 * time.Second

// resume opens a new window running `claude --resume` for the closed agent a,
// unless Enter already resumed it within resumeGuard, another sidebar holds
// its resume lock, or a fresh check finds it open.
func (m Model) resume(a Agent) Model {
	if m.selfErr != nil {
		return m.done("", fmt.Errorf("locate zenify binary: %w", m.selfErr))
	}
	now := time.Now()
	if at, ok := m.resuming[a.SessionID]; ok && now.Sub(at) < resumeGuard {
		m.notice, m.failed = "already resuming", false
		return m
	}
	if !ValidSessionID(a.SessionID) { // it names the lock file below
		return m.done("", fmt.Errorf("invalid session id %q", a.SessionID))
	}
	got, err := takeResumeLock(m.home, a.SessionID, now)
	if err != nil {
		return m.done("", err)
	}
	if !got {
		m.notice, m.failed = "already resuming in another sidebar", false
		return m
	}
	// The cached state may be a poll round old: another sidebar may have
	// resumed the agent already.
	fresh, open, err := freshLiveness(m.home, m.tm, a.SessionID)
	switch {
	case err != nil:
		return m.done("", errors.Join(err, removeResumeLock(m.home, a.SessionID)))
	case fresh == nil:
		return m.done("agent session ended", removeResumeLock(m.home, a.SessionID))
	case open:
		return m.done("agent is already open", removeResumeLock(m.home, a.SessionID))
	}
	err = ResumeAgentWindow(m.tm, m.root, m.self, a.SessionID, WindowName(a.Slug))
	if err != nil {
		return m.done("", errors.Join(err, removeResumeLock(m.home, a.SessionID)))
	}
	m.resuming[a.SessionID] = now
	return m.done("resumed "+agentLabel(&a)+" in a new window", nil)
}

// freshLiveness re-reads id's agent file and asks tmux whether its pane is
// live right now; a is nil when the file is gone.
func freshLiveness(home string, tm tmuxx.Runner, id string) (a *Agent, open bool, err error) {
	a, err = readAgent(home, id)
	if err != nil || a == nil {
		return a, false, err
	}
	marked, err := MarkOpen(tm, map[string]Agent{id: *a})
	if err != nil {
		return a, false, fmt.Errorf("check agent %s liveness: %w", id, err)
	}
	return a, marked[id].Open, nil
}

// resumeLockTTL is how long a resume lock keeps other sidebars from resuming
// the same agent.
const resumeLockTTL = 30 * time.Second

func resumeLockPath(home, id string) string {
	return filepath.Join(AgentsDir(home), id+".resuming")
}

// takeResumeLock creates agents/<id>.resuming holding now as unix seconds,
// so sidebars in other processes do not resume the same agent. The lock is
// created already filled (temp file + hard link), so no reader sees it empty.
// A lock older than resumeLockTTL is replaced; a younger one yields false.
func takeResumeLock(home, id string, now time.Time) (bool, error) {
	if err := os.MkdirAll(AgentsDir(home), 0o700); err != nil {
		return false, fmt.Errorf("create agents dir: %w", err)
	}
	p := resumeLockPath(home, id)
	for attempt := 0; attempt < 2; attempt++ {
		err := linkResumeLock(p, now)
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return false, fmt.Errorf("create resume lock %s: %w", id, err)
		}
		at, mtime, err := readResumeLock(p)
		if err != nil {
			return false, err
		}
		if now.Sub(at) < resumeLockTTL {
			return false, nil
		}
		if ok, err := removeStaleResumeLock(p, mtime); err != nil || !ok {
			return false, err
		}
	}
	return false, nil // another sidebar replaced the stale lock first
}

// linkResumeLock writes now to a temp file next to p and hard-links it to p,
// which fails with fs.ErrExist when the lock is held.
func linkResumeLock(p string, now time.Time) error {
	tmp, err := os.CreateTemp(filepath.Dir(p), ".resume-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, werr := tmp.WriteString(strconv.FormatInt(now.Unix(), 10))
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return errors.Join(werr, os.Remove(name))
	}
	lerr := os.Link(name, p)
	if rerr := os.Remove(name); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
		return errors.Join(lerr, rerr)
	}
	return lerr
}

// removeStaleResumeLock removes the lock at p only when its mtime is still
// seen; a changed mtime means another sidebar took it, so it counts as held
// (false). A lock that is already gone is reported as removed.
func removeStaleResumeLock(p string, seen time.Time) (bool, error) {
	fi, err := os.Stat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat resume lock %s: %w", filepath.Base(p), err)
	}
	if !fi.ModTime().Equal(seen) {
		return false, nil
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("remove resume lock %s: %w", filepath.Base(p), err)
	}
	return true, nil
}

// resumeLockTime reads the time a resume lock was taken: the unix seconds it
// holds, or the file mtime when the content is empty or unparsable.
func resumeLockTime(p string) (time.Time, error) {
	at, _, err := readResumeLock(p)
	return at, err
}

// ResumeLocked reports whether agent id has a resume lock taken less than
// resumeLockTTL before now, i.e. a sidebar is resuming it right now.
func ResumeLocked(home, id string, now time.Time) (bool, error) {
	at, err := resumeLockTime(resumeLockPath(home, id))
	if err != nil || at.IsZero() {
		return false, err
	}
	return now.Sub(at) < resumeLockTTL, nil
}

// readResumeLock returns the lock time and the file mtime. A missing file
// yields the zero time; an empty or unparsable lock falls back to the mtime.
func readResumeLock(p string) (at, mtime time.Time, err error) {
	fi, err := os.Stat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return time.Time{}, time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("stat resume lock %s: %w", filepath.Base(p), err)
	}
	b, err := os.ReadFile(p) //nolint:gosec // G304 -- path is built from the hub agents dir, not external input
	if errors.Is(err, fs.ErrNotExist) {
		return time.Time{}, time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("read resume lock %s: %w", filepath.Base(p), err)
	}
	sec, perr := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if perr != nil {
		return fi.ModTime(), fi.ModTime(), nil
	}
	return time.Unix(sec, 0), fi.ModTime(), nil
}

func removeResumeLock(home, id string) error {
	if err := os.Remove(resumeLockPath(home, id)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove resume lock %s: %w", id, err)
	}
	return nil
}

// sweepResumeLocks removes the resume locks of agents that show open and of
// locks older than resumeLockTTL.
func sweepResumeLocks(home string, agents map[string]Agent, now time.Time) error {
	entries, err := os.ReadDir(AgentsDir(home))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read agents dir: %w", err)
	}
	var errs []error
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".resuming")
		if !ok || e.IsDir() {
			continue
		}
		if !agents[id].Open {
			at, err := resumeLockTime(resumeLockPath(home, id))
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if now.Sub(at) < resumeLockTTL {
				continue
			}
		}
		errs = append(errs, removeResumeLock(home, id))
	}
	return errors.Join(errs...)
}

// forget asks, then deletes the file of the closed agent on an AGENTS row.
// At confirm time it re-reads the file and liveness, and refuses when the
// agent opened again or its record changed since the prompt.
func (m Model) forget(it Item) Model {
	if it.Kind != KindAgent || it.Agent == nil {
		return m
	}
	a := *it.Agent
	if a.Open {
		m.notice, m.failed = "agent is running", false
		return m
	}
	name := agentLabel(&a)
	return m.ask("y/n · forget "+name+"?", func(m Model) Model {
		fresh, open, err := freshLiveness(m.home, m.tm, a.SessionID)
		switch {
		case err != nil:
			return m.done("", err)
		case fresh == nil:
			return m.done("agent session ended", nil)
		case open:
			return m.done("agent is open again", nil)
		case fresh.UpdatedAt != a.UpdatedAt:
			return m.done("agent changed", nil)
		}
		return m.done("forgot agent "+name, ForgetAgent(m.home, a.SessionID))
	})
}

func (m Model) copy(s string) Model {
	return m.done("copied "+s, m.clip(s))
}

// ack clears pr's alert, if it has one. Call it only after the action ran.
func (m Model) ack(pr *PR) error {
	if pr == nil || pr.Alert == "" {
		return nil
	}
	return Ack(m.home, *pr)
}

func (m Model) ask(prompt string, run func(Model) Model) Model {
	m.prompt, m.confirm = prompt, run
	return m
}

// done shows err in the footer and appends it to hub.log, else shows msg (an
// empty msg clears the footer).
func (m Model) done(msg string, err error) Model {
	if err != nil {
		LogError(m.home, err, time.Now().Unix())
		m.notice, m.failed = err.Error(), true
	} else {
		m.notice, m.failed = msg, false
	}
	return m
}
