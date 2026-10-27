package hub

import (
	"path"
	"strconv"
	"strings"
	"time"

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
const keyHint = " ⏎ jump  ␣ fold  o open  y copy  m merge  f fix  x forget  r refresh  q quit"

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

// prLine renders "<name> #<n> <checks> <mergeState> <review> <n threads>",
// led by "!" on an alert so truncation never hides it; empty parts (name
// included) are omitted.
func prLine(name string, pr *PR) string {
	var parts []string
	if pr.Alert != "" {
		parts = append(parts, red.Render("!"))
	}
	if name != "" {
		parts = append(parts, name)
	}
	parts = append(parts, "#"+strconv.Itoa(pr.Number))
	threads := ""
	if pr.UnresolvedThreads > 0 {
		threads = strconv.Itoa(pr.UnresolvedThreads) + " threads"
	}
	for _, p := range []string{checksIcon(pr.Checks), pr.MergeState, reviewShort[pr.ReviewDecision], threads} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " ")
}

func agentName(a *Agent) string {
	if a.Title != "" {
		return a.Title
	}
	return a.SessionID
}

// agentLabel is an AGENTS row's name: the title, else the slug, else the
// session id.
func agentLabel(a *Agent) string {
	if a.Title != "" {
		return a.Title
	}
	if a.Slug != "" {
		return a.Slug
	}
	return a.SessionID
}

// live marks an agent open (▶), closed (⏸) or on another tmux server (⇄).
func live(a *Agent) string {
	if !a.OnHub() {
		return "⇄"
	}
	if a.Open {
		return "▶"
	}
	return "⏸"
}

// fold is the arrow of a group row: open or closed.
func fold(open bool) string {
	if open {
		return "▾"
	}
	return "▸"
}

func itemLine(it Item) string {
	switch it.Kind {
	case KindHeader:
		count := strconv.Itoa(it.Count)
		if it.Title == agentsHeader {
			count += "/" + strconv.Itoa(it.Total)
		}
		return bold.Render(it.Title + " " + count)
	case KindRepoGroup:
		return fold(it.Expanded) + " " + it.Title + " " + strconv.Itoa(it.Count)
	case KindMergedGroup:
		return fold(it.Expanded) + " merged (" + strconv.Itoa(it.Count) + ")"
	case KindTask:
		line := "  " + stateIcon(it.Task.State) + " " + it.Title
		if it.Agent != nil {
			line += "  " + live(it.Agent) + " " + agentName(it.Agent)
		}
		if it.Merged {
			line += gray.Render(" merged")
		}
		return line
	case KindTaskPR:
		return "      " + prLine("", it.PR)
	case KindAgent:
		a := it.Agent
		if it.Task != nil { // under its worktree
			return "      " + live(a) + " " + stateIcon(a.State) + " " + agentName(a)
		}
		if !a.OnHub() {
			return live(a) + " " + agentLabel(a) + " · other tmux"
		}
		line := live(a) + " " + stateIcon(a.State) + " " + agentLabel(a)
		if a.Slug != "" {
			line += " · " + a.Slug
		}
		if a.Open {
			line += " w" + strconv.Itoa(a.Window)
		}
		return line
	case KindPR:
		return prLine(path.Base(it.PR.Repo), it.PR)
	}
	return ""
}

// body renders the item rows, with a rule before every section after the
// first, and returns the line index of the cursor and, for every line, the
// index of its item (-1 for a rule).
func (m Model) body() (lines []string, cursorLine int, lineItem []int) {
	rule := m.rule()
	for i, it := range m.items {
		if it.Kind == KindHeader && i > 0 {
			lines = append(lines, rule)
			lineItem = append(lineItem, -1)
		}
		line := itemLine(it)
		if i == m.cursor {
			cursorLine = len(lines)
			// Plain text: a styled piece's reset would end the background early.
			line = selected.Render("▌" + ansi.Strip(line))
		} else {
			line = " " + line
		}
		lines = append(lines, line)
		lineItem = append(lineItem, i)
	}
	return lines, cursorLine, lineItem
}

// headerLines is the title and the rule above the rows.
const headerLines = 2

// scroll returns the first body line shown and how many are shown, so the
// cursor stays visible above the footer (all of them before the pane size
// is known).
func (m Model) scroll(lines, cursor, footLines int) (off, n int) {
	if m.height <= 0 {
		return 0, lines
	}
	visible := max(m.height-headerLines-1-footLines, 1)
	off = max(cursor-visible+1, 0)
	return off, min(off+visible, lines) - off
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

// footer is any prompt, notice or error, and the key hint.
func (m Model) footer() []string {
	var foot []string
	switch {
	case m.prompt != "":
		for _, l := range wrapLines(m.prompt, m.width, 2) {
			foot = append(foot, yellow.Render(l))
		}
	case m.notice != "" && m.failed:
		foot = append(foot, red.Render(oneLine(m.notice, m.width)))
	case m.notice != "":
		foot = append(foot, gray.Render(oneLine(m.notice, m.width)))
	}
	var errs []string
	for _, err := range []error{m.err, m.localErr, m.githubErr} {
		if err != nil {
			errs = append(errs, err.Error())
		}
	}
	if m.state.Error != "" {
		errs = append(errs, m.state.Error)
	}
	if len(errs) > 0 {
		foot = append(foot, red.Render(oneLine(strings.Join(errs, "\n"), m.width)))
	}
	return append(foot, gray.Render(keyHint))
}

// itemAt is the index of the item drawn on screen line y, or -1 for the
// title, a rule, the footer or past the end.
func (m Model) itemAt(y int) int {
	rows, cursor, lineItem := m.body()
	off, n := m.scroll(len(rows), cursor, len(m.footer()))
	if y < headerLines || y >= headerLines+n {
		return -1
	}
	return lineItem[off+y-headerLines]
}

// View is the title and a rule, the item rows scrolled so the cursor stays
// visible, then a rule, any prompt, notice or error, and the key hint.
func (m Model) View() string {
	foot := m.footer()
	rows, cursor, _ := m.body()
	off, n := m.scroll(len(rows), cursor, len(foot))
	rows = rows[off : off+n]
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

// wrapLines word-wraps s to width and keeps at most limit lines, the last one
// truncated when more text follows.
func wrapLines(s string, width, limit int) []string {
	if width <= 0 {
		return []string{s}
	}
	lines := strings.Split(ansi.Wrap(s, width, ""), "\n")
	if len(lines) > limit {
		rest := strings.Join(lines[limit-1:], " ")
		lines = append(lines[:limit-1], ansi.Truncate(rest, width-1, "")+"…")
	}
	return lines
}

// minErrWidth is the fewest columns an error keeps before its hub.log hint is
// dropped.
const minErrWidth = 10

// oneLine renders s as its first line, truncated so that it and, when s has
// more lines, a "(+N, hub.log)" hint fit width; the hint is dropped when it
// would leave the message fewer than minErrWidth columns.
func oneLine(s string, width int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	suffix := ""
	if n := len(lines) - 1; n > 0 {
		suffix = " (+" + strconv.Itoa(n) + ", hub.log)"
	}
	first := lines[0]
	if width > 0 {
		if width-ansi.StringWidth(suffix) < minErrWidth {
			suffix = ""
		}
		first = ansi.Truncate(first, max(width-ansi.StringWidth(suffix), 1), "…")
	}
	return first + suffix
}
