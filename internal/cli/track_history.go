package cli

import (
	"bufio"
	"crypto/sha1" //nolint:gosec // G505 -- a short stable id, not a security use
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/spf13/cobra"
)

var (
	histBranchRe = regexp.MustCompile(`^[^/]+/(feat|fix|hotfix)/(.+)$`)
	histKindOf   = map[string]string{"feat": "cook", "fix": "fix", "hotfix": "hotfix"}
	histSkills   = map[string]string{"znf:cook": "cook", "znf:fix": "fix", "znf:hotfix": "hotfix"}
	// histNotPrompt are the text prefixes of user records that are not typed by a human.
	// The first is split so the source holds no repo-name literal (see release_report_test.go).
	histNotPrompt = []string{"<task-" + "notif" + "ication>", "<system-reminder>", "<command-name>", "<local-command"}
)

func newTrackOnboardCmd(ws *string) *cobra.Command {
	c := &cobra.Command{
		Use:   "onboard",
		Short: "Nạp dữ liệu ban đầu cho track từ memory và lịch sử transcript", //znf:allow-lang
	}
	c.AddCommand(newTrackOnboardMemoryCmd(ws), newTrackOnboardHistoryCmd(ws))
	return c
}

func newTrackOnboardMemoryCmd(ws *string) *cobra.Command {
	return &cobra.Command{
		Use:   "memory",
		Short: "Đánh dấu đã onboard từ memory (append dòng onboard)", //znf:allow-lang
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			w := cmd.OutOrStdout()
			wsDir, on, err := trackWorkspace(*ws)
			if err != nil {
				return err
			}
			if !on {
				_, err = fmt.Fprintln(w, "track: off")
				return err
			}
			if err := appendTrackRow(trackLogPath(wsDir), "onboard", []string{"memory"}); err != nil {
				return err
			}
			_, err = fmt.Fprintln(w, "track: onboarded")
			return err
		},
	}
}

func newTrackOnboardHistoryCmd(ws *string) *cobra.Command {
	var write bool
	var since string
	c := &cobra.Command{
		Use:   "history [--write] [--since YYYY-MM-DD]",
		Short: "Quét transcript Claude Code để nạp lịch sử làm việc vào sổ", //znf:allow-lang
		Long: "Quét ~/.claude/projects/*/*.jsonl và */subagents/agent-*.jsonl, gom theo (repo, slug) của\n" + //znf:allow-lang
			"worktree trong workspace và in bảng xem trước. Với --write, append một dòng history cho mỗi\n" + //znf:allow-lang
			"nhóm chưa có; chạy lại không tạo trùng. Preview chỉ đọc nên chạy được cả khi feature tắt.", //znf:allow-lang
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			w := cmd.OutOrStdout()
			var sinceT time.Time
			if since != "" {
				t, err := time.Parse("2006-01-02", since)
				if err != nil {
					return badTrack("--since must be YYYY-MM-DD: %v", err)
				}
				sinceT = t
			}
			wsDir, on, err := trackWorkspace(*ws)
			if err != nil {
				return err
			}
			if write && !on {
				_, err = fmt.Fprintln(w, "track: off")
				return err
			}
			home, err := os.UserHomeDir()
			if err != nil {
				return exitcode.New(exitcode.Fail, err)
			}
			groups, skipped, err := scanHistory(filepath.Join(home, ".claude", "projects"), wsDir, sinceT)
			if err != nil {
				return exitcode.New(exitcode.Fail, err)
			}
			_, _ = fmt.Fprintln(w, "repo  slug  kind  agent  tokens")
			for _, g := range groups {
				_, _ = fmt.Fprintf(w, "%s  %s  %s  %dm  %s\n", g.repo, g.slug, g.kind(), g.minutes(), g.tokensField())
			}
			_, _ = fmt.Fprintf(w, "skipped %d\n", skipped)
			if !write {
				return nil
			}
			return writeHistory(w, trackLogPath(wsDir), groups)
		},
	}
	c.Flags().BoolVar(&write, "write", false, "append the history rows (default: preview only)")
	c.Flags().StringVar(&since, "since", "", "ignore records before this day, YYYY-MM-DD")
	return c
}

func histID(repo, slug string) string {
	h := sha1.Sum([]byte(repo + "|" + slug)) //nolint:gosec // G401 -- see import
	return hex.EncodeToString(h[:])[:8]
}

func writeHistory(w io.Writer, logPath string, groups []*histGroup) error {
	rows, err := readTrackRows(logPath)
	if err != nil {
		return err
	}
	// Oldest work first: the rows share one write time, and the scale takes the last rows
	// of the log as the most recent history (FR-14.5).
	byLast := slices.Clone(groups)
	sort.SliceStable(byLast, func(i, j int) bool { return byLast[i].lastAt().Before(byLast[j].lastAt()) })
	n := 0
	for _, g := range byLast {
		id := histID(g.repo, g.slug)
		if rowsHave(rows, "history", id) {
			continue
		}
		if err := validTrackText("repo", g.repo); err != nil {
			return err
		}
		if err := appendTrackRow(logPath, "history", []string{id, g.kind(), g.repo, fmt.Sprint(g.minutes()), g.tokensField()}); err != nil {
			return err
		}
		n++
	}
	_, err = fmt.Fprintf(w, "wrote %d history row(s)\n", n)
	return err
}

type histSpan struct{ from, to time.Time }

type histGroup struct {
	repo, slug string
	skills     map[string]int
	branchKind string
	spans      []histSpan
	waits      []histSpan
	tokens     map[string][2]int
}

func (g *histGroup) kind() string {
	best, bestN := "", 0
	for _, s := range []string{"znf:cook", "znf:fix", "znf:hotfix"} {
		if g.skills[s] > bestN {
			best, bestN = histSkills[s], g.skills[s]
		}
	}
	switch {
	case best != "":
		return best
	case g.branchKind != "":
		return g.branchKind
	}
	return "-"
}

// lastAt is the end of the group's latest span.
func (g *histGroup) lastAt() time.Time {
	var last time.Time
	for _, s := range g.spans {
		if s.to.After(last) {
			last = s.to
		}
	}
	return last
}

// minutes is the union of the prompt spans minus the waits, rounded up.
func (g *histGroup) minutes() int {
	var total time.Duration
	for _, s := range subtractSpans(mergeSpans(g.spans), mergeSpans(g.waits)) {
		total += s.to.Sub(s.from)
	}
	return int((total + time.Minute - 1) / time.Minute)
}

func (g *histGroup) tokensField() string {
	if len(g.tokens) == 0 {
		return "-"
	}
	models := make([]string, 0, len(g.tokens))
	for m := range g.tokens {
		models = append(models, m)
	}
	sort.Strings(models)
	parts := make([]string, len(models))
	for i, m := range models {
		parts[i] = fmt.Sprintf("%s:%d/%d", m, g.tokens[m][0], g.tokens[m][1])
	}
	return strings.Join(parts, ",")
}

func mergeSpans(in []histSpan) []histSpan {
	s := append([]histSpan(nil), in...)
	sort.Slice(s, func(i, j int) bool { return s[i].from.Before(s[j].from) })
	var out []histSpan
	for _, x := range s {
		if n := len(out); n > 0 && !x.from.After(out[n-1].to) {
			if x.to.After(out[n-1].to) {
				out[n-1].to = x.to
			}
			continue
		}
		out = append(out, x)
	}
	return out
}

// subtractSpans removes the (merged) cut spans from the (merged) base spans.
func subtractSpans(base, cut []histSpan) []histSpan {
	var out []histSpan
	for _, b := range base {
		cur := b.from
		for _, c := range cut {
			if !c.to.After(cur) || !c.from.Before(b.to) {
				continue
			}
			if c.from.After(cur) {
				out = append(out, histSpan{cur, c.from})
			}
			if c.to.After(cur) {
				cur = c.to
			}
		}
		if cur.Before(b.to) {
			out = append(out, histSpan{cur, b.to})
		}
	}
	return out
}

type histBlock struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	Name      string `json:"name"`
	ID        string `json:"id"`
	ToolUseID string `json:"tool_use_id"`
}

type histRecord struct {
	Type             string `json:"type"`
	Cwd              string `json:"cwd"`
	GitBranch        string `json:"gitBranch"`
	AttributionSkill string `json:"attributionSkill"`
	IsMeta           bool   `json:"isMeta"`
	Timestamp        string `json:"timestamp"`
	Message          struct {
		ID      string          `json:"id"`
		Model   string          `json:"model"`
		Content json.RawMessage `json:"content"`
		Usage   *struct {
			In  int `json:"input_tokens"`
			Out int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// blocks decodes message.content; a plain string becomes one text block.
func (r *histRecord) blocks() ([]histBlock, error) {
	c := r.Message.Content
	if len(c) == 0 || string(c) == "null" {
		return nil, nil
	}
	if c[0] == '"' {
		var s string
		if err := json.Unmarshal(c, &s); err != nil {
			return nil, err
		}
		return []histBlock{{Type: "text", Text: s}}, nil
	}
	var bs []histBlock
	if err := json.Unmarshal(c, &bs); err != nil {
		return nil, err
	}
	return bs, nil
}

// isHumanPrompt follows FR-14.3: a user record, not meta, without a tool_result
// block, whose text does not start with a system marker. Only the prefix is
// examined and no text is kept.
func (r *histRecord) isHumanPrompt(bs []histBlock) bool {
	if r.Type != "user" || r.IsMeta {
		return false
	}
	text := ""
	for _, b := range bs {
		if b.Type == "tool_result" {
			return false
		}
		if text == "" && b.Type == "text" {
			text = strings.TrimSpace(b.Text)
		}
	}
	for _, p := range histNotPrompt {
		if strings.HasPrefix(text, p) {
			return false
		}
	}
	return true
}

type histScan struct {
	ws, repos string
	since     time.Time
	groups    map[string]*histGroup
	seenMsg   map[string]bool
	skipped   int
}

// scanHistory reads every transcript under projects and returns the groups
// sorted by (repo, slug) and the number of unreadable lines.
func scanHistory(projects, wsDir string, since time.Time) ([]*histGroup, int, error) {
	s := &histScan{
		ws: wsDir, repos: filepath.Join(filepath.Clean(wsDir), "repos") + string(filepath.Separator),
		since: since, groups: map[string]*histGroup{}, seenMsg: map[string]bool{},
	}
	var files, subs []string
	for _, p := range []struct {
		pat string
		dst *[]string
	}{
		{filepath.Join(projects, "*", "*.jsonl"), &files},
		{filepath.Join(projects, "*", "*", "subagents", "agent-*.jsonl"), &subs},
	} {
		m, err := filepath.Glob(p.pat)
		if err != nil {
			return nil, 0, err
		}
		sort.Strings(m)
		*p.dst = m
	}
	for _, f := range files {
		if err := s.scanFile(f, false); err != nil {
			return nil, 0, err
		}
	}
	for _, f := range subs {
		if err := s.scanFile(f, true); err != nil {
			return nil, 0, err
		}
	}
	out := make([]*histGroup, 0, len(s.groups))
	for _, g := range s.groups {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].repo != out[j].repo {
			return out[i].repo < out[j].repo
		}
		return out[i].slug < out[j].slug
	})
	return out, s.skipped, nil
}

// groupOf maps a record to its (repo, slug) group; ok is false when the record
// is outside the workspace repos or not attributable to a feature.
func (s *histScan) groupOf(cwd, branch string) (g *histGroup, ok bool) {
	if cwd == "" {
		return nil, false
	}
	rel, found := strings.CutPrefix(filepath.Clean(cwd)+string(filepath.Separator), s.repos)
	if !found {
		return nil, false
	}
	parts := strings.Split(strings.Trim(rel, string(filepath.Separator)), string(filepath.Separator))
	repo := parts[0]
	var slug, bk string
	switch {
	case len(parts) >= 3 && parts[1] == ".worktrees":
		slug = parts[2]
	default:
		m := histBranchRe.FindStringSubmatch(branch)
		if m == nil {
			return nil, false
		}
		slug, bk = m[2], histKindOf[m[1]]
	}
	key := repo + "\x00" + slug
	g = s.groups[key]
	if g == nil {
		g = &histGroup{repo: repo, slug: slug, skills: map[string]int{}, tokens: map[string][2]int{}}
		s.groups[key] = g
	}
	if g.branchKind == "" {
		if bk != "" {
			g.branchKind = bk
		} else if m := histBranchRe.FindStringSubmatch(branch); m != nil {
			g.branchKind = histKindOf[m[1]]
		}
	}
	return g, true
}

func (s *histScan) scanFile(path string, subagent bool) error {
	f, err := os.Open(path) //nolint:gosec // G304 -- listed by glob under ~/.claude/projects
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // read-only
	var (
		curFrom time.Time
		inPrmpt bool
		lastA   = map[*histGroup]time.Time{} // last assistant record per group since the prompt
		pending = map[string]struct {
			t time.Time
			g *histGroup
		}{}
	)
	// closeCur credits [prompt, last assistant record of g] to every group g that
	// answered the prompt, whichever group the prompt itself was typed in.
	closeCur := func() {
		if inPrmpt {
			for g, t := range lastA {
				if t.After(curFrom) {
					g.spans = append(g.spans, histSpan{curFrom, t})
				}
			}
		}
		inPrmpt, lastA = false, map[*histGroup]time.Time{}
	}
	rd := bufio.NewReaderSize(f, 1<<20)
	for line := 0; ; line++ {
		b, rerr := rd.ReadBytes('\n')
		if len(strings.TrimSpace(string(b))) > 0 {
			var r histRecord
			if err := json.Unmarshal(b, &r); err != nil {
				s.skipped++
			} else if t, ok := s.recordTime(&r); ok {
				bs, err := r.blocks()
				if err != nil {
					s.skipped++
				} else {
					g, in := s.groupOf(r.Cwd, r.GitBranch)
					if !in {
						g = nil
					}
					forced := subagent && line == 0
					if forced || r.isHumanPrompt(bs) {
						closeCur()
						inPrmpt, curFrom = true, t
					}
					if g != nil {
						if _, ok := histSkills[r.AttributionSkill]; ok {
							g.skills[r.AttributionSkill]++
						}
					}
					switch r.Type {
					case "assistant":
						if inPrmpt && g != nil {
							lastA[g] = t
						}
						for _, b := range bs {
							if b.Type == "tool_use" && b.Name == "AskUserQuestion" && g != nil {
								pending[b.ID] = struct {
									t time.Time
									g *histGroup
								}{t, g}
							}
						}
						s.addTokens(&r, g)
					case "user":
						for _, b := range bs {
							if p, ok := pending[b.ToolUseID]; ok && b.Type == "tool_result" {
								p.g.waits = append(p.g.waits, histSpan{p.t, t})
								delete(pending, b.ToolUseID)
							}
						}
					}
				}
			}
		}
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			return fmt.Errorf("read %s: %w", path, rerr)
		}
	}
	closeCur()
	return nil
}

// recordTime returns the record time; false drops the record (no timestamp, or
// before --since). A cwd record with a bad timestamp counts as skipped.
func (s *histScan) recordTime(r *histRecord) (time.Time, bool) {
	if r.Timestamp == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, r.Timestamp)
	if err != nil {
		s.skipped++
		return time.Time{}, false
	}
	if !s.since.IsZero() && t.Before(s.since) {
		return time.Time{}, false
	}
	return t, true
}

func (s *histScan) addTokens(r *histRecord, g *histGroup) {
	m := r.Message
	if g == nil || m.ID == "" || m.Usage == nil || s.seenMsg[m.ID] {
		return
	}
	s.seenMsg[m.ID] = true
	model := m.Model
	if model == "" {
		model = "unknown"
	}
	t := g.tokens[model]
	g.tokens[model] = [2]int{t[0] + m.Usage.In, t[1] + m.Usage.Out}
}
