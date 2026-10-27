package hub

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// scriptTmux records every tmux call and answers with out[subcommand].
type scriptTmux struct {
	out   map[string]string
	calls [][]string
}

func (f *scriptTmux) Run(args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	return []byte(f.out[args[0]]), nil
}

// countTicker counts the local and GitHub rounds it ran.
type countTicker struct{ local, github int }

func (c *countTicker) Local() error {
	c.local++
	return nil
}

func (c *countTicker) GitHub() error {
	c.github++
	return nil
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
	ct := &countTicker{}
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
	for _, want := range []string{"TASKS 1", "▌◆ t1  s1\n", "     be #812 ✗ !\n", "AGENTS 0", "MY PRS 0"} {
		if !strings.Contains(v, want) {
			t.Errorf("View missing %q:\n%s", want, v)
		}
	}
}

func TestEnter_SelectsPane(t *testing.T) {
	m, tm, _ := newTestModel(t, t.TempDir(), fixState(Done))
	update(t, m, key("enter")) // cursor starts on task t1
	want := [][]string{{"select-window", "-t", "%3"}, {"select-pane", "-t", "%3"}}
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
	update(t, m, key("enter"))
	a, err := readAgent(home, "s1")
	if err != nil || a.State != Idle {
		t.Errorf("agent after enter = %+v, %v; want idle", a, err)
	}
}

func TestEnter_NoAgentAsksThenOpens(t *testing.T) {
	m, tm, _ := newTestModel(t, t.TempDir(), State{Tasks: []Task{{Slug: "t2"}}})
	tm.out = map[string]string{"new-window": "@9\n"}
	m = update(t, m, key("enter"))
	if !strings.Contains(m.View(), "Open a new agent for t2? y/n") {
		t.Fatalf("View missing prompt:\n%s", m.View())
	}
	if len(tm.calls) != 0 {
		t.Fatalf("tmux called before confirm: %q", tm.calls)
	}
	update(t, m, key("y"))
	if len(tm.calls) != 2 || tm.calls[0][0] != "new-window" || tm.calls[1][0] != "split-window" {
		t.Errorf("tmux calls = %q", tm.calls)
	}
}

func TestKeyF_SendsFix(t *testing.T) {
	home := t.TempDir()
	m, tm, _ := newTestModel(t, home, fixState(Idle))
	m = update(t, m, key("j")) // repo row be #812
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
	m, tm, _ := newTestModel(t, t.TempDir(), fixState(Blocked))
	m = update(t, m, key("j"))
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
	update(t, m, key("o"))
	want := [][]string{{"pr", "view", "812", "--repo", "o/r", "--web"}}
	if !reflect.DeepEqual(gh.args, want) {
		t.Errorf("gh calls = %q, want %q", gh.args, want)
	}
}

func TestOnlyLockHolderPolls(t *testing.T) {
	home := t.TempDir()
	a, _, ca := newTestModel(t, home, State{})
	b, _, cb := newTestModel(t, home, State{})
	for i := 1; i <= 6; i++ {
		now := tickMsg(t0.Add(time.Duration(i) * time.Second))
		a = update(t, a, now)
		b = update(t, b, now)
	}
	if cb.local != 0 || cb.github != 0 {
		t.Errorf("non-holder ran rounds: local=%d github=%d", cb.local, cb.github)
	}
	if b.err != nil {
		t.Errorf("non-holder error = %v, want nil (a held lock is not an error)", b.err)
	}
	// local every 2s: t0, t0+2, t0+4, t0+6; GitHub only on the first round.
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

func TestPollOneRoundInFlight(t *testing.T) {
	m, _, ct := newTestModel(t, t.TempDir(), State{})
	m, first := send(t, m, tickMsg(t0.Add(2*time.Second))) // local round due, not finished
	m, second := send(t, m, tickMsg(t0.Add(4*time.Second)))
	m = drain(t, m, second)
	if ct.local != 1 {
		t.Fatalf("local rounds while one is in flight = %d, want 1 (the initial one)", ct.local)
	}
	m = drain(t, m, first)
	update(t, m, tickMsg(t0.Add(6*time.Second)))
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
	for i := 2; i <= 6; i += 2 {
		var cmd tea.Cmd
		m, cmd = send(t, m, tickMsg(t0.Add(time.Duration(i)*time.Second)))
		m = drainHolding(t, m, cmd, &held)
	}
	// initial local + t0+2, t0+4, t0+6; GitHub: initial + the refresh one, still in flight.
	if ct.local != 4 || ct.github != 2 || len(held) != 1 {
		t.Fatalf("local=%d github=%d held=%d, want 4, 2, 1", ct.local, ct.github, len(held))
	}
	m = update(t, m, held[0])
	m = update(t, m, key("r"))
	update(t, m, tickMsg(t0.Add(8*time.Second)))
	if ct.github != 3 {
		t.Errorf("github rounds after the round finished = %d, want 3", ct.github)
	}
}

func TestView_ReviewState(t *testing.T) {
	s := fixState(Idle)
	pr := s.Tasks[0].Repos[0].PR
	pr.Checks, pr.MergeState, pr.ReviewDecision, pr.UnresolvedThreads, pr.Alert = "SUCCESS", "BLOCKED", "CHANGES_REQUESTED", 2, ""
	m, _, _ := newTestModel(t, t.TempDir(), s)
	if v := m.View(); !strings.Contains(v, "be #812 ✓ BLOCKED changes 2 threads\n") {
		t.Errorf("View missing review state:\n%s", v)
	}
}

func TestKeyF_BlockedDecline(t *testing.T) {
	for _, k := range []string{"n", "q"} {
		home := t.TempDir()
		m, tm, _ := newTestModel(t, home, fixState(Blocked))
		m = update(t, m, key("j"))
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
	m = update(t, m, tickMsg(t0.Add(2*time.Second))) // cadence round: caches the repo list
	m = update(t, m, tickMsg(t0.Add(3*time.Second)))
	if ct.local != 2 {
		t.Fatalf("local rounds with no change = %d, want 2", ct.local)
	}
	if err := os.MkdirAll(filepath.Join(root, "repos", "be", ".worktrees", "new-task"), 0o750); err != nil {
		t.Fatal(err)
	}
	m = update(t, m, tickMsg(t0.Add(3500*time.Millisecond))) // cadence not due until t0+4
	if ct.local != 3 {
		t.Fatalf("local rounds after a new worktree dir = %d, want 3", ct.local)
	}
	update(t, m, tickMsg(t0.Add(3700*time.Millisecond)))
	if ct.local != 3 {
		t.Errorf("local rounds with an unchanged fingerprint = %d, want 3", ct.local)
	}
}

func TestNewWorktreeWhileLocalInFlightRerunsAfter(t *testing.T) {
	root := mkWorkspace(t)
	m, _, ct := newTestModel(t, t.TempDir(), State{})
	m.root = root
	m = update(t, m, tickMsg(t0.Add(2*time.Second)))
	m, inFlight := send(t, m, tickMsg(t0.Add(4*time.Second))) // cadence round started, not finished
	if err := os.MkdirAll(filepath.Join(root, "repos", "be", ".worktrees", "new-task"), 0o750); err != nil {
		t.Fatal(err)
	}
	m, cmd := send(t, m, tickMsg(t0.Add(4500*time.Millisecond)))
	m = drain(t, m, cmd)
	if ct.local != 2 {
		t.Fatalf("local rounds while one is in flight = %d, want 2", ct.local)
	}
	drain(t, m, inFlight) // finishing it starts the rerun at once
	if ct.local != 4 {
		t.Errorf("local rounds after the in-flight one finished = %d, want 4", ct.local)
	}
}
