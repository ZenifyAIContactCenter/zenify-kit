package hub

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// prRows is a sidebar whose only rows are five PRs under MY PRS. Screen lines:
// 0 title, 1 rule, 2 TASKS, 3 rule, 4 AGENTS, 5 rule, 6 MY PRS, 7..11 #1..#5.
func prRows() State {
	var prs []PR
	for i, r := range []string{"o/a", "o/b", "o/c", "o/d", "o/e"} {
		prs = append(prs, PR{Number: i + 1, Repo: r, URL: "u"})
	}
	return State{OtherPRs: prs}
}

func click(y int) tea.MouseMsg {
	return tea.MouseMsg{X: 3, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
}

func TestMouse_ClickSelectsThenClickAgainIsEnter(t *testing.T) { // SC-27
	m, _, _ := newTestModel(t, t.TempDir(), prRows())
	gh := m.gh.(*fakeGH)
	m = update(t, m, click(9)) // third PR row
	if it := m.current(); it.Kind != KindPR || it.PR.Number != 3 {
		t.Fatalf("after first click cursor on %+v, want PR #3", it)
	}
	if !m.moved {
		t.Error("a click must count as moving the cursor")
	}
	if gh.args != nil {
		t.Fatalf("first click must only select, gh calls %q", gh.args)
	}
	m = update(t, m, click(9))
	if it := m.current(); it.Kind != KindPR || it.PR.Number != 3 {
		t.Errorf("after second click cursor on %+v, want PR #3", it)
	}
	if want := [][]string{{"pr", "view", "3", "--repo", "o/c", "--web"}}; !reflect.DeepEqual(gh.args, want) {
		t.Errorf("second click: gh calls = %q, want Enter's %q", gh.args, want)
	}
}

func TestMouse_ClickIgnoresTitleHeaderRuleAndFooter(t *testing.T) {
	m, _, _ := newTestModel(t, t.TempDir(), prRows())
	m = update(t, m, click(8)) // PR #2
	for _, y := range []int{0, 1, 2, 3, 6, 12, 13, 40} {
		m = update(t, m, click(y))
		if it := m.current(); it.Kind != KindPR || it.PR.Number != 2 {
			t.Errorf("click at y=%d moved the cursor to %+v", y, it)
		}
	}
	if gh := m.gh.(*fakeGH); gh.args != nil {
		t.Errorf("ignored clicks ran an action: %q", gh.args)
	}
}

func TestMouse_ClickMapsThroughScroll(t *testing.T) {
	m, _, _ := newTestModel(t, t.TempDir(), prRows())
	// 8 lines: title, rule, 4 rows, rule, key hint.
	m = update(t, m, tea.WindowSizeMsg{Width: 40, Height: 8})
	for range 5 {
		m = update(t, m, key("j")) // to PR #5; body lines 6..9 (#2..#5) are shown
	}
	m = update(t, m, click(2)) // first shown row
	if it := m.current(); it.Kind != KindPR || it.PR.Number != 2 {
		t.Errorf("click at y=2 selected %+v, want PR #2\n%s", it, m.View())
	}
	m = update(t, m, click(6)) // the rule under the rows
	if it := m.current(); it.Kind != KindPR || it.PR.Number != 2 {
		t.Errorf("click on the footer rule selected %+v", it)
	}
}

func TestMouse_WheelMovesLikeJK(t *testing.T) {
	m, _, _ := newTestModel(t, t.TempDir(), prRows())
	m = update(t, m, tea.MouseMsg{Y: 9, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if it := m.current(); it.Kind != KindPR || it.PR.Number != 2 {
		t.Fatalf("wheel down: cursor on %+v, want PR #2", it)
	}
	if !m.moved {
		t.Error("wheel must count as moving the cursor")
	}
	m = update(t, m, tea.MouseMsg{Y: 9, Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	if it := m.current(); it.Kind != KindPR || it.PR.Number != 1 {
		t.Errorf("wheel up: cursor on %+v, want PR #1", it)
	}
}

func TestMouse_ReleaseAndRightClickIgnored(t *testing.T) {
	m, _, _ := newTestModel(t, t.TempDir(), prRows())
	m = update(t, m, tea.MouseMsg{Y: 9, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
	m = update(t, m, tea.MouseMsg{Y: 9, Button: tea.MouseButtonRight, Action: tea.MouseActionPress})
	if it := m.current(); it.Kind != KindPR || it.PR.Number != 1 {
		t.Errorf("cursor on %+v, want PR #1", it)
	}
}

func TestMouse_IgnoredWhilePromptIsUp(t *testing.T) {
	m, _, _ := newTestModel(t, t.TempDir(), prRows())
	m = m.ask("y/n · test?", func(m Model) Model { return m })
	m = update(t, m, click(9))
	if m.prompt == "" {
		t.Error("a click cancelled the prompt")
	}
	if it := m.current(); it.PR.Number != 1 {
		t.Errorf("a click moved the cursor under a prompt to %+v", it)
	}
}

func TestMouse_ScrolledClickKeepsOffsetAndSecondClickActs(t *testing.T) {
	m, _, _ := newTestModel(t, t.TempDir(), prRows())
	m = update(t, m, tea.WindowSizeMsg{Width: 40, Height: 8})
	for range 5 {
		m = update(t, m, key("j")) // cursor on PR #5; rows #2..#5 shown
	}
	off := m.offset
	m = update(t, m, click(4)) // third shown row: PR #4
	if it := m.current(); it.Kind != KindPR || it.PR.Number != 4 {
		t.Fatalf("first click selected %+v, want PR #4\n%s", it, m.View())
	}
	if m.offset != off {
		t.Fatalf("click shifted offset %d -> %d", off, m.offset)
	}
	gh := m.gh.(*fakeGH)
	if gh.args != nil {
		t.Fatalf("first click ran %q", gh.args)
	}
	m = update(t, m, click(4))
	if it := m.current(); it.PR.Number != 4 {
		t.Errorf("second click moved cursor to %+v", it)
	}
	if want := [][]string{{"pr", "view", "4", "--repo", "o/d", "--web"}}; !reflect.DeepEqual(gh.args, want) {
		t.Errorf("second click gh calls = %q, want %q", gh.args, want)
	}
}

func TestScroll_KeyboardKeepsCursorVisible(t *testing.T) {
	m, _, _ := newTestModel(t, t.TempDir(), prRows())
	m = update(t, m, tea.WindowSizeMsg{Width: 40, Height: 8})
	for range 5 {
		m = update(t, m, key("j"))
	}
	for range 5 {
		m = update(t, m, key("k"))
		if !strings.Contains(m.View(), "#"+strconv.Itoa(m.current().PR.Number)) {
			t.Fatalf("cursor row not visible:\n%s", m.View())
		}
	}
}
