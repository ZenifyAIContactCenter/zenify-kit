package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/cost"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ui"
	"github.com/spf13/cobra"
)

var costNow = time.Now // seam for tests

// parseSince accepts "7d", "36h", "2w", or an RFC3339 date; "" → zero time.
func parseSince(s string, now time.Time) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	if len(s) >= 2 {
		n, err := strconv.Atoi(s[:len(s)-1])
		if err == nil && n > 0 {
			switch s[len(s)-1] {
			case 'h':
				return now.Add(-time.Duration(n) * time.Hour), nil
			case 'd':
				return now.Add(-time.Duration(n) * 24 * time.Hour), nil
			case 'w':
				return now.Add(-time.Duration(n) * 7 * 24 * time.Hour), nil
			}
		}
	}
	return time.Time{}, fmt.Errorf("--since %q: want <n>h|<n>d|<n>w or YYYY-MM-DD", s)
}

// costRoot resolves the ~/.claude/projects/<slug> directory for project.
func costRoot(home, project string) (string, error) {
	abs, err := filepath.Abs(project)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	root := filepath.Join(home, ".claude", "projects", cost.ProjectSlug(abs))
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return "", fmt.Errorf("no transcripts for %s (expected %s)", abs, root)
	}
	return root, nil
}

func humanTok(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.2fB", float64(n)/1e9)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	}
	return strconv.FormatInt(n, 10)
}

func pct(part, whole int64) string {
	if whole == 0 {
		return "0%"
	}
	return fmt.Sprintf("%.1f%%", 100*float64(part)/float64(whole))
}

func sortedKeys[V any](m map[string]V, less func(a, b string) bool) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool { return less(ks[i], ks[j]) })
	return ks
}

func mtokPerCall(total int64, calls int) string {
	if calls == 0 {
		return "—"
	}
	return fmt.Sprintf("%.3f", float64(total)/float64(calls)/1e6)
}

func bySkillRows(r *cost.Report) [][]string {
	keys := sortedKeys(r.SkillTok, func(a, b string) bool {
		if ta, tb := r.SkillTok[a].Total(), r.SkillTok[b].Total(); ta != tb {
			return ta > tb
		}
		return a < b
	})
	rows := make([][]string, 0, len(keys))
	for _, k := range keys {
		calls := r.SkillsBy[k]
		rows = append(rows, []string{k, strconv.Itoa(calls), humanTok(r.SkillTok[k].Total()), mtokPerCall(r.SkillTok[k].Total(), calls)})
	}
	return rows
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// renderCost prints the human report. Vietnamese labels are the convention for
// operator-facing CLI output in this repo (see observe report).
func renderCost(w io.Writer, r *cost.Report, bySkill bool) {
	u := ui.New(w)
	u.Header("zenify cost")
	window := "toàn bộ" //znf:allow-lang
	if !r.Since.IsZero() {
		window = "từ " + r.Since.UTC().Format("2006-01-02") //znf:allow-lang
	}
	u.KV([][2]string{
		{"Nguồn", r.Root},  //znf:allow-lang
		{"Cửa sổ", window}, //znf:allow-lang
		{"Session", strconv.Itoa(r.Sessions)},
		{"Subagent run", strconv.Itoa(r.SubagentRuns)},
		{"Dòng đã đọc", strconv.Itoa(r.Lines)}, //znf:allow-lang
	})

	grand := r.Main.Total() + r.Sub.Total()
	u.Section("Token (main | subagent)")                                //znf:allow-lang
	u.Table([]string{"LOẠI", "MAIN", "SUBAGENT", "% TỔNG"}, [][]string{ //znf:allow-lang
		{"input", humanTok(r.Main.Input), humanTok(r.Sub.Input), pct(r.Main.Input+r.Sub.Input, grand)},
		{"cache_creation", humanTok(r.Main.CacheCreation), humanTok(r.Sub.CacheCreation), pct(r.Main.CacheCreation+r.Sub.CacheCreation, grand)},
		{"cache_read", humanTok(r.Main.CacheRead), humanTok(r.Sub.CacheRead), pct(r.Main.CacheRead+r.Sub.CacheRead, grand)},
		{"output", humanTok(r.Main.Output), humanTok(r.Sub.Output), pct(r.Main.Output+r.Sub.Output, grand)},
		{"tổng", humanTok(r.Main.Total()), humanTok(r.Sub.Total()), "100%"}, //znf:allow-lang
		{"turn", strconv.Itoa(r.Main.Turns), strconv.Itoa(r.Sub.Turns), ""},
	})
	u.Note(fmt.Sprintf("Subagent chiếm %s tổng token. Context mỗi turn (main): median %s, p90 %s (n=%d).", //znf:allow-lang
		pct(r.Sub.Total(), grand), humanTok(r.CtxMedian), humanTok(r.CtxP90), r.CtxN))

	effs := sortedKeys(r.Efforts, func(a, b string) bool { return r.Efforts[a] > r.Efforts[b] })
	parts := make([]string, 0, len(effs))
	for _, k := range effs {
		parts = append(parts, fmt.Sprintf("%s(%d)", k, r.Efforts[k]))
	}
	if len(parts) == 0 {
		parts = append(parts, "-")
	}
	u.Note(fmt.Sprintf("Thinking token main: %s (%s output). Effort đã thấy: %s.", //znf:allow-lang
		humanTok(r.Main.Thinking), pct(r.Main.Thinking, r.Main.Output), strings.Join(parts, " ")))

	u.Section("Model") //znf:allow-lang
	rows := [][]string{}
	for _, k := range sortedKeys(r.MainModels, func(a, b string) bool { return r.MainModels[a] > r.MainModels[b] }) {
		rows = append(rows, []string{"main", k, humanTok(r.MainModels[k]), pct(r.MainModels[k], r.Main.Total())})
	}
	for _, k := range sortedKeys(r.SubModels, func(a, b string) bool { return r.SubModels[a] > r.SubModels[b] }) {
		rows = append(rows, []string{"subagent", k, humanTok(r.SubModels[k]), pct(r.SubModels[k], r.Sub.Total())})
	}
	u.Table([]string{"PHÍA", "MODEL", "TOKEN", "%"}, rows) //znf:allow-lang

	u.Section(fmt.Sprintf("Agent dispatch: %d (bỏ trống model: %d)", r.AgentDispatches, r.AgentsNoModel)) //znf:allow-lang
	rows = rows[:0]
	for _, k := range sortedKeys(r.AgentsByType, func(a, b string) bool { return r.AgentsByType[a] > r.AgentsByType[b] }) {
		rows = append(rows, []string{"type", k, strconv.Itoa(r.AgentsByType[k])})
	}
	for _, k := range sortedKeys(r.AgentsByModel, func(a, b string) bool { return r.AgentsByModel[a] > r.AgentsByModel[b] }) {
		rows = append(rows, []string{"model", k, strconv.Itoa(r.AgentsByModel[k])})
	}
	if len(rows) > 0 {
		u.Table([]string{"THEO", "GIÁ TRỊ", "SỐ"}, rows) //znf:allow-lang
	}

	if bySkill {
		u.Section(fmt.Sprintf("Skill theo token: %d lần gọi", r.SkillCalls)) //znf:allow-lang
		if br := bySkillRows(r); len(br) > 0 {
			u.Table([]string{"SKILL", "CALLS", "TOKEN", "MTOK/CALL"}, br)
		} else {
			u.Note("không có") //znf:allow-lang
		}
		u.Note("Ước lượng theo turn: token gán theo attributionSkill của mỗi message; skill lồng nhau tính riêng, không cộng dồn vào skill cha.") //znf:allow-lang
	} else {
		u.Section(fmt.Sprintf("Skill: %d lần gọi", r.SkillCalls)) //znf:allow-lang
		rows = rows[:0]
		for i, k := range sortedKeys(r.SkillsBy, func(a, b string) bool {
			if r.SkillsBy[a] != r.SkillsBy[b] {
				return r.SkillsBy[a] > r.SkillsBy[b]
			}
			return a < b
		}) {
			if i >= 20 {
				break
			}
			rows = append(rows, []string{k, strconv.Itoa(r.SkillsBy[k])})
		}
		if len(rows) > 0 {
			u.Table([]string{"SKILL", "SỐ"}, rows) //znf:allow-lang
		}
	}

	u.Section("Session nặng nhất (main + subagent)") //znf:allow-lang
	rows = rows[:0]
	for _, s := range r.Top {
		rows = append(rows, []string{short(s.ID), humanTok(s.Tokens), strconv.Itoa(s.Turns), strconv.Itoa(s.Agents), strconv.Itoa(s.Skills), s.TopSkill})
	}
	u.Table([]string{"SESSION", "TOKEN", "TURN", "AGENT", "SKILL", "SKILL CHÍNH"}, rows) //znf:allow-lang

	u.Section("Dấu hiệu lãng phí") //znf:allow-lang
	if len(r.DupReads) == 0 && len(r.LargeRes) == 0 && len(r.IdleAgents) == 0 {
		u.Note("không có") //znf:allow-lang
		return
	}
	rows = rows[:0]
	for i, d := range r.DupReads {
		if i >= 10 {
			break
		}
		rows = append(rows, []string{"đọc lặp", short(d.Session), fmt.Sprintf("%d× %s", d.Count, d.Path)}) //znf:allow-lang
	}
	for i, l := range r.LargeRes {
		if i >= 10 {
			break
		}
		rows = append(rows, []string{"tool result lớn", short(l.Session), humanBytes(int64(l.Bytes))}) //znf:allow-lang
	}
	for i, a := range r.IdleAgents {
		if i >= 10 {
			break
		}
		rows = append(rows, []string{"agent im lặng", short(a.Session), strings.TrimSpace(a.Agent + " " + a.Type + " " + a.Desc)}) //znf:allow-lang
	}
	u.Table([]string{"LOẠI", "SESSION", "CHI TIẾT"}, rows)                                            //znf:allow-lang
	u.Note(fmt.Sprintf("Ngưỡng: đọc lặp ≥ %d lần, tool result > %dKB, agent trả về < %d ký tự text.", //znf:allow-lang
		cost.DupReadMin, cost.LargeResultBytes/1000, cost.IdleTextChars))
}

// runCost is the testable core: resolves the transcript root, scans, renders.
func runCost(w io.Writer, home, project, since string, top int, asJSON, bySkill bool) error {
	sinceT, err := parseSince(since, costNow())
	if err != nil {
		return exitcode.New(exitcode.BadArgs, err)
	}
	root, err := costRoot(home, project)
	if err != nil {
		return exitcode.New(exitcode.BadArgs, err)
	}
	r, err := cost.Scan(root, cost.Options{Since: sinceT, Top: top})
	if err != nil {
		return exitcode.New(exitcode.Fail, err)
	}
	if asJSON {
		return writeJSON(w, r)
	}
	renderCost(w, r, bySkill)
	return nil
}

const costLong = "Đọc transcript Claude Code (~/.claude/projects/<slug>/*.jsonl và\n" + //znf:allow-lang
	"<session>/subagents/agent-*.jsonl) và cho biết token đi đâu: tổng theo bốn loại\n" + //znf:allow-lang
	"cho main và subagent, context mỗi turn (median/p90), model mix, số lần gọi\n" + //znf:allow-lang
	"Skill/Agent (kể cả dispatch bỏ trống model), session nặng nhất, và dấu hiệu\n" + //znf:allow-lang
	"lãng phí (đọc lặp một file, tool result lớn, agent im lặng không báo cáo).\n" + //znf:allow-lang
	"\n" +
	"Lọc theo timestamp từng dòng, không theo mtime file, nên một session sống lâu\n" + //znf:allow-lang
	"chỉ góp phần nằm trong cửa sổ. Chỉ đọc, không ghi gì." //znf:allow-lang

func newCostCmd() *cobra.Command {
	var (
		since   string
		project string
		top     int
		asJSON  bool
		bySkill bool
	)
	c := &cobra.Command{
		Use:   "cost",
		Short: "Token đi đâu: context/turn, subagent, model, skill — từ transcript local", //znf:allow-lang
		Long:  costLong,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return exitcode.New(exitcode.Fail, errors.New("cannot resolve home dir"))
			}
			if project == "" {
				project, _ = os.Getwd()
			}
			return runCost(cmd.OutOrStdout(), home, project, since, top, asJSON, bySkill)
		},
	}
	c.Flags().StringVar(&since, "since", "7d", "window: <n>h|<n>d|<n>w or YYYY-MM-DD; empty = all")
	c.Flags().StringVar(&project, "project", "", "project directory whose transcripts to read (default: cwd)")
	c.Flags().IntVar(&top, "top", cost.TopSessions, "rows in the heaviest-sessions table")
	c.Flags().BoolVar(&asJSON, "json", false, "output JSON instead of tables")
	c.Flags().BoolVar(&bySkill, "by-skill", false, "bảng token theo từng skill (calls, token, Mtok/lần)") //znf:allow-lang
	return c
}
