package hub

import (
	"fmt"
	"os"
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

func TestRepoName_PadsByRunes(t *testing.T) {
	task := &Task{Repos: []TaskRepo{{Name: "\u00df\u00df"}, {Name: "ab"}}} // two runes, four bytes
	if got := repoName(task, &task.Repos[1]); got != "ab" {
		t.Errorf("repoName = %q, want %q", got, "ab")
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
		"▌○ zenify-hub · kit",
		rule,
		" AGENTS 1",
		" ○ Daily work",
		rule,
		" MY PRS 1",
		" monitoring #205 ◐ BLOCKED",
		rule,
		" ⏎ jump  o open  y copy  m merge  f fix ",
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

func TestView_TaskOrderAndMergedGroup(t *testing.T) {
	m, _, _ := newTestModel(t, t.TempDir(), orderState())
	want := []string{
		" TASKS 5",
		"▌◆ d-block · web  agent#2",
		" ● c-work · be  agent#1",
		" ○ b-pr",
		"     be #7",
		" ○ a-idle · be",
		" ○ z-half",
		"     be merged",
		"     web",
		" ▸ merged (2)",
	}
	if got := taskBlock(m.View()); !reflect.DeepEqual(got, want) {
		t.Fatalf("TASKS block:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	for i := 0; i < 8; i++ {
		m = update(t, m, key("j"))
	}
	m = update(t, m, key("enter"))
	want = append(want[:9],
		"▌▾ merged (2)",
		" ○ m1",
		"     be merged",
		"     web merged",
		" ○ m2 · kit",
	)
	want[1] = " ◆ d-block · web  agent#2"
	if got := taskBlock(m.View()); !reflect.DeepEqual(got, want) {
		t.Fatalf("expanded TASKS block:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	m = update(t, m, key("enter"))
	if got := taskBlock(m.View()); got[len(got)-1] != "▌▸ merged (2)" {
		t.Errorf("enter did not collapse:\n%s", strings.Join(got, "\n"))
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
	want := []string{" TASKS 1", "▌● t1 · be", "     ● one", "     ○ two"}
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
	for i := 0; i < 24; i++ {
		m = update(t, m, key("j"))
	}
	v := m.View()
	if !strings.Contains(v, "\n▌○ task-25 · be\n") {
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
	want := []string{" TASKS 1", "▌◆ m2 · kit  agent#2", " ▸ merged (1)"}
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
	want := []string{" TASKS 1", "▌○ m2", "     kit #9", " ▸ merged (1)"}
	if got := taskBlock(m.View()); !reflect.DeepEqual(got, want) {
		t.Errorf("TASKS block:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
