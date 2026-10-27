package hub

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// scriptTmux records every tmux call and answers with out[subcommand], or
// with fail[subcommand] when set.
type scriptTmux struct {
	out   map[string]string
	fail  map[string]error
	calls [][]string
}

func (f *scriptTmux) Run(args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	if err := f.fail[args[0]]; err != nil {
		return nil, err
	}
	return []byte(f.out[args[0]]), nil
}

// countTicker counts the local and GitHub rounds it ran. A GitHub round
// stamps State.GitHubAt in home, as a real round with known owners does,
// unless deferred is set (owners unknown: GitHubAt stays put).
type countTicker struct {
	home          string
	deferred      bool
	local, github int
}

func (c *countTicker) Local() error {
	c.local++
	return nil
}

func (c *countTicker) GitHub() error {
	c.github++
	if c.deferred {
		return nil
	}
	s, err := ReadState(c.home)
	if err != nil {
		return err
	}
	s.GitHubAt = time.Now().Unix()
	return WriteState(c.home, s)
}

// t0 is in the past so files touched at wall-clock time are newer.
var t0 = time.Unix(1_700_000_000, 0)

// newTestModel builds a sidebar over home with fake tmux, gh and poll
// dependencies, writes s as state.json and runs one tick to load it.
func newTestModel(t *testing.T, home string, s State) (Model, *scriptTmux, *countTicker) {
	t.Helper()
	if err := WriteState(home, s); err != nil {
		t.Fatal(err)
	}
	tm := &scriptTmux{}
	ct := &countTicker{home: home}
	m := NewModel(home, "/ws", tm, &Poller{})
	m.poll = ct
	m.every = func() tea.Cmd { return nil } // tests send tickMsg themselves
	m.gh = &fakeGH{}
	m.clip = osc52Clipboard(&bytes.Buffer{})
	m.self = "/bin/zenify"
	m = update(t, m, tickMsg(t0))
	t.Cleanup(func() {
		if err := m.lock.Release(); err != nil {
			t.Error(err)
		}
	})
	return m, tm, ct
}

// update applies msg and then runs every command it returns (polls, gh
// calls) to completion, feeding their messages back in.
func update(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	m, cmd := send(t, m, msg)
	return drain(t, m, cmd)
}

// send applies msg without running the returned command.
func send(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

func drain(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	switch msg := cmd().(type) {
	case nil, tea.QuitMsg:
	case tea.BatchMsg:
		for _, c := range msg {
			m = drain(t, m, c)
		}
	default:
		m = update(t, m, msg)
	}
	return m
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// liveAgents writes every agent of s as its agent file, as the hook would for
// a running session, and returns s.
func liveAgents(t *testing.T, home string, s State) State {
	t.Helper()
	for _, a := range s.Agents {
		if err := writeAgent(home, &a); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

// fixState is one task t1 with repo be (PR 812 failing, alert ci-failed) and
// an agent on pane %3 in the given state.
func fixState(agentState AgentState) State {
	pr := PR{
		Number: 812, Repo: "o/r", URL: "https://github.com/o/r/pull/812", Checks: "FAILURE",
		FailedURL: "https://github.com/o/r/actions/runs/1/job/2", Alert: AlertCIFailed, AlertAt: 1,
	}
	return State{
		Tasks:  []Task{{Slug: "t1", Repos: []TaskRepo{{Name: "be", Repo: "o/r", PR: &pr}}, Agents: []string{"s1"}, State: agentState}},
		Agents: map[string]Agent{"s1": {SessionID: "s1", Pane: "%3", State: agentState, Slug: "t1", UpdatedAt: 5}},
	}
}

func TestView_ShowsError(t *testing.T) {
	m, _, _ := newTestModel(t, t.TempDir(), State{Error: "gh: rate limited"})
	if !strings.Contains(m.View(), "gh: rate limited") {
		t.Errorf("View missing error:\n%s", m.View())
	}
}

func TestView_Rows(t *testing.T) {
	m, _, _ := newTestModel(t, t.TempDir(), fixState(Blocked))
	v := m.View()
	for _, want := range []string{"TASKS 1", "   ◆ t1  s1\n", "       #812 ✗ !\n", "AGENTS 0", "MY PRS 0"} {
		if !strings.Contains(v, want) {
			t.Errorf("View missing %q:\n%s", want, v)
		}
	}
}

func TestEnter_SelectsPane(t *testing.T) {
	m, tm, _ := newTestModel(t, t.TempDir(), fixState(Done))
	m = update(t, m, key("j")) // worktree row t1
	update(t, m, key("enter"))
	want := [][]string{{"switch-client", "-t", "%3"}, {"select-window", "-t", "%3"}, {"select-pane", "-t", "%3"}}
	if !reflect.DeepEqual(tm.calls, want) {
		t.Errorf("tmux calls = %q, want %q", tm.calls, want)
	}
}

func TestEnter_MarksSeen(t *testing.T) {
	home := t.TempDir()
	if err := writeAgent(home, &Agent{SessionID: "s1", Pane: "%3", State: Done, Slug: "t1"}); err != nil {
		t.Fatal(err)
	}
	m, _, _ := newTestModel(t, home, fixState(Done))
	m = update(t, m, key("j"))
	update(t, m, key("enter"))
	a, err := readAgent(home, "s1")
	if err != nil || a.State != Idle {
		t.Errorf("agent after enter = %+v, %v; want idle", a, err)
	}
}

func TestEnter_NoAgentAsksThenOpens(t *testing.T) {
	m, tm, _ := newTestModel(t, t.TempDir(), State{Tasks: []Task{{Slug: "t2", Repos: []TaskRepo{{Name: "be"}}}}})
	tm.out = map[string]string{"new-window": "@9\n"}
	m = update(t, m, key("enter")) // open the idle repo be
	m = update(t, m, key("j"))
	m = update(t, m, key("enter"))
	if !strings.Contains(m.View(), "Open a new agent for t2? y/n") {
		t.Fatalf("View missing prompt:\n%s", m.View())
	}
	if len(tm.calls) != 0 {
		t.Fatalf("tmux called before confirm: %q", tm.calls)
	}
	update(t, m, key("y"))
	if len(tm.calls) != 4 || tm.calls[0][0] != "has-session" || tm.calls[1][0] != "new-window" || tm.calls[2][0] != "split-window" || tm.calls[3][0] != "set-option" {
		t.Errorf("tmux calls = %q", tm.calls)
	}
}

func TestKeyF_SendsFix(t *testing.T) {
	home := t.TempDir()
	m, tm, _ := newTestModel(t, home, liveAgents(t, home, fixState(Idle)))
	m = update(t, m, key("j"))
	m = update(t, m, key("j")) // PR row #812
	update(t, m, key("f"))
	want := [][]string{
		{"send-keys", "-t", "%3", "-l", "/znf:fix https://github.com/o/r/actions/runs/1/job/2"},
		{"send-keys", "-t", "%3", "Enter"},
	}
	if !reflect.DeepEqual(tm.calls, want) {
		t.Errorf("tmux calls = %q, want %q", tm.calls, want)
	}
	if _, err := os.Stat(filepath.Join(home, ".zenify", "hub", "acks", "o_r_812")); err != nil {
		t.Errorf("ack file: %v", err)
	}
}

func TestKeyF_BlockedAsks(t *testing.T) {
	home := t.TempDir()
	m, tm, _ := newTestModel(t, home, liveAgents(t, home, fixState(Blocked)))
	m = update(t, m, key("j"))
	m = update(t, m, key("j")) // PR row #812
	m = update(t, m, key("f"))
	if len(tm.calls) != 0 {
		t.Fatalf("sent before confirm: %q", tm.calls)
	}
	if !strings.Contains(m.View(), "Agent is waiting for input — send anyway? y/n") {
		t.Fatalf("View missing prompt:\n%s", m.View())
	}
	update(t, m, key("y"))
	if len(tm.calls) != 2 || tm.calls[0][0] != "send-keys" {
		t.Errorf("tmux calls after y = %q", tm.calls)
	}
}

func TestKeyF_NoAgent(t *testing.T) {
	s := fixState(Idle)
	s.Tasks[0].Agents = nil
	m, tm, _ := newTestModel(t, t.TempDir(), s)
	m = update(t, m, key("j"))
	m = update(t, m, key("j")) // PR row #812
	m = update(t, m, key("f"))
	if len(tm.calls) != 0 {
		t.Errorf("tmux called: %q", tm.calls)
	}
	if !strings.Contains(m.View(), "no agent on this task") {
		t.Errorf("View missing notice:\n%s", m.View())
	}
}

func TestKeyF_NotFailingDoesNothing(t *testing.T) {
	s := fixState(Idle)
	s.Tasks[0].Repos[0].PR.Checks = "SUCCESS"
	m, tm, _ := newTestModel(t, t.TempDir(), s)
	m = update(t, m, key("j"))
	m = update(t, m, key("j")) // PR row #812
	update(t, m, key("f"))
	if len(tm.calls) != 0 {
		t.Errorf("tmux called: %q", tm.calls)
	}
}

func TestKeyYM_CopyOSC52(t *testing.T) {
	t.Setenv("TMUX", "")
	m, _, _ := newTestModel(t, t.TempDir(), fixState(Idle))
	out := &bytes.Buffer{}
	m.clip = osc52Clipboard(out)
	m = update(t, m, key("j"))
	m = update(t, m, key("j")) // PR row #812
	m = update(t, m, key("y"))
	m = update(t, m, key("m"))
	want := OSC52("https://github.com/o/r/pull/812") + OSC52("gh pr merge 812 --repo o/r --merge")
	if out.String() != want {
		t.Errorf("clipboard writes = %q, want %q", out.String(), want)
	}
}

func TestKeyO_OpensBrowser(t *testing.T) {
	m, _, _ := newTestModel(t, t.TempDir(), fixState(Idle))
	gh := m.gh.(*fakeGH)
	m = update(t, m, key("j"))
	m = update(t, m, key("j")) // PR row #812
	update(t, m, key("o"))
	want := [][]string{{"pr", "view", "812", "--repo", "o/r", "--web"}}
	if !reflect.DeepEqual(gh.args, want) {
		t.Errorf("gh calls = %q, want %q", gh.args, want)
	}
	if _, err := os.Stat(filepath.Join(m.home, ".zenify", "hub", "acks", "o_r_812")); err != nil {
		t.Errorf("ack file after o: %v", err)
	}
}

func TestOnlyLockHolderPolls(t *testing.T) {
	home := t.TempDir()
	a, _, ca := newTestModel(t, home, State{})
	b, _, cb := newTestModel(t, home, State{})
	for i := 1; i <= 6; i++ {
		now := tickMsg(t0.Add(time.Duration(i) * 5 * time.Second))
		a = update(t, a, now)
		b = update(t, b, now)
	}
	if cb.local != 0 || cb.github != 0 {
		t.Errorf("non-holder ran rounds: local=%d github=%d", cb.local, cb.github)
	}
	if b.err != nil {
		t.Errorf("non-holder error = %v, want nil (a held lock is not an error)", b.err)
	}
	// local every 10s: t0, t0+10, t0+20, t0+30; GitHub only on the first round.
	if ca.local != 4 || ca.github != 1 {
		t.Errorf("a rounds: local=%d github=%d, want 4 and 1", ca.local, ca.github)
	}
}

func TestRefreshForcesGitHub(t *testing.T) {
	m, _, ct := newTestModel(t, t.TempDir(), State{})
	m = update(t, m, key("r")) // touches refresh at wall-clock now, after t0
	update(t, m, tickMsg(t0.Add(time.Second)))
	if ct.github != 2 {
		t.Errorf("github rounds = %d, want 2", ct.github)
	}
}

func TestDeferredGitHubRoundRetriesNextTick(t *testing.T) {
	m, _, ct := newTestModel(t, t.TempDir(), State{})
	ct.deferred = true         // owners unknown: the round leaves GitHubAt as it was
	m = update(t, m, key("r")) // forces GitHub round 2
	m = update(t, m, tickMsg(t0.Add(time.Second)))
	update(t, m, tickMsg(t0.Add(2*time.Second)))
	if ct.github != 3 {
		t.Errorf("github rounds = %d, want 3 (the deferred round retried on the next tick)", ct.github)
	}
}

func TestPollOneRoundInFlight(t *testing.T) {
	m, _, ct := newTestModel(t, t.TempDir(), State{})
	m, first := send(t, m, tickMsg(t0.Add(10*time.Second))) // local round due, not finished
	m, second := send(t, m, tickMsg(t0.Add(20*time.Second)))
	m = drain(t, m, second)
	if ct.local != 1 {
		t.Fatalf("local rounds while one is in flight = %d, want 1 (the initial one)", ct.local)
	}
	m = drain(t, m, first)
	update(t, m, tickMsg(t0.Add(30*time.Second)))
	if ct.local != 3 {
		t.Errorf("local rounds after the round finished = %d, want 3", ct.local)
	}
}

// drainHolding runs cmd like drain but holds back GitHub round results,
// as if that round were still running.
func drainHolding(t *testing.T, m Model, cmd tea.Cmd, held *[]tea.Msg) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	switch msg := cmd().(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			m = drainHolding(t, m, c, held)
		}
	case polledMsg:
		if msg.github {
			*held = append(*held, msg)
			return m
		}
		m = update(t, m, msg)
	default:
		m = update(t, m, msg)
	}
	return m
}

func TestLocalRoundsRunWhileGitHubInFlight(t *testing.T) {
	m, _, ct := newTestModel(t, t.TempDir(), State{})
	m = update(t, m, key("r"))
	var held []tea.Msg
	for i := 10; i <= 30; i += 10 {
		var cmd tea.Cmd
		m, cmd = send(t, m, tickMsg(t0.Add(time.Duration(i)*time.Second)))
		m = drainHolding(t, m, cmd, &held)
	}
	// initial local + t0+10, t0+20, t0+30; GitHub: initial + the refresh one, still in flight.
	if ct.local != 4 || ct.github != 2 || len(held) != 1 {
		t.Fatalf("local=%d github=%d held=%d, want 4, 2, 1", ct.local, ct.github, len(held))
	}
	m = update(t, m, held[0])
	m = update(t, m, key("r"))
	update(t, m, tickMsg(t0.Add(40*time.Second)))
	if ct.github != 3 {
		t.Errorf("github rounds after the round finished = %d, want 3", ct.github)
	}
}

func TestView_ReviewState(t *testing.T) {
	s := fixState(Idle)
	pr := s.Tasks[0].Repos[0].PR
	pr.Checks, pr.MergeState, pr.ReviewDecision, pr.UnresolvedThreads, pr.Alert = "SUCCESS", "BLOCKED", "CHANGES_REQUESTED", 2, ""
	m, _, _ := newTestModel(t, t.TempDir(), s)
	if v := m.View(); !strings.Contains(v, "       #812 ✓ BLOCKED changes 2 threads\n") {
		t.Errorf("View missing review state:\n%s", v)
	}
}

func TestKeyF_BlockedDecline(t *testing.T) {
	for _, k := range []string{"n", "q"} {
		home := t.TempDir()
		m, tm, _ := newTestModel(t, home, liveAgents(t, home, fixState(Blocked)))
		m = update(t, m, key("j"))
		m = update(t, m, key("j")) // PR row #812
		m = update(t, m, key("f"))
		m = update(t, m, key(k))
		if len(tm.calls) != 0 {
			t.Errorf("%s: tmux called after decline: %q", k, tm.calls)
		}
		if _, err := os.Stat(filepath.Join(home, ".zenify", "hub", "acks", "o_r_812")); !os.IsNotExist(err) {
			t.Errorf("%s: ack file exists after decline (err=%v)", k, err)
		}
		if strings.Contains(m.View(), "send anyway?") {
			t.Errorf("%s: prompt still shown", k)
		}
		if m.lock == nil {
			t.Errorf("%s: decline released the lock (quit)", k)
		}
	}
}

func TestKeyF_NoAgentKeepsAlert(t *testing.T) {
	home := t.TempDir()
	s := fixState(Idle)
	s.Tasks[0].Agents = nil
	m, _, _ := newTestModel(t, home, s)
	m = update(t, m, key("j"))
	m = update(t, m, key("j")) // PR row #812
	update(t, m, key("f"))
	if _, err := os.Stat(filepath.Join(home, ".zenify", "hub", "acks", "o_r_812")); !os.IsNotExist(err) {
		t.Errorf("ack file exists without an agent (err=%v)", err)
	}
}

func TestKeyO_AckErrorShown(t *testing.T) {
	home := t.TempDir()
	m, _, _ := newTestModel(t, home, fixState(Idle))
	// A file where the acks directory should be makes Ack fail.
	if err := os.WriteFile(filepath.Join(home, ".zenify", "hub", "acks"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	m = update(t, m, key("j"))
	m = update(t, m, key("j")) // PR row #812
	m = update(t, m, key("o"))
	if v := m.View(); !strings.Contains(v, "ack o/r#812") {
		t.Errorf("View missing ack error:\n%s", v)
	}
}

// mkWorkspace creates root/repos/be as a wt-managed repo (a .git dir and
// .claude/worktree.json) and returns root.
func mkWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Join(root, "repos", "be")
	for _, d := range []string{".git", ".claude"} {
		if err := os.MkdirAll(filepath.Join(repo, d), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, ".claude", "worktree.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestNewWorktreeTriggersLocalRound(t *testing.T) {
	root := mkWorkspace(t)
	m, _, ct := newTestModel(t, t.TempDir(), State{})
	m.root = root
	m = update(t, m, tickMsg(t0.Add(10*time.Second))) // cadence round: caches the repo list
	m = update(t, m, tickMsg(t0.Add(11*time.Second)))
	if ct.local != 2 {
		t.Fatalf("local rounds with no change = %d, want 2", ct.local)
	}
	if err := os.MkdirAll(filepath.Join(root, "repos", "be", ".worktrees", "new-task"), 0o750); err != nil {
		t.Fatal(err)
	}
	m = update(t, m, tickMsg(t0.Add(11500*time.Millisecond))) // cadence not due until t0+20
	if ct.local != 3 {
		t.Fatalf("local rounds after a new worktree dir = %d, want 3", ct.local)
	}
	update(t, m, tickMsg(t0.Add(11700*time.Millisecond)))
	if ct.local != 3 {
		t.Errorf("local rounds with an unchanged fingerprint = %d, want 3", ct.local)
	}
}

func TestNewWorktreeWhileLocalInFlightRerunsAfter(t *testing.T) {
	root := mkWorkspace(t)
	m, _, ct := newTestModel(t, t.TempDir(), State{})
	m.root = root
	m = update(t, m, tickMsg(t0.Add(10*time.Second)))
	m, inFlight := send(t, m, tickMsg(t0.Add(20*time.Second))) // cadence round started, not finished
	if err := os.MkdirAll(filepath.Join(root, "repos", "be", ".worktrees", "new-task"), 0o750); err != nil {
		t.Fatal(err)
	}
	m, cmd := send(t, m, tickMsg(t0.Add(20500*time.Millisecond)))
	m = drain(t, m, cmd)
	if ct.local != 2 {
		t.Fatalf("local rounds while one is in flight = %d, want 2", ct.local)
	}
	drain(t, m, inFlight) // finishing it starts the rerun at once
	if ct.local != 4 {
		t.Errorf("local rounds after the in-flight one finished = %d, want 4", ct.local)
	}
}

func TestEnter_ClearsOldNotice(t *testing.T) {
	m, _, _ := newTestModel(t, t.TempDir(), fixState(Done))
	m.notice, m.failed = "old failure", true
	m = update(t, m, key("j"))
	m = update(t, m, key("enter"))
	if m.notice != "" || m.failed {
		t.Errorf("notice %q failed %v after a clean jump, want cleared", m.notice, m.failed)
	}
}

func TestEnter_RepoRowAcksAndMarksSeen(t *testing.T) {
	home := t.TempDir()
	if err := writeAgent(home, &Agent{SessionID: "s1", Pane: "%3", State: Done, Slug: "t1"}); err != nil {
		t.Fatal(err)
	}
	m, _, _ := newTestModel(t, home, fixState(Done))
	m = update(t, m, key("j"))
	m = update(t, m, key("j")) // PR row #812
	update(t, m, key("enter"))
	if _, err := os.Stat(filepath.Join(home, ".zenify", "hub", "acks", "o_r_812")); err != nil {
		t.Errorf("ack file after enter: %v", err)
	}
	if a, err := readAgent(home, "s1"); err != nil || a.State != Idle {
		t.Errorf("agent after enter = %+v, %v; want idle", a, err)
	}
}

func TestEnter_MyPROpensAndAcks(t *testing.T) {
	home := t.TempDir()
	s := State{OtherPRs: []PR{{Number: 205, Repo: "o/mon", URL: "u", Alert: AlertReady, AlertAt: 1}}}
	m, _, _ := newTestModel(t, home, s) // cursor lands on the only PR row
	gh := m.gh.(*fakeGH)
	update(t, m, key("enter"))
	want := [][]string{{"pr", "view", "205", "--repo", "o/mon", "--web"}}
	if !reflect.DeepEqual(gh.args, want) {
		t.Errorf("gh calls = %q, want %q", gh.args, want)
	}
	if _, err := os.Stat(filepath.Join(home, ".zenify", "hub", "acks", "o_mon_205")); err != nil {
		t.Errorf("ack file after enter: %v", err)
	}
}

func TestKeyF_NoFailedURL(t *testing.T) {
	s := fixState(Idle)
	s.Tasks[0].Repos[0].PR.FailedURL = ""
	m, tm, _ := newTestModel(t, t.TempDir(), s)
	m = update(t, m, key("j"))
	m = update(t, m, key("j")) // PR row #812
	m = update(t, m, key("f"))
	if len(tm.calls) != 0 {
		t.Errorf("tmux called: %q", tm.calls)
	}
	if !strings.Contains(m.View(), "no failed job URL for this PR") {
		t.Errorf("View missing notice:\n%s", m.View())
	}
}

func TestQuit_KeepsLockWhileRoundInFlight(t *testing.T) {
	home := t.TempDir()
	a, _, _ := newTestModel(t, home, State{})
	a, _ = send(t, a, tickMsg(t0.Add(10*time.Second))) // local round started, not finished
	send(t, a, key("q"))
	b, _, cb := newTestModel(t, home, State{})
	if b.lock != nil || cb.local != 0 {
		t.Errorf("lock released while a round was in flight: b holds=%v local=%d", b.lock != nil, cb.local)
	}
}

func TestLockFailover(t *testing.T) {
	home := t.TempDir()
	a, _, _ := newTestModel(t, home, State{})
	b, _, cb := newTestModel(t, home, State{})
	if b.lock != nil || cb.local != 0 {
		t.Fatalf("b polls while a holds the lock")
	}
	update(t, a, key("q")) // a is idle: it releases the lock
	b = update(t, b, tickMsg(t0.Add(time.Second)))
	if b.lock == nil || cb.local != 1 {
		t.Errorf("after a quit: b holds=%v local=%d, want the lock and one local round", b.lock != nil, cb.local)
	}
}

func TestKeyF_FreshBlockedAsks(t *testing.T) {
	home := t.TempDir()
	m, tm, _ := newTestModel(t, home, fixState(Working))
	// The agent became blocked after state.json was written.
	if err := writeAgent(home, &Agent{SessionID: "s1", Pane: "%3", State: Blocked, Slug: "t1"}); err != nil {
		t.Fatal(err)
	}
	m = update(t, m, key("j"))
	m = update(t, m, key("j")) // PR row #812
	m = update(t, m, key("f"))
	if len(tm.calls) != 0 {
		t.Fatalf("sent to a blocked agent without asking: %q", tm.calls)
	}
	if !strings.Contains(m.View(), "Agent is waiting for input — send anyway? y/n") {
		t.Errorf("View missing prompt:\n%s", m.View())
	}
}

// failTicker fails every local round with err.
type failTicker struct{ err error }

func (f *failTicker) Local() error  { return f.err }
func (f *failTicker) GitHub() error { return nil }

func TestPollError_SurvivesReload(t *testing.T) {
	m, _, ct := newTestModel(t, t.TempDir(), fixState(Idle))
	m.poll = &failTicker{err: errors.New("write state: disk full")}
	m = update(t, m, tickMsg(t0.Add(localEvery)))
	m = update(t, m, tickMsg(t0.Add(localEvery+time.Second))) // state.json mtime unchanged
	if !strings.Contains(m.View(), "write state: disk full") {
		t.Fatalf("View lost the poll error after a reload:\n%s", m.View())
	}
	m.poll = ct
	m = update(t, m, tickMsg(t0.Add(2*localEvery)))
	if strings.Contains(m.View(), "write state: disk full") {
		t.Errorf("View keeps the poll error after a successful local round:\n%s", m.View())
	}
}

// cursorState is task t1 in api (PR #1 failing, agent a1 on %5) and task t2
// in web (PR #2). With more, it adds t0 (web, blocked) and t00 (api,
// working), which sort web above api and t00 above t1.
func cursorState(more bool) State {
	s := State{
		Tasks: []Task{
			{Slug: "t1", Agents: []string{"a1"}, State: Idle, Repos: []TaskRepo{{Name: "api", Repo: "o/api", PR: &PR{Number: 1, Repo: "o/api", Checks: "FAILURE", FailedURL: "u1"}}}},
			{Slug: "t2", Repos: []TaskRepo{{Name: "web", Repo: "o/web", PR: &PR{Number: 2, Repo: "o/web"}}}},
		},
		Agents: map[string]Agent{"a1": {SessionID: "a1", Pane: "%5", State: Idle, Slug: "t1"}},
	}
	if more {
		s.Tasks = append(s.Tasks,
			Task{Slug: "t0", Agents: []string{"a0"}, State: Blocked, Repos: []TaskRepo{{Name: "web", Repo: "o/web"}}},
			Task{Slug: "t00", Agents: []string{"a2"}, State: Working, Repos: []TaskRepo{{Name: "api", Repo: "o/api"}}},
		)
		s.Agents["a0"] = Agent{SessionID: "a0", Pane: "%6", State: Blocked, Slug: "t0"}
		s.Agents["a2"] = Agent{SessionID: "a2", Pane: "%7", State: Working, Slug: "t00"}
	}
	return s
}

func TestCursor_FollowsRowAcrossReorder(t *testing.T) {
	home := t.TempDir()
	m, tm, _ := newTestModel(t, home, liveAgents(t, home, cursorState(false)))
	m = update(t, m, key("j"))
	m = update(t, m, key("j")) // api: t1, #1
	if it := m.current(); it.Kind != KindTaskPR || it.PR.Number != 1 {
		t.Fatalf("setup: cursor on %+v", it)
	}

	rewriteState(t, home, cursorState(true), time.Now().Add(time.Hour))
	m = update(t, m, tickMsg(t0.Add(time.Second)))
	if it := m.current(); it.Kind != KindTaskPR || it.PR.Number != 1 || it.Repo.Name != "api" {
		t.Fatalf("after reorder the cursor left PR #1: %+v\n%s", it, m.View())
	}
	if got := taskBlock(m.View())[1]; got != " ▾ web 2" {
		t.Fatalf("reload did not reorder repos, first group %q", got)
	}

	m = update(t, m, key("k")) // worktree api/t1
	rewriteState(t, home, cursorState(false), time.Now().Add(2*time.Hour))
	m = update(t, m, tickMsg(t0.Add(2*time.Second)))
	if it := m.current(); it.Kind != KindTask || it.Repo.Name != "api" || it.Task.Slug != "t1" {
		t.Fatalf("after reorder the cursor left api/t1: %+v\n%s", it, m.View())
	}

	gh := m.gh.(*fakeGH)
	m = update(t, m, key("o"))
	if want := [][]string{{"pr", "view", "1", "--repo", "o/api", "--web"}}; !reflect.DeepEqual(gh.args, want) {
		t.Errorf("o: gh calls = %q, want %q", gh.args, want)
	}
	update(t, m, key("f"))
	want := [][]string{{"send-keys", "-t", "%5", "-l", "/znf:fix u1"}, {"send-keys", "-t", "%5", "Enter"}}
	if !reflect.DeepEqual(tm.calls, want) {
		t.Errorf("f: tmux calls = %q, want %q", tm.calls, want)
	}
}

func TestCursor_FirstLoadStaysOnFirstRow(t *testing.T) {
	home := t.TempDir()
	m, _, _ := newTestModel(t, home, State{})
	s := State{Tasks: []Task{
		{Slug: "x", Repos: []TaskRepo{{Name: "a-repo"}}},
		{Slug: "y", Repos: []TaskRepo{{Name: "b-repo"}}},
	}}
	rewriteState(t, home, s, time.Now().Add(time.Hour))
	m = update(t, m, tickMsg(t0.Add(time.Second)))
	if it := m.current(); it.Kind != KindRepoGroup || it.Title != "a-repo" {
		t.Fatalf("cursor on %+v, want a-repo", it)
	}
	s.Tasks = append(s.Tasks, Task{Slug: "z", Repos: []TaskRepo{{Name: "z-repo"}}, Agents: []string{"a"}, State: Working})
	s.Agents = map[string]Agent{"a": {SessionID: "a", State: Working, Slug: "z"}}
	rewriteState(t, home, s, time.Now().Add(2*time.Hour))
	m = update(t, m, tickMsg(t0.Add(2*time.Second)))
	if it := m.current(); it.Kind != KindRepoGroup || it.Title != "z-repo" {
		t.Errorf("cursor on %+v, want the first row z-repo\n%s", it, m.View())
	}
}

// hubLog is the content of home's hub.log ("" when missing).
func hubLog(t *testing.T, home string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(Dir(home), "hub.log"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(b)
}

func TestTickError_LoggedOnceAndHinted(t *testing.T) {
	home := t.TempDir()
	m, _, _ := newTestModel(t, home, fixState(Idle))
	m = update(t, m, tea.WindowSizeMsg{Width: 200, Height: 40})
	// state.json replaced by a directory: every tick fails to read it.
	if err := os.Remove(statePath(home)); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(statePath(home), "x"), 0o700); err != nil {
		t.Fatal(err)
	}
	m = update(t, m, tickMsg(t0.Add(time.Second)))
	m = update(t, m, tickMsg(t0.Add(2*time.Second)))
	if m.err == nil {
		t.Fatal("setup: tick produced no error")
	}
	if n := strings.Count(hubLog(t, home), m.err.Error()); n != 1 {
		t.Errorf("tick error logged %d times, want 1:\n%s", n, hubLog(t, home))
	}
	m.localErr = errors.New("a\nb") // a second source hides the tick error behind the hint
	if !strings.Contains(m.View(), "more, see ~/.zenify/hub/hub.log)") {
		t.Errorf("footer missing hub.log hint:\n%s", m.View())
	}
}

func TestKeyF_SendErrorLogged(t *testing.T) {
	home := t.TempDir()
	if err := writeAgent(home, &Agent{SessionID: "s1", Pane: "%3", State: Idle, Slug: "t1"}); err != nil {
		t.Fatal(err)
	}
	m, tm, _ := newTestModel(t, home, fixState(Idle))
	tm.fail = map[string]error{"send-keys": errors.New("can't find pane: %3")}
	m = update(t, m, key("j"))
	m = update(t, m, key("j")) // PR row #812
	update(t, m, key("f"))
	if !strings.Contains(hubLog(t, home), "can't find pane: %3") {
		t.Errorf("hub.log missing the send error:\n%s", hubLog(t, home))
	}
}

func TestKeyF_EndedSessionRefuses(t *testing.T) {
	m, tm, _ := newTestModel(t, t.TempDir(), fixState(Idle)) // no agent file: the session ended
	m = update(t, m, key("j"))
	m = update(t, m, key("j")) // PR row #812
	m = update(t, m, key("f"))
	if len(tm.calls) != 0 {
		t.Errorf("sent to an ended session: %q", tm.calls)
	}
	if !strings.Contains(m.View(), "agent session ended") {
		t.Errorf("View missing notice:\n%s", m.View())
	}
}

func TestKeyF_ConfirmAfterSessionEndedRefuses(t *testing.T) {
	home := t.TempDir()
	m, tm, _ := newTestModel(t, home, liveAgents(t, home, fixState(Blocked)))
	m = update(t, m, key("j"))
	m = update(t, m, key("j")) // PR row #812
	m = update(t, m, key("f"))
	if err := os.Remove(agentPath(home, "s1")); err != nil { // SessionEnd while the prompt is up
		t.Fatal(err)
	}
	m = update(t, m, key("y"))
	if len(tm.calls) != 0 {
		t.Errorf("sent to an ended session: %q", tm.calls)
	}
	if !strings.Contains(m.View(), "agent session ended") {
		t.Errorf("View missing notice:\n%s", m.View())
	}
}

func TestKeyF_UsesFreshPane(t *testing.T) {
	home := t.TempDir()
	if err := writeAgent(home, &Agent{SessionID: "s1", Pane: "%9", State: Idle, Slug: "t1"}); err != nil {
		t.Fatal(err)
	}
	m, tm, _ := newTestModel(t, home, fixState(Idle)) // cached pane %3
	m = update(t, m, key("j"))
	m = update(t, m, key("j")) // PR row #812
	update(t, m, key("f"))
	if len(tm.calls) == 0 || tm.calls[0][2] != "%9" {
		t.Errorf("tmux calls = %q, want send-keys to the fresh pane %%9", tm.calls)
	}
}
