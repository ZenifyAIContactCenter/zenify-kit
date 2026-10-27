package hub

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// TestMain renders without colour so View tests do not depend on the
// terminal running them.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

// withProfile renders with p for the rest of the test.
func withProfile(t *testing.T, p termenv.Profile) {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(p)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
}

func TestView_SelectedRowFullBackground(t *testing.T) {
	withProfile(t, termenv.ANSI256)
	m, _, _ := newTestModel(t, t.TempDir(), fixState(Working))
	m = update(t, m, key("j")) // worktree row t1
	var row string
	for _, l := range strings.Split(m.View(), "\n") {
		if strings.Contains(l, "t1") {
			row = l
			break
		}
	}
	bg, reset, slug := strings.Index(row, "48;5;237"), strings.Index(row, "\x1b[0m"), strings.Index(row, "t1")
	if bg < 0 || reset < slug {
		t.Errorf("selected row background must span the row past the icon: %q", row)
	}
}

// inZone runs the test with time.Local set to UTC+7.
func inZone(t *testing.T) {
	t.Helper()
	prev := time.Local
	time.Local = time.FixedZone("ICT", 7*3600)
	t.Cleanup(func() { time.Local = prev })
}

func TestView_HeaderSectionsFooter(t *testing.T) {
	inZone(t)
	s := State{
		Tasks:    []Task{{Slug: "zenify-hub", Repos: []TaskRepo{{Name: "kit"}}}},
		Agents:   map[string]Agent{"s9": {SessionID: "s9", Title: "Daily work"}},
		OtherPRs: []PR{{Number: 205, Repo: "o/tools", Checks: "PENDING", MergeState: "BLOCKED"}},
		LocalAt:  1_700_000_000,
	}
	m, _, _ := newTestModel(t, t.TempDir(), s)
	m = update(t, m, tea.WindowSizeMsg{Width: 40})
	rule := strings.Repeat("─", 40)
	want := strings.Join([]string{
		" zenify hub                     05:13:20",
		rule,
		" TASKS 1",
		"▌▸ kit 1",
		rule,
		" AGENTS 0/1",
		" ⏸ ○ Daily work",
		rule,
		" MY PRS 1",
		" tools #205 ◐ BLOCKED",
		rule,
		" ⏎ jump  ␣ fold  o open  y copy  m merge",
	}, "\n")
	if got := m.View(); got != want {
		t.Errorf("View:\n%s\nwant:\n%s", got, want)
	}
}

// taskBlock is the TASKS section of v: from the TASKS header up to the next rule.
func taskBlock(v string) []string {
	var out []string
	in := false
	for _, l := range strings.Split(v, "\n") {
		if strings.HasPrefix(l, " TASKS") {
			in = true
		} else if in && strings.HasPrefix(l, "─") {
			break
		}
		if in {
			out = append(out, l)
		}
	}
	return out
}

func orderState() State {
	return State{
		Tasks: []Task{
			{Slug: "a-idle", Repos: []TaskRepo{{Name: "be"}}},
			{Slug: "b-pr", Repos: []TaskRepo{{Name: "be", PR: &PR{Number: 7, Repo: "o/be"}}}},
			{Slug: "c-work", Repos: []TaskRepo{{Name: "be"}}, Agents: []string{"w"}, State: Working},
			{Slug: "d-block", Repos: []TaskRepo{{Name: "web"}}, Agents: []string{"k"}, State: Blocked},
			{Slug: "e-old", Repos: []TaskRepo{{Name: "api"}}},
			{Slug: "m1", Repos: []TaskRepo{{Name: "be", Merged: true}, {Name: "web", Merged: true}}},
			{Slug: "m2", Repos: []TaskRepo{{Name: "kit", Merged: true}}},
			{Slug: "z-half", Repos: []TaskRepo{{Name: "be", Merged: true}, {Name: "web"}}},
		},
		Agents: map[string]Agent{
			"w": {SessionID: "w", Title: "agent#1", State: Working, Slug: "c-work"},
			"k": {SessionID: "k", Title: "agent#2", State: Blocked, Slug: "d-block"},
		},
	}
}

func TestView_RepoOrderAndMergedGroup(t *testing.T) {
	m, _, _ := newTestModel(t, t.TempDir(), orderState())
	want := []string{
		" TASKS 7",
		"▌▾ web 2",
		"   ◆ d-block  ⏸ agent#2",
		"   ○ z-half",
		" ▾ be 4",
		"   ● c-work  ⏸ agent#1",
		"   ○ b-pr",
		"       #7",
		"   ○ a-idle",
		"   ○ z-half merged",
		" ▸ api 1",
		" ▸ merged (3)",
	}
	if got := taskBlock(m.View()); !reflect.DeepEqual(got, want) {
		t.Fatalf("TASKS block:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	for i := 0; i < 10; i++ {
		m = update(t, m, key("j"))
	}
	m = update(t, m, key("enter"))
	want = append(want[:11],
		"▌▾ merged (3)",
		"   ○ be/m1",
		"   ○ kit/m2",
		"   ○ web/m1",
	)
	want[1] = " ▾ web 2"
	if got := taskBlock(m.View()); !reflect.DeepEqual(got, want) {
		t.Fatalf("expanded TASKS block:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	m = update(t, m, key(" "))
	if got := taskBlock(m.View()); got[len(got)-1] != "▌▸ merged (3)" {
		t.Errorf("space did not collapse:\n%s", strings.Join(got, "\n"))
	}
}

// sc21State is task t in repos be (PR #1) and web (PR #2), both with an
// alert, worked on by agent A on pane %4.
func sc21State() State {
	return State{
		Tasks: []Task{{Slug: "t", State: Working, Agents: []string{"a"}, Repos: []TaskRepo{
			{Name: "be", PR: &PR{Number: 1, Repo: "o/be", URL: "https://github.com/o/be/pull/1", Alert: AlertReady, AlertAt: 1}},
			{Name: "web", PR: &PR{Number: 2, Repo: "o/web", URL: "https://github.com/o/web/pull/2", Alert: AlertReady, AlertAt: 1}},
		}}},
		Agents: map[string]Agent{"a": {SessionID: "a", Title: "A", Pane: "%4", State: Working, Slug: "t", Open: true, Window: 1}},
	}
}

func TestView_TaskUnderEachRepo(t *testing.T) { // SC-21
	m, _, _ := newTestModel(t, t.TempDir(), sc21State())
	want := []string{
		" TASKS 2",
		"▌▾ be 1",
		"   ● t  ▶ A",
		"       ! #1",
		" ▾ web 1",
		"   ● t  ▶ A",
		"       ! #2",
	}
	if got := taskBlock(m.View()); !reflect.DeepEqual(got, want) {
		t.Errorf("TASKS block:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestKeyO_WorktreePRRowOpensThatRepoPR(t *testing.T) {
	m, _, _ := newTestModel(t, t.TempDir(), sc21State())
	gh := m.gh.(*fakeGH)
	for i := 0; i < 5; i++ { // be, t, #1, web, t, #2
		m = update(t, m, key("j"))
	}
	update(t, m, key("o"))
	want := [][]string{{"pr", "view", "2", "--repo", "o/web", "--web"}}
	if !reflect.DeepEqual(gh.args, want) {
		t.Errorf("gh calls = %q, want %q", gh.args, want)
	}
}

func TestEnter_WorktreeRowJumpsAndAcksThatRepoPR(t *testing.T) {
	home := t.TempDir()
	m, tm, _ := newTestModel(t, home, sc21State())
	for i := 0; i < 4; i++ { // worktree t under web
		m = update(t, m, key("j"))
	}
	update(t, m, key("enter"))
	if len(tm.calls) == 0 || !reflect.DeepEqual(tm.calls[0], []string{"switch-client", "-t", "%4"}) {
		t.Errorf("tmux calls = %q, want a jump to %%4", tm.calls)
	}
	acks := filepath.Join(home, ".zenify", "hub", "acks")
	if _, err := os.Stat(filepath.Join(acks, "o_web_2")); err != nil {
		t.Errorf("web PR not acked: %v", err)
	}
	if _, err := os.Stat(filepath.Join(acks, "o_be_1")); !os.IsNotExist(err) {
		t.Errorf("be PR acked (err=%v), want only the web PR", err)
	}
}

// rewriteState writes s as state.json with a newer mtime, so the next tick
// reloads it.
func rewriteState(t *testing.T, home string, s State, at time.Time) {
	t.Helper()
	if err := WriteState(home, s); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(statePath(home), at, at); err != nil {
		t.Fatal(err)
	}
}

func TestFold_IdleRepoCollapsedUserChoiceSurvivesReload(t *testing.T) { // SC-22
	home := t.TempDir()
	s := State{Tasks: []Task{{Slug: "w", Repos: []TaskRepo{{Name: "x"}}}}}
	m, _, _ := newTestModel(t, home, s)
	if got, want := taskBlock(m.View()), []string{" TASKS 1", "▌▸ x 1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("TASKS block:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	m = update(t, m, key("enter"))
	if got, want := taskBlock(m.View()), []string{" TASKS 1", "▌▾ x 1", "   ○ w"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after enter:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	// An active repo now sorts above x: the cursor follows x, which stays open.
	s.Tasks = append(s.Tasks, Task{Slug: "v", Repos: []TaskRepo{{Name: "be"}}, Agents: []string{"a"}, State: Working})
	s.Agents = map[string]Agent{"a": {SessionID: "a", Title: "A", State: Working, Slug: "v"}}
	rewriteState(t, home, s, time.Now().Add(time.Hour))
	m = update(t, m, tickMsg(t0.Add(time.Second)))
	want := []string{" TASKS 2", " ▾ be 1", "   ● v  ⏸ A", "▌▾ x 1", "   ○ w"}
	if got := taskBlock(m.View()); !reflect.DeepEqual(got, want) {
		t.Fatalf("after reload:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	m = update(t, m, key(" "))
	want = []string{" TASKS 2", " ▾ be 1", "   ● v  ⏸ A", "▌▸ x 1"}
	if got := taskBlock(m.View()); !reflect.DeepEqual(got, want) {
		t.Errorf("after space:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestKeyHint_Fold(t *testing.T) {
	if !strings.Contains(keyHint, "␣ fold") {
		t.Errorf("keyHint %q missing %q", keyHint, "␣ fold")
	}
}

func TestView_MultiAgentTaskListsAgents(t *testing.T) {
	s := State{
		Tasks: []Task{{Slug: "t1", Repos: []TaskRepo{{Name: "be"}}, Agents: []string{"a", "b"}, State: Working}},
		Agents: map[string]Agent{
			"a": {SessionID: "a", Title: "one", State: Working, Slug: "t1"},
			"b": {SessionID: "b", Title: "two", State: Idle, Slug: "t1"},
		},
	}
	m, _, _ := newTestModel(t, t.TempDir(), s)
	want := []string{" TASKS 1", "▌▾ be 1", "   ● t1", "       ⏸ ● one", "       ⏸ ○ two"}
	if got := taskBlock(m.View()); !reflect.DeepEqual(got, want) {
		t.Errorf("TASKS block:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestView_ScrollsToCursor(t *testing.T) { // SC-17
	var s State
	for i := 1; i <= 25; i++ {
		s.Tasks = append(s.Tasks, Task{Slug: fmt.Sprintf("task-%02d", i), Repos: []TaskRepo{{Name: "be"}}})
	}
	m, _, _ := newTestModel(t, t.TempDir(), s)
	m = update(t, m, tea.WindowSizeMsg{Width: 30, Height: 20})
	m = update(t, m, key("enter")) // open the idle repo be
	for i := 0; i < 25; i++ {
		m = update(t, m, key("j"))
	}
	v := m.View()
	if !strings.Contains(v, "\n▌  ○ task-25\n") {
		t.Errorf("View missing highlighted last task:\n%s", v)
	}
	if strings.Contains(v, "task-09") {
		t.Errorf("View did not scroll:\n%s", v)
	}
	if n := strings.Count(v, "\n") + 1; n != 20 {
		t.Errorf("View has %d lines, want 20:\n%s", n, v)
	}
}

func TestView_MergedTaskWithAgentStaysActive(t *testing.T) {
	s := State{
		Tasks: []Task{
			{Slug: "m1", Repos: []TaskRepo{{Name: "be", Merged: true}}},
			{Slug: "m2", Repos: []TaskRepo{{Name: "kit", Merged: true}}, Agents: []string{"k"}, State: Blocked},
		},
		Agents: map[string]Agent{"k": {SessionID: "k", Title: "agent#2", State: Blocked, Slug: "m2"}},
	}
	m, _, _ := newTestModel(t, t.TempDir(), s)
	want := []string{" TASKS 1", "▌▾ kit 1", "   ◆ m2  ⏸ agent#2 merged", " ▸ merged (1)"}
	if got := taskBlock(m.View()); !reflect.DeepEqual(got, want) {
		t.Errorf("TASKS block:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestView_MergedTaskWithOpenPRStaysActive(t *testing.T) {
	s := State{
		Tasks: []Task{
			{Slug: "m1", Repos: []TaskRepo{{Name: "be", Merged: true}}},
			{Slug: "m2", Repos: []TaskRepo{{Name: "kit", Merged: true, PR: &PR{Number: 9, Repo: "o/kit"}}}},
		},
	}
	m, _, _ := newTestModel(t, t.TempDir(), s)
	want := []string{" TASKS 1", "▌▾ kit 1", "   ○ m2 merged", "       #9", " ▸ merged (1)"}
	if got := taskBlock(m.View()); !reflect.DeepEqual(got, want) {
		t.Errorf("TASKS block:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestView_MultiLineErrorKeepsCursorVisible(t *testing.T) {
	var s State
	for i := 1; i <= 25; i++ {
		s.Tasks = append(s.Tasks, Task{Slug: fmt.Sprintf("task-%02d", i), Repos: []TaskRepo{{Name: "be"}}})
	}
	m, _, _ := newTestModel(t, t.TempDir(), s)
	m = update(t, m, tea.WindowSizeMsg{Width: 60, Height: 20})
	m = update(t, m, key("enter")) // open the idle repo be
	for i := 0; i < 25; i++ {
		m = update(t, m, key("j"))
	}
	var lines []string
	for i := 1; i <= 20; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	m.poll = &failTicker{err: errors.New(strings.Join(lines, "\n"))}
	m = update(t, m, tickMsg(t0.Add(localEvery)))
	v := m.View()
	if !strings.Contains(v, "\n▌  ○ task-25") {
		t.Errorf("selected row pushed off-screen:\n%s", v)
	}
	if !strings.Contains(v, "line 1 (+19, hub.log)") || strings.Contains(v, "line 2") {
		t.Errorf("error not shown as one line:\n%s", v)
	}
	if n := strings.Count(v, "\n") + 1; n != 20 {
		t.Errorf("View has %d lines, want 20:\n%s", n, v)
	}
}

// agentsBlock is the AGENTS section of v: from its header up to the next rule.
func agentsBlock(v string) []string {
	var out []string
	in := false
	for _, l := range strings.Split(v, "\n") {
		if strings.HasPrefix(l, " AGENTS") {
			in = true
		} else if in && strings.HasPrefix(l, "─") {
			break
		}
		if in {
			out = append(out, l)
		}
	}
	return out
}

// livenessState has agent A on an old tmux server (closed) and agent B whose
// pane is live in window 2 (open), both on task t1, plus closed agents C (no
// slug) and D (no title), with B and D also open-sorting candidates.
func livenessState() State {
	return State{
		Tasks: []Task{{Slug: "t1", Repos: []TaskRepo{{Name: "be"}}, Agents: []string{"b"}, State: Working}},
		Agents: map[string]Agent{
			"a": {SessionID: "a", Title: "old run", Slug: "gone", State: Idle, UpdatedAt: 50},
			"b": {SessionID: "b", Title: "new run", Slug: "t1", State: Working, UpdatedAt: 10, Open: true, Window: 2},
			"c": {SessionID: "c", Title: "scratch", State: Done, UpdatedAt: 70},
			"d": {SessionID: "d", Slug: "t9", State: Blocked, UpdatedAt: 5, Open: true, Window: 4},
		},
	}
}

func TestView_AgentsLiveness(t *testing.T) { // SC-23 (view side)
	m, _, _ := newTestModel(t, t.TempDir(), livenessState())
	v := m.View()
	want := []string{
		" AGENTS 2/4",
		" ▶ ◆ t9 · t9 w4",
		" ▶ ● new run · t1 w2",
		" ⏸ ✓ scratch",
		" ⏸ ○ old run · gone",
	}
	if got := agentsBlock(v); !reflect.DeepEqual(got, want) {
		t.Errorf("AGENTS block:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if !strings.Contains(v, "   ● t1  ▶ new run\n") {
		t.Errorf("worktree row missing liveness:\n%s", v)
	}
}

// toAgent moves the cursor down until it rests on the agent row of id.
func toAgent(t *testing.T, m Model, id string) Model {
	t.Helper()
	for range len(m.items) {
		if it := m.current(); it.Kind == KindAgent && it.Agent.SessionID == id {
			return m
		}
		m = update(t, m, key("j"))
	}
	t.Fatalf("no agent row %s", id)
	return m
}

func TestEnter_ClosedAgentResumes(t *testing.T) {
	m, tm, _ := newTestModel(t, t.TempDir(), livenessState())
	tm.out = map[string]string{"new-window": "@7\n"}
	tm.fail = map[string]error{"has-session": errors.New("can't find session: zenify")}
	a := uuidAgent()
	if err := writeAgent(m.home, &a); err != nil {
		t.Fatal(err)
	}
	m.state.Agents[a.SessionID] = a
	m.relayout()
	m = toAgent(t, m, a.SessionID)
	m = update(t, m, key("enter"))
	want := [][]string{
		{"display-message", "-p", "#{pid}"},
		{"list-panes", "-a", "-F", "#{pane_id} #{window_index}"},
		{"has-session", "-t", "=zenify"},
		{"new-window", "-P", "-F", "#{window_id}", "-c", "/ws", "-n", "be-2376-tests", "claude", "--resume", a.SessionID},
		{"split-window", "-d", "-h", "-b", "-l", "30%", "-t", "@7", "-c", "/ws", "/bin/zenify", "hub", "sidebar"},
		{"set-option", "-w", "-t", "@7", "allow-passthrough", "on"},
	}
	if !reflect.DeepEqual(tm.calls, want) {
		t.Errorf("calls = %q\nwant %q", tm.calls, want)
	}
	if m.failed {
		t.Errorf("notice error: %s", m.notice)
	}
	if _, err := os.Stat(resumeLockPath(m.home, a.SessionID)); err != nil {
		t.Errorf("resume lock not taken: %v", err)
	}
}

// uuidAgent is a closed agent with a real-looking session id and a slug that
// needs no window-name escaping.
func uuidAgent() Agent {
	return Agent{SessionID: "0b5e7c1a-9f3d-4c2e-8a6b-1d2e3f4a5b6c", Slug: "be-2376-tests", Title: "tests", UpdatedAt: 1}
}

func TestEnter_OpenAgentJumps(t *testing.T) {
	m, tm, _ := newTestModel(t, t.TempDir(), livenessState())
	m = toAgent(t, m, "b")
	update(t, m, key("enter"))
	if len(tm.calls) == 0 || tm.calls[0][0] != "switch-client" {
		t.Errorf("calls = %q, want a jump", tm.calls)
	}
}

func TestKeyX_ClosedAgentConfirmThenForget(t *testing.T) {
	home := t.TempDir()
	s := liveAgents(t, home, livenessState())
	m, _, _ := newTestModel(t, home, s)
	m = toAgent(t, m, "a")
	m = update(t, m, key("x"))
	if !strings.Contains(m.View(), "y/n · forget old run?") {
		t.Fatalf("View missing prompt:\n%s", m.View())
	}
	if a, _ := readAgent(home, "a"); a == nil {
		t.Fatal("deleted before confirm")
	}
	m = update(t, m, key("y"))
	if a, err := readAgent(home, "a"); a != nil || err != nil {
		t.Errorf("agent file kept: %+v %v", a, err)
	}
	if m.failed || m.notice != "forgot agent old run" {
		t.Errorf("notice %q failed=%v", m.notice, m.failed)
	}
}

func TestKeyX_ForgetErrorShown(t *testing.T) {
	home := t.TempDir()
	m, _, _ := newTestModel(t, home, livenessState())
	// A directory in place of the agent file makes the removal fail.
	if err := os.MkdirAll(filepath.Join(AgentsDir(home), "a.json", "x"), 0o700); err != nil {
		t.Fatal(err)
	}
	m = toAgent(t, m, "a")
	m = update(t, m, key("x"))
	m = update(t, m, key("y"))
	if !m.failed || !strings.Contains(m.notice, "read agent a") {
		t.Errorf("notice %q failed=%v", m.notice, m.failed)
	}
}

func TestKeyX_OpenAgentRefuses(t *testing.T) {
	home := t.TempDir()
	m, _, _ := newTestModel(t, home, liveAgents(t, home, livenessState()))
	m = toAgent(t, m, "b")
	m = update(t, m, key("x"))
	if m.prompt != "" || m.notice != "agent is running" {
		t.Errorf("prompt %q notice %q", m.prompt, m.notice)
	}
	if a, _ := readAgent(home, "b"); a == nil {
		t.Error("open agent deleted")
	}
}

func TestEnter_TaskRowJumpsToOpenAgent(t *testing.T) {
	s := State{
		Tasks: []Task{{Slug: "t1", Repos: []TaskRepo{{Name: "be"}}, Agents: []string{"a", "b"}}},
		Agents: map[string]Agent{
			"a": {SessionID: "a", Pane: "%1", State: Blocked, Slug: "t1", UpdatedAt: 9},
			"b": {SessionID: "b", Pane: "%2", State: Idle, Slug: "t1", UpdatedAt: 1, Open: true},
		},
	}
	m, tm, _ := newTestModel(t, t.TempDir(), s)
	m = update(t, m, key("j")) // worktree row t1
	update(t, m, key("enter"))
	if len(tm.calls) == 0 || !reflect.DeepEqual(tm.calls[0], []string{"switch-client", "-t", "%2"}) {
		t.Errorf("calls = %q, want a jump to %%2", tm.calls)
	}
}

func TestEnter_DoubleResumeGuarded(t *testing.T) {
	home := t.TempDir()
	m, tm, _ := newTestModel(t, home, liveAgents(t, home, livenessState()))
	tm.out = map[string]string{"new-window": "@7\n"}
	m = toAgent(t, m, "a")
	m = update(t, m, key("enter"))
	n := len(tm.calls)
	m = update(t, m, key("enter"))
	if len(tm.calls) != n || m.notice != "already resuming" {
		t.Errorf("second enter: calls %q notice %q", tm.calls[n:], m.notice)
	}
}

func TestResuming_ClearedWhenOpen(t *testing.T) {
	home := t.TempDir()
	s := liveAgents(t, home, livenessState())
	m, tm, _ := newTestModel(t, home, s)
	tm.out = map[string]string{"new-window": "@7\n"}
	m = toAgent(t, m, "a")
	m = update(t, m, key("enter"))
	a := s.Agents["a"]
	a.Open, a.Window = true, 5
	s.Agents["a"] = a
	rewriteState(t, home, s, time.Now().Add(time.Hour))
	m = update(t, m, tickMsg(t0.Add(time.Second)))
	if _, ok := m.resuming["a"]; ok {
		t.Error("resuming kept after the agent opened")
	}
	if _, err := os.Stat(resumeLockPath(home, "a")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("resume lock kept after the agent opened: %v", err)
	}
}

// writeResumeLock leaves a resume lock for id taken at, as another sidebar would.
func writeResumeLock(t *testing.T, home, id string, at time.Time) {
	t.Helper()
	if err := os.WriteFile(resumeLockPath(home, id), []byte(strconv.FormatInt(at.Unix(), 10)), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestEnter_FreshResumeLockRefuses(t *testing.T) {
	home := t.TempDir()
	m, tm, _ := newTestModel(t, home, liveAgents(t, home, livenessState()))
	writeResumeLock(t, home, "a", time.Now().Add(-5*time.Second))
	m = toAgent(t, m, "a")
	m = update(t, m, key("enter"))
	if len(tm.calls) != 0 || m.notice != "already resuming in another sidebar" || m.failed {
		t.Errorf("calls %q notice %q failed=%v", tm.calls, m.notice, m.failed)
	}
}

func TestEnter_StaleResumeLockProceeds(t *testing.T) {
	home := t.TempDir()
	m, tm, _ := newTestModel(t, home, liveAgents(t, home, livenessState()))
	tm.out = map[string]string{"new-window": "@7\n"}
	writeResumeLock(t, home, "a", time.Now().Add(-time.Minute))
	m = toAgent(t, m, "a")
	m = update(t, m, key("enter"))
	if m.failed || m.notice != "resumed old run in a new window" {
		t.Errorf("notice %q failed=%v", m.notice, m.failed)
	}
	b, err := os.ReadFile(resumeLockPath(home, "a"))
	if err != nil {
		t.Fatal(err)
	}
	if sec, _ := strconv.ParseInt(string(b), 10, 64); time.Since(time.Unix(sec, 0)) > 5*time.Second {
		t.Errorf("stale lock not replaced: %s", b)
	}
}

func TestEnter_OpenOnFreshCheckRefuses(t *testing.T) {
	home := t.TempDir()
	s := livenessState()
	a := s.Agents["a"] // closed in the cached state, open on the server now
	a.Pane, a.TmuxServer = "%9", "4242"
	s.Agents["a"] = a
	m, tm, _ := newTestModel(t, home, liveAgents(t, home, s))
	tm.out = map[string]string{"display-message": "4242\n", "list-panes": "%9 3\n"}
	m = toAgent(t, m, "a")
	m = update(t, m, key("enter"))
	for _, c := range tm.calls {
		if c[0] == "new-window" {
			t.Fatalf("resumed an open agent: %q", tm.calls)
		}
	}
	if m.notice != "agent is already open" || m.failed {
		t.Errorf("notice %q failed=%v", m.notice, m.failed)
	}
	if _, err := os.Stat(resumeLockPath(home, "a")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("resume lock kept after refusing: %v", err)
	}
}

func TestLoadAgents_IgnoresResumeLocks(t *testing.T) {
	home := t.TempDir()
	liveAgents(t, home, livenessState())
	writeResumeLock(t, home, "a", time.Now())
	got, err := LoadAgents(home)
	if err != nil || len(got) != 4 {
		t.Errorf("agents %d err %v, want 4 and nil", len(got), err)
	}
}

func TestKeyX_OpenAtConfirmRefuses(t *testing.T) {
	home := t.TempDir()
	s := livenessState()
	a := s.Agents["a"]
	a.Pane, a.TmuxServer = "%9", "4242"
	s.Agents["a"] = a
	m, tm, _ := newTestModel(t, home, liveAgents(t, home, s))
	m = toAgent(t, m, "a")
	m = update(t, m, key("x"))
	tm.out = map[string]string{"display-message": "4242\n", "list-panes": "%9 3\n"} // resumed elsewhere meanwhile
	m = update(t, m, key("y"))
	if m.notice != "agent is open again" || m.failed {
		t.Errorf("notice %q failed=%v", m.notice, m.failed)
	}
	if got, _ := readAgent(home, "a"); got == nil {
		t.Error("open agent deleted")
	}
}

func TestKeyX_ChangedAtConfirmRefuses(t *testing.T) {
	home := t.TempDir()
	s := liveAgents(t, home, livenessState())
	m, _, _ := newTestModel(t, home, s)
	m = toAgent(t, m, "a")
	m = update(t, m, key("x"))
	a := s.Agents["a"]
	a.UpdatedAt++ // a hook reported meanwhile
	if err := writeAgent(home, &a); err != nil {
		t.Fatal(err)
	}
	m = update(t, m, key("y"))
	if m.notice != "agent changed" || m.failed {
		t.Errorf("notice %q failed=%v", m.notice, m.failed)
	}
	if got, _ := readAgent(home, "a"); got == nil {
		t.Error("changed agent deleted")
	}
}

func TestKeyF_ClosedAgentRefuses(t *testing.T) {
	s := fixState(Done)
	a := s.Agents["s1"]
	a.Open = false
	s.Agents["s1"] = a
	m, tm, _ := newTestModel(t, t.TempDir(), s)
	m = update(t, m, key("j"))
	m = update(t, m, key("f"))
	if len(tm.calls) != 0 || m.notice != "agent is closed — press Enter to resume" {
		t.Errorf("calls %q notice %q", tm.calls, m.notice)
	}
}

func TestKeyF_ConfirmAfterAgentClosedRefuses(t *testing.T) {
	home := t.TempDir()
	s := liveAgents(t, home, fixState(Blocked))
	m, tm, _ := newTestModel(t, home, s)
	m = update(t, m, key("j"))
	m = update(t, m, key("f")) // blocked: asks first
	if m.prompt == "" {
		t.Fatal("no prompt")
	}
	a := s.Agents["s1"]
	a.Open = false
	s.Agents["s1"] = a
	rewriteState(t, home, s, time.Now().Add(time.Hour))
	m = update(t, m, tickMsg(t0.Add(time.Second)))
	m = update(t, m, key("y"))
	if len(tm.calls) != 0 || m.notice != "agent is closed — press Enter to resume" {
		t.Errorf("calls %q notice %q", tm.calls, m.notice)
	}
}

func TestKeyHint_Forget(t *testing.T) {
	if !strings.Contains(keyHint, "x forget") {
		t.Errorf("keyHint %q missing x forget", keyHint)
	}
}

func TestTakeResumeLock_EmptyFreshLockRefuses(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(AgentsDir(home), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resumeLockPath(home, "a"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := takeResumeLock(home, "a", time.Now())
	if err != nil || got {
		t.Fatalf("got %v err %v, want refused", got, err)
	}
	if _, err := os.Stat(resumeLockPath(home, "a")); err != nil {
		t.Errorf("fresh empty lock was removed: %v", err)
	}
}

func TestTakeResumeLock_StaleReplaced(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(AgentsDir(home), 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	writeResumeLock(t, home, "a", now.Add(-time.Minute))
	got, err := takeResumeLock(home, "a", now)
	if err != nil || !got {
		t.Fatalf("got %v err %v, want taken", got, err)
	}
	b, err := os.ReadFile(resumeLockPath(home, "a"))
	if err != nil || string(b) != strconv.FormatInt(now.Unix(), 10) {
		t.Errorf("lock content %q err %v", b, err)
	}
	left, _ := filepath.Glob(filepath.Join(AgentsDir(home), ".resume-*.tmp"))
	if len(left) != 0 {
		t.Errorf("temp files left: %v", left)
	}
}

func TestTakeResumeLock_HeldWhenLinkedTwice(t *testing.T) {
	home := t.TempDir()
	now := time.Now()
	if got, err := takeResumeLock(home, "a", now); err != nil || !got {
		t.Fatalf("first: %v %v", got, err)
	}
	if got, err := takeResumeLock(home, "a", now); err != nil || got {
		t.Fatalf("second: got %v err %v, want refused", got, err)
	}
}

func TestRemoveStaleResumeLock_MtimeChangedRefuses(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(AgentsDir(home), 0o700); err != nil {
		t.Fatal(err)
	}
	writeResumeLock(t, home, "a", time.Now().Add(-time.Minute))
	p := resumeLockPath(home, "a")
	_, seen, err := readResumeLock(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, seen.Add(time.Hour), seen.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	ok, err := removeStaleResumeLock(p, seen)
	if err != nil || ok {
		t.Fatalf("got %v err %v, want refused", ok, err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Errorf("lock removed despite changed mtime: %v", err)
	}
}

func TestView_NarrowPromptWrapsWithYN(t *testing.T) {
	m, _, _ := newTestModel(t, t.TempDir(), State{})
	m = update(t, m, tea.WindowSizeMsg{Width: 36, Height: 20})
	m = m.ask("y/n · open new agent for "+strings.Repeat("long-slug-", 8)+"?", func(m Model) Model { return m })
	v := m.View()
	lines := strings.Split(v, "\n")
	at := -1
	for i, l := range lines {
		if lipgloss.Width(l) > 36 {
			t.Errorf("line wider than 36: %q", l)
		}
		if strings.HasPrefix(l, "y/n · open") {
			at = i
		}
	}
	if at < 0 || at+2 >= len(lines) || !strings.HasSuffix(lines[at+1], "…") || strings.Contains(lines[at+2], "long-slug") {
		t.Errorf("prompt not shown as 2 lines with y/n first:\n%s", v)
	}
}

func TestView_NarrowErrorKeepsFirstWords(t *testing.T) {
	m, _, _ := newTestModel(t, t.TempDir(), State{})
	m = update(t, m, tea.WindowSizeMsg{Width: 36, Height: 20})
	m.err = errors.New("dial tcp 127.0.0.1: connection refused\nsecond line")
	if v := m.View(); !strings.Contains(v, "dial tcp") || !strings.Contains(v, "(+1, hub.log)") {
		t.Errorf("error at width 36:\n%s", v)
	}
	m = update(t, m, tea.WindowSizeMsg{Width: 20, Height: 20})
	if v := m.View(); !strings.Contains(v, "dial tcp") || strings.Contains(v, "hub.log") {
		t.Errorf("error at width 20 kept the hint or lost the message:\n%s", v)
	}
}
