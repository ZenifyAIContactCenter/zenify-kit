package hub

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/tmuxx"
)

// CopyURL is the text `y` copies: the PR URL.
func CopyURL(pr PR) string { return pr.URL }

// MergeCommand is the text `m` copies. The hub never merges by itself.
func MergeCommand(pr PR) string {
	return fmt.Sprintf("gh pr merge %d --repo %s --merge", pr.Number, pr.Repo)
}

// OSC52 wraps s in the terminal clipboard escape. Inside tmux the sequence is
// sent through tmux passthrough so it reaches the outer terminal (also over SSH).
func OSC52(s string) string {
	seq := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(s)) + "\a"
	if os.Getenv("TMUX") != "" {
		return "\x1bPtmux;\x1b" + seq + "\x1b\\"
	}
	return seq
}

// FixPrompt is the text `f` types into the task's agent.
func FixPrompt(pr PR) string { return "/znf:fix " + pr.FailedURL }

// Ack records that the user saw pr's alert: it creates or touches
// acks/<owner>_<repo>_<n>, which the poller compares with AlertAt.
func Ack(home string, pr PR) error {
	if err := touch(filepath.Join(AcksDir(home), ackName(pr)), time.Now()); err != nil {
		return fmt.Errorf("ack %s: %w", prKey(pr), err)
	}
	return nil
}

// touch creates path (and its directory) if missing and sets its mtime to now.
func touch(path string, now time.Time) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // G304 -- path is built from the hub data dir, not external input
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Chtimes(path, now, now)
}

// ItemKind is the kind of one sidebar row.
type ItemKind int

const (
	KindHeader      ItemKind = iota
	KindRepoGroup            // a repo in TASKS; Enter or Space expands or collapses it
	KindTask                 // a worktree (task × repo), under its repo group or the merged group
	KindTaskPR               // the open PR of the worktree row above it
	KindAgent                // under a task with several agents, or in the AGENTS section
	KindPR                   // a PR that matches no task
	KindMergedGroup          // the "merged (n)" row; Enter or Space expands or collapses it
)

// Item is one sidebar row. Worktree rows and the rows under them carry Task
// and the Repo they render under, with that repo's PR; a worktree row carries
// Agent when the task has exactly one agent.
type Item struct {
	Kind     ItemKind
	Title    string // header text, repo group name, or worktree label
	Count    int    // header count; group size; open agents in the AGENTS header
	Total    int    // all agents, in the AGENTS header only
	Expanded bool   // a repo or merged group is open
	Merged   bool   // a worktree row whose repo is merged, kept in its repo group
	Task     *Task
	Repo     *TaskRepo
	Agent    *Agent
	PR       *PR
}

// allMerged reports whether every repo of t is merged (FR-10.4).
func allMerged(t Task) bool {
	for _, r := range t.Repos {
		if !r.Merged {
			return false
		}
	}
	return len(t.Repos) > 0
}

func hasPR(t Task) bool {
	for _, r := range t.Repos {
		if r.PR != nil {
			return true
		}
	}
	return false
}

// worktree is one task in one of its repos.
type worktree struct {
	t *Task
	r *TaskRepo
}

// active reports work in flight: an agent on the task or an open PR here.
func (w worktree) active() bool { return len(w.t.Agents) > 0 || w.r.PR != nil }

// repoGroup is the worktrees of one repo, sorted for display.
type repoGroup struct {
	name   string
	ws     []worktree
	active bool
	top    int // highest roll-up rank of its active worktrees
}

// Flatten lays s out as rows: TASKS, AGENTS (every agent, open ones first)
// and MY PRS (other PRs); headers are always present. TASKS groups worktrees by repo: repos
// with an active worktree come first (by highest roll-up, then name) and are
// open unless folds says otherwise, the rest follow by name, collapsed
// unless folds opens them. Inside a repo, active worktrees come first (by
// roll-up, then slug), then the rest by slug. A worktree whose task is merged
// in every repo and has neither an agent nor an open PR goes into one merged
// group at the end, listed as repo/slug when expanded (FR-10.4).
func Flatten(s State, expanded bool, folds map[string]bool) []Item {
	byName := map[string]*repoGroup{}
	var groups []*repoGroup
	var done []worktree
	for i := range s.Tasks {
		t := &s.Tasks[i]
		for j := range t.Repos {
			w := worktree{t: t, r: &t.Repos[j]}
			if allMerged(*t) && len(t.Agents) == 0 && !hasPR(*t) {
				done = append(done, w)
				continue
			}
			g := byName[w.r.Name]
			if g == nil {
				g = &repoGroup{name: w.r.Name}
				byName[w.r.Name] = g
				groups = append(groups, g)
			}
			g.ws = append(g.ws, w)
			if w.active() {
				g.top = max(g.top, rank(t.State))
				g.active = true
			}
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		a, b := groups[i], groups[j]
		if a.active != b.active {
			return a.active
		}
		if a.active && a.top != b.top {
			return a.top > b.top
		}
		return a.name < b.name
	})
	n := 0
	for _, g := range groups {
		n += len(g.ws)
		sort.Slice(g.ws, func(i, j int) bool {
			a, b := g.ws[i], g.ws[j]
			if a.active() != b.active() {
				return a.active()
			}
			if a.active() && rank(a.t.State) != rank(b.t.State) {
				return rank(a.t.State) > rank(b.t.State)
			}
			return a.t.Slug < b.t.Slug
		})
	}
	sort.Slice(done, func(i, j int) bool { return label(done[i]) < label(done[j]) })

	items := []Item{{Kind: KindHeader, Title: "TASKS", Count: n}}
	for _, g := range groups {
		open := g.active
		if v, ok := folds[g.name]; ok {
			open = v
		}
		items = append(items, Item{Kind: KindRepoGroup, Title: g.name, Count: len(g.ws), Expanded: open})
		if open {
			for _, w := range g.ws {
				items = appendWorktree(items, s, w, w.t.Slug, w.r.Merged)
			}
		}
	}
	if len(done) > 0 {
		items = append(items, Item{Kind: KindMergedGroup, Count: len(done), Expanded: expanded})
		if expanded {
			for _, w := range done {
				items = appendWorktree(items, s, w, label(w), false)
			}
		}
	}
	agents := sortedAgents(s.Agents)
	open := 0
	for _, a := range agents {
		if a.Open {
			open++
		}
	}
	items = append(items, Item{Kind: KindHeader, Title: agentsHeader, Count: open, Total: len(agents)})
	for i := range agents {
		items = append(items, Item{Kind: KindAgent, Agent: &agents[i]})
	}
	items = append(items, Item{Kind: KindHeader, Title: "MY PRS", Count: len(s.OtherPRs)})
	for i := range s.OtherPRs {
		items = append(items, Item{Kind: KindPR, PR: &s.OtherPRs[i]})
	}
	return items
}

// agentsHeader is the title of the AGENTS section, whose count is open/total.
const agentsHeader = "AGENTS"

// sortedAgents lists agents open first (by state rank, then newest
// UpdatedAt), then closed (newest UpdatedAt); ties by session id.
func sortedAgents(agents map[string]Agent) []Agent {
	out := make([]Agent, 0, len(agents))
	for _, a := range agents {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Open != b.Open {
			return a.Open
		}
		if a.Open && rank(a.State) != rank(b.State) {
			return rank(a.State) > rank(b.State)
		}
		if a.UpdatedAt != b.UpdatedAt {
			return a.UpdatedAt > b.UpdatedAt
		}
		return a.SessionID < b.SessionID
	})
	return out
}

// label is a worktree's name in the merged group.
func label(w worktree) string { return w.r.Name + "/" + w.t.Slug }

// appendWorktree adds w's row titled title (tagged merged when its repo is
// merged but it stays in its repo group), its PR row when the repo has an
// open PR and, when the task has several agents, its agent rows.
func appendWorktree(items []Item, s State, w worktree, title string, merged bool) []Item {
	var agents []*Agent
	for _, id := range w.t.Agents {
		if a, ok := s.Agents[id]; ok {
			agents = append(agents, &a)
		}
	}
	row := Item{Kind: KindTask, Title: title, Task: w.t, Repo: w.r, PR: w.r.PR, Merged: merged}
	if len(agents) == 1 {
		row.Agent = agents[0]
	}
	items = append(items, row)
	if w.r.PR != nil {
		items = append(items, Item{Kind: KindTaskPR, Task: w.t, Repo: w.r, PR: w.r.PR})
	}
	if len(agents) > 1 {
		for _, a := range agents {
			items = append(items, Item{Kind: KindAgent, Task: w.t, Repo: w.r, Agent: a})
		}
	}
	return items
}

// itemKey identifies a row across layouts, so the cursor can follow it.
func itemKey(it Item) string {
	switch it.Kind {
	case KindHeader, KindRepoGroup:
		return fmt.Sprintf("%d/%s", it.Kind, it.Title)
	case KindMergedGroup:
		return "merged"
	case KindPR:
		return fmt.Sprintf("pr/%s#%d", it.PR.Repo, it.PR.Number)
	}
	k := strconv.Itoa(int(it.Kind))
	if it.Repo != nil {
		k += "/" + it.Repo.Name + "/" + it.Task.Slug
	}
	if it.Agent != nil {
		k += "/" + it.Agent.SessionID
	}
	return k
}

// TargetAgent picks the agent a row jumps to: an agent row is itself; a row
// inside a task picks an open task agent when there is one, then the highest
// roll-up state, ties broken by the newest UpdatedAt.
func TargetAgent(s State, it Item) (Agent, bool) {
	if it.Kind == KindAgent && it.Agent != nil {
		return *it.Agent, true
	}
	if it.Task == nil {
		return Agent{}, false
	}
	var best Agent
	found := false
	for _, id := range it.Task.Agents {
		a, ok := s.Agents[id]
		if !ok {
			continue
		}
		if !found || (a.Open && !best.Open) || (a.Open == best.Open && (rank(a.State) > rank(best.State) ||
			(rank(a.State) == rank(best.State) && a.UpdatedAt > best.UpdatedAt))) {
			best, found = a, true
		}
	}
	return best, found
}

// NewAgentWindow opens a window running claude in root, with a hub sidebar
// (self is the zenify binary) split to its left; focus stays on claude. The
// window goes into the zenify session, or into the caller's current session
// (tmux resolves it from the TMUX env) when no zenify session exists; in the
// zenify session the client is switched to the new window. When the split
// fails the new window is killed. allow-passthrough (OSC 52 copy) comes from
// the hub tmux.conf.
func NewAgentWindow(r tmuxx.Runner, root, self string) error {
	return agentWindow(r, root, self, "agent", "claude")
}

// ResumeAgentWindow is NewAgentWindow running `claude --resume sessionID` in
// a window called name. An invalid session id is refused before any tmux call.
func ResumeAgentWindow(r tmuxx.Runner, root, self, sessionID, name string) error {
	if !ValidSessionID(sessionID) {
		return fmt.Errorf("invalid session id %q", sessionID)
	}
	return agentWindow(r, root, self, name, "claude", "--resume", sessionID)
}

// sessionIDRe is the UUID-ish shape of a Claude session id; it starts with a
// hex digit so it can never pass for a flag.
var sessionIDRe = regexp.MustCompile(`^[0-9a-fA-F][0-9a-fA-F-]*$`)

// ValidSessionID reports whether id is safe to put on a command line.
func ValidSessionID(id string) bool { return sessionIDRe.MatchString(id) }

// WindowName is the tmux window name for an agent on slug: the lowercased
// slug with every rune outside [a-z0-9-] as '-' and outer '-' trimmed, or
// "agent" when nothing is left.
func WindowName(slug string) string {
	name := strings.Trim(strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		return '-'
	}, strings.ToLower(slug)), "-")
	if name == "" {
		return "agent"
	}
	return name
}

// agentWindow opens the window name running command, with the sidebar split
// to its left; see NewAgentWindow.
func agentWindow(r tmuxx.Runner, root, self, name string, command ...string) error {
	args := append([]string{"new-window", "-P", "-F", "#{window_id}", "-c", root, "-n", name}, command...)
	inHub := tmuxx.HasSession(r)
	if inHub {
		args = append([]string{"new-window", "-t", tmuxx.Target + ":"}, args[1:]...)
	}
	out, err := r.Run(args...)
	if err != nil {
		return err
	}
	win := strings.TrimSpace(string(out))
	if win == "" {
		return errors.New("tmux new-window printed no window id")
	}
	if _, err := r.Run("split-window", "-d", "-h", "-b", "-l", "30%", "-t", win, "-c", root, self, "hub", "sidebar"); err != nil {
		// A window without its sidebar would linger as a bare claude.
		_, kerr := r.Run("kill-window", "-t", win)
		return errors.Join(err, kerr)
	}
	if inHub { // the caller may sit in another session; follow the window like jump
		if _, err := r.Run("switch-client", "-t", win); err != nil {
			return err
		}
	}
	return nil
}
