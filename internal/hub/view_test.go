package hub

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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
		Tasks:      []Task{{Slug: "zenify-hub", Repos: []TaskRepo{{Name: "kit"}}}},
		Unattached: []Agent{{SessionID: "s9", Title: "Daily work"}},
		OtherPRs:   []PR{{Number: 205, Repo: "o/monitoring", Checks: "PENDING", MergeState: "BLOCKED"}},
		LocalAt:    1_700_000_000,
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
		" AGENTS 1",
		" ○ Daily work",
		rule,
		" MY PRS 1",
		" monitoring #205 ◐ BLOCKED",
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
		" TASKS 6",
		"▌▾ web 2",
		"   ◆ d-block  agent#2",
		"   ○ z-half",
		" ▾ be 3",
		"   ● c-work  agent#1",
		"   ○ b-pr",
		"       #7",
		"   ○ a-idle",
		" ▸ api 1",
		" ▸ merged (4)",
	}
	if got := taskBlock(m.View()); !reflect.DeepEqual(got, want) {
		t.Fatalf("TASKS block:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	for i := 0; i < 9; i++ {
		m = update(t, m, key("j"))
	}
	m = update(t, m, key("enter"))
	want = append(want[:10],
		"▌▾ merged (4)",
		"   ○ be/m1",
		"   ○ be/z-half",
		"   ○ kit/m2",
		"   ○ web/m1",
	)
	want[1] = " ▾ web 2"
	if got := taskBlock(m.View()); !reflect.DeepEqual(got, want) {
		t.Fatalf("expanded TASKS block:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	m = update(t, m, key(" "))
	if got := taskBlock(m.View()); got[len(got)-1] != "▌▸ merged (4)" {
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
		Agents: map[string]Agent{"a": {SessionID: "a", Title: "A", Pane: "%4", State: Working, Slug: "t"}},
	}
}

func TestView_TaskUnderEachRepo(t *testing.T) { // SC-21
	m, _, _ := newTestModel(t, t.TempDir(), sc21State())
	want := []string{
		" TASKS 2",
		"▌▾ be 1",
		"   ● t  A",
		"       #1 !",
		" ▾ web 1",
		"   ● t  A",
		"       #2 !",
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
	want := []string{" TASKS 2", " ▾ be 1", "   ● v  A", "▌▾ x 1", "   ○ w"}
	if got := taskBlock(m.View()); !reflect.DeepEqual(got, want) {
		t.Fatalf("after reload:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	m = update(t, m, key(" "))
	want = []string{" TASKS 2", " ▾ be 1", "   ● v  A", "▌▸ x 1"}
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
	want := []string{" TASKS 1", "▌▾ be 1", "   ● t1", "       ● one", "       ○ two"}
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
	want := []string{" TASKS 1", "▌▾ kit 1", "   ◆ m2  agent#2 merged", " ▸ merged (1)"}
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
	if !strings.Contains(v, "line 1 (+19 more, see ~/.zenify/hub/hub.log)") || strings.Contains(v, "line 2") {
		t.Errorf("error not shown as one line:\n%s", v)
	}
	if n := strings.Count(v, "\n") + 1; n != 20 {
		t.Errorf("View has %d lines, want 20:\n%s", n, v)
	}
}
