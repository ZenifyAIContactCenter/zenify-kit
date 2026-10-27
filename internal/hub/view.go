package hub

import (
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	red      = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	green    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	yellow   = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	gray     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	bold     = lipgloss.NewStyle().Bold(true)
	selected = lipgloss.NewStyle().Background(lipgloss.Color("237"))
)

// keyHint is the last line of the sidebar, clipped to the pane width.
const keyHint = " ⏎ jump  o open  y copy  m merge  f fix  r refresh  q quit"

// defaultRule is the rule width before the pane size is known.
const defaultRule = 30

func stateIcon(s AgentState) string {
	switch s {
	case Working:
		return green.Render("●")
	case Blocked:
		return red.Render("◆")
	case Done:
		return yellow.Render("✓")
	}
	return gray.Render("○")
}

func checksIcon(c string) string {
	switch c {
	case "SUCCESS":
		return green.Render("✓")
	case "FAILURE":
		return red.Render("✗")
	case "PENDING":
		return yellow.Render("◐")
	}
	return ""
}

// reviewShort is the PR line's form of ReviewDecision; unknown values omit.
var reviewShort = map[string]string{
	"APPROVED":          "approved",
	"CHANGES_REQUESTED": "changes",
	"REVIEW_REQUIRED":   "review",
}

// prLine renders "<name> #<n> <checks> <mergeState> <review> <n threads>"
// plus "!" on an alert; empty parts are omitted.
func prLine(name string, pr *PR) string {
	parts := []string{name, "#" + strconv.Itoa(pr.Number)}
	threads := ""
	if pr.UnresolvedThreads > 0 {
		threads = strconv.Itoa(pr.UnresolvedThreads) + " threads"
	}
	for _, p := range []string{checksIcon(pr.Checks), pr.MergeState, reviewShort[pr.ReviewDecision], threads} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if pr.Alert != "" {
		parts = append(parts, red.Render("!"))
	}
	return strings.Join(parts, " ")
}

func agentName(a *Agent) string {
	if a.Title != "" {
		return a.Title
	}
	return a.SessionID
}

// repoName is r's name padded to the longest repo name of t, so PR columns
// line up.
func repoName(t *Task, r *TaskRepo) string {
	w := 0
	for _, x := range t.Repos {
		w = max(w, utf8.RuneCountInString(x.Name)) // %-*s pads by runes
	}
	return fmt.Sprintf("%-*s", w, r.Name)
}

func itemLine(it Item) string {
	switch it.Kind {
	case KindHeader:
		return bold.Render(it.Title + " " + strconv.Itoa(it.Count))
	case KindMergedGroup:
		return "▸ merged (" + strconv.Itoa(it.Count) + ")"
	case KindTask:
		line := stateIcon(it.Task.State) + " " + it.Task.Slug
		if it.Repo != nil {
			line += " · " + it.Repo.Name
		}
		if it.Agent != nil {
			line += "  " + agentName(it.Agent)
		}
		return line
	case KindRepo:
		if it.PR != nil {
			return "    " + prLine(repoName(it.Task, it.Repo), it.PR)
		}
		if it.Repo.Merged {
			return "    " + it.Repo.Name + gray.Render(" merged")
		}
		return "    " + it.Repo.Name
	case KindAgent:
		indent := ""
		if it.Task != nil {
			indent = "    "
		}
		return indent + stateIcon(it.Agent.State) + " " + agentName(it.Agent)
	case KindPR:
		return prLine(path.Base(it.PR.Repo), it.PR)
	}
	return ""
}

// body renders the item rows, with a rule before every section after the
// first, and returns the line index of the cursor.
func (m Model) body() (lines []string, cursorLine int) {
	rule := m.rule()
	for i, it := range m.items {
		if it.Kind == KindHeader && i > 0 {
			lines = append(lines, rule)
		}
		line := itemLine(it)
		if it.Kind == KindMergedGroup && m.expanded {
			line = "▾" + strings.TrimPrefix(line, "▸")
		}
		if i == m.cursor {
			cursorLine = len(lines)
			// Plain text: a styled piece's reset would end the background early.
			line = selected.Render("▌" + ansi.Strip(line))
		} else {
			line = " " + line
		}
		lines = append(lines, line)
	}
	return lines, cursorLine
}

func (m Model) rule() string {
	w := m.width
	if w <= 0 {
		w = defaultRule
	}
	return gray.Render(strings.Repeat("─", w))
}

// title is "zenify hub" with the last local round's time on the right.
func (m Model) title() string {
	left, right := " zenify hub", "--:--:--"
	if m.state.LocalAt > 0 {
		right = time.Unix(m.state.LocalAt, 0).Format("15:04:05")
	}
	gap := max(m.width-lipgloss.Width(left)-lipgloss.Width(right), 2)
	return bold.Render(left) + strings.Repeat(" ", gap) + gray.Render(right)
}

// View is the title and a rule, the item rows scrolled so the cursor stays
// visible, then a rule, any prompt, notice or error, and the key hint.
func (m Model) View() string {
	var foot []string
	switch {
	case m.prompt != "":
		foot = append(foot, yellow.Render(m.prompt))
	case m.notice != "" && m.failed:
		foot = append(foot, red.Render(m.notice))
	case m.notice != "":
		foot = append(foot, gray.Render(m.notice))
	}
	if m.err != nil {
		foot = append(foot, red.Render(m.err.Error()))
	}
	if m.state.Error != "" {
		foot = append(foot, red.Render(m.state.Error))
	}
	foot = append(foot, gray.Render(keyHint))

	rows, cursor := m.body()
	if m.height > 0 {
		footLines := strings.Count(strings.Join(foot, "\n"), "\n") + 1 // errors may span lines
		visible := max(m.height-2-1-footLines, 1)
		off := max(cursor-visible+1, 0)
		rows = rows[off:min(off+visible, len(rows))]
	}
	lines := append([]string{m.title(), m.rule()}, rows...)
	lines = append(append(lines, m.rule()), foot...)
	if m.width > 0 {
		clip := lipgloss.NewStyle().MaxWidth(m.width)
		for i, l := range lines {
			lines[i] = clip.Render(l)
		}
	}
	return strings.Join(lines, "\n")
}
