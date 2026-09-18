// Package applyview is the live bubbletea view for `zenify up`'s apply phase:
// a per-repo filling bar (▰▱), a braille spinner on the in-flight repo, and a
// streaming list of finished repos. It runs ONLY on a real TTY; the caller must
// not start it for a piped/NO_COLOR run.
package applyview

import (
	"fmt"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/reconcile"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ProgressMsg reports that one repo finished; Done runs 1..Total. Failed
// reports whether that repo's Result.Err was set, so the view can mark it
// distinctly from a success.
type ProgressMsg struct {
	Done, Total int
	Repo        string
	State       reconcile.State
	Failed      bool
}

// DoneMsg tells the model the apply loop has returned; the model then quits.
type DoneMsg struct{}

type finished struct {
	repo   string
	state  reconcile.State
	failed bool
}

// Model is the tea.Model for the apply phase.
type Model struct {
	total  int
	done   int
	list   []finished
	spin   spinner.Model
	filled lipgloss.Style
	empty  lipgloss.Style
	quit   bool
}

// New builds the model for a run over `total` repos.
func New(total int) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot // braille frames ⣾⣽⣻⢿⡿⣟⣯⣷
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	return Model{
		total:  total,
		spin:   sp,
		filled: lipgloss.NewStyle().Foreground(lipgloss.Color("2")), // mint/green
		empty:  lipgloss.NewStyle().Faint(true),
	}
}

func (m Model) Init() tea.Cmd { return m.spin.Tick }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ProgressMsg:
		m.done = msg.Done
		m.total = msg.Total
		m.list = append(m.list, finished{repo: msg.Repo, state: msg.State, failed: msg.Failed})
		return m, nil
	case DoneMsg:
		m.quit = true
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder
	bar := Bar(m.done, m.total, 11, m.filled, m.empty)
	fmt.Fprintf(&b, "%s  %s  %s  %d/%d\n", dim("│"), spin(m), "Applying", m.done, m.total)
	fmt.Fprintf(&b, "%s  %s\n", dim("│"), bar)
	for _, f := range m.list {
		glyph, color := "✓", "2" // mint/green
		if f.failed {
			glyph, color = "✗", "1" // red
		}
		mark := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(glyph)
		fmt.Fprintf(&b, "%s  %s %s\n", dim("│"), mark, f.repo)
	}
	return b.String()
}

func spin(m Model) string {
	if m.quit {
		return " "
	}
	return m.spin.View()
}

func dim(s string) string { return lipgloss.NewStyle().Faint(true).Render(s) }

// Bar renders a clack-style block bar: `filled` ▰ cells then `empty` ▱ cells,
// width total, proportional to done/total. Exported for reuse + testing.
func Bar(done, total, width int, filled, empty lipgloss.Style) string {
	if total <= 0 {
		total = 1
	}
	n := done * width / total
	if n > width {
		n = width
	}
	return filled.Render(strings.Repeat("▰", n)) + empty.Render(strings.Repeat("▱", width-n))
}
