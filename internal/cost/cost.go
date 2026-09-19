// Package cost reads Claude Code session transcripts (the JSONL files under
// ~/.claude/projects/<slug>/) and aggregates where tokens go: per-session
// totals, context size per turn, the subagent share, model mix, and the
// Skill/Agent tool calls that drove the spend.
//
// Two numbers here exist nowhere else (surveyed 2026-09-18: ccusage,
// Claude-Code-Usage-Monitor, native OTel): context size per assistant turn
// (cache_read + cache_creation + input, the weight paid on every turn) and the
// roll-up of <session>/subagents/agent-*.jsonl into the parent session.
package cost

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Usage is the four token counters Claude Code writes on every assistant turn.
type Usage struct {
	Input         int64 `json:"input"`
	CacheCreation int64 `json:"cache_creation"`
	CacheRead     int64 `json:"cache_read"`
	Output        int64 `json:"output"`
	Turns         int   `json:"turns"`
}

// Total is every counter summed — the volume figure used for ratios.
func (u Usage) Total() int64 { return u.Input + u.CacheCreation + u.CacheRead + u.Output }

func (u *Usage) add(o rawUsage) {
	u.Input += o.InputTokens
	u.CacheCreation += o.CacheCreationInputTokens
	u.CacheRead += o.CacheReadInputTokens
	u.Output += o.OutputTokens
	u.Turns++
}

// Session is one main transcript plus every subagent transcript spawned under it.
type Session struct {
	ID        string           `json:"id"`
	Main      Usage            `json:"main"`
	Sub       Usage            `json:"sub"`
	Agents    int              `json:"agents"`
	Skills    int              `json:"skills"`
	SkillsBy  map[string]int   `json:"skills_by,omitempty"`
	SkillTok  map[string]Usage `json:"skill_tok,omitempty"`
	First     time.Time        `json:"first"`
	Last      time.Time        `json:"last"`
	ctx       []int64          // per-turn context sizes (main only)
	reads     map[string]int   // Read file_path → count
	largeRes  []int            // tool_result sizes above the threshold
	models    map[string]int64 // main model → tokens
	subModels map[string]int64 // subagent model → tokens
	seen      map[string]bool  // message.id đã cộng usage (dedupe khối content lặp theo apiBlockIndex) //znf:allow-lang
}

// Total is main + subagent volume.
func (s *Session) Total() int64 { return s.Main.Total() + s.Sub.Total() }

// TopSkill is the most-invoked skill in the session, "-" when none.
func (s *Session) TopSkill() string {
	best, n := "-", 0
	for k, v := range s.SkillsBy {
		if v > n || (v == n && k < best) {
			best, n = k, v
		}
	}
	return best
}

// Report is the aggregate over every session in scope.
type Report struct {
	Root         string    `json:"root"`
	Since        time.Time `json:"since"`
	Sessions     int       `json:"sessions"`
	SubagentRuns int       `json:"subagent_runs"`
	Lines        int       `json:"lines"`
	First        time.Time `json:"first"`
	Last         time.Time `json:"last"`

	Main Usage `json:"main"`
	Sub  Usage `json:"sub"`

	CtxMedian int64 `json:"ctx_median"`
	CtxP90    int64 `json:"ctx_p90"`
	CtxN      int   `json:"ctx_n"`

	MainModels map[string]int64 `json:"main_models"`
	SubModels  map[string]int64 `json:"sub_models"`

	AgentDispatches int            `json:"agent_dispatches"`
	AgentsByType    map[string]int `json:"agents_by_type"`
	AgentsByModel   map[string]int `json:"agents_by_model"`
	AgentsNoModel   int            `json:"agents_no_model"`

	SkillCalls int              `json:"skill_calls"`
	SkillsBy   map[string]int   `json:"skills_by"`
	SkillTok   map[string]Usage `json:"skill_tok"`

	Top        []SessionRow `json:"top"`
	DupReads   []DupRead    `json:"dup_reads"`
	LargeRes   []LargeRes   `json:"large_results"`
	IdleAgents []IdleAgent  `json:"idle_agents"`
}

// SessionRow is the per-session line of the top table.
type SessionRow struct {
	ID       string `json:"id"`
	Tokens   int64  `json:"tokens"`
	Turns    int    `json:"turns"`
	Agents   int    `json:"agents"`
	Skills   int    `json:"skills"`
	TopSkill string `json:"top_skill"`
}

// DupRead is a file path Read repeatedly inside one session.
type DupRead struct {
	Session string `json:"session"`
	Path    string `json:"path"`
	Count   int    `json:"count"`
}

// LargeRes is a tool_result whose serialized content exceeded LargeResultBytes.
type LargeRes struct {
	Session string `json:"session"`
	Bytes   int    `json:"bytes"`
}

// IdleAgent is a subagent transcript whose whole text output was under
// IdleTextChars — the "went idle without a report" pattern.
type IdleAgent struct {
	Session string `json:"session"`
	Agent   string `json:"agent"`
	Type    string `json:"type,omitempty"`
	Desc    string `json:"description,omitempty"`
}

// Thresholds. Exported so the CLI help can print them and tests can pin them.
const (
	LargeResultBytes = 50_000
	DupReadMin       = 5
	IdleTextChars    = 50
	TopSessions      = 10
)

// NoSkillKey là khóa bucket cho token main-session không thuộc skill nào. //znf:allow-lang
const NoSkillKey = "(no skill)"

// Options narrows the scan.
type Options struct {
	Since time.Time // zero → no lower bound
	Top   int       // rows in the top table; 0 → TopSessions
}

// ProjectSlug converts an absolute project path to the directory name Claude
// Code uses under ~/.claude/projects (every path separator becomes '-').
func ProjectSlug(abs string) string {
	return strings.ReplaceAll(filepath.ToSlash(abs), "/", "-")
}

// rawUsage mirrors message.usage in the transcript.
type rawUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
}

type rawContent struct {
	Type  string          `json:"type"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
	Text  string          `json:"text"`
	// tool_result content: string or array — keep raw to size it
	Content json.RawMessage `json:"content"`
}

type rawMessage struct {
	ID      string          `json:"id"`
	Model   string          `json:"model"`
	Usage   *rawUsage       `json:"usage"`
	Content json.RawMessage `json:"content"`
}

type rawLine struct {
	Type             string     `json:"type"`
	Timestamp        string     `json:"timestamp"`
	AttributionSkill string     `json:"attributionSkill"`
	Message          rawMessage `json:"message"`
}

type agentInput struct {
	SubagentType string `json:"subagent_type"`
	Model        string `json:"model"`
}

type skillInput struct {
	Skill string `json:"skill"`
}

type readInput struct {
	FilePath string `json:"file_path"`
}

type agentMeta struct {
	AgentType   string `json:"agentType"`
	Description string `json:"description"`
}

// Scan walks root (a ~/.claude/projects/<slug> directory) and aggregates every
// main transcript (*.jsonl at the top level) plus its subagent transcripts
// (<session>/subagents/agent-*.jsonl). Lines are filtered by their own
// timestamp, never by file mtime, so a long-lived session contributes only
// the turns inside the window.
func Scan(root string, opt Options) (*Report, error) {
	mains, err := filepath.Glob(filepath.Join(root, "*.jsonl"))
	if err != nil {
		return nil, err
	}
	if len(mains) == 0 {
		return nil, errors.New("no transcripts under " + root)
	}
	if opt.Top <= 0 {
		opt.Top = TopSessions
	}
	r := &Report{
		Root: root, Since: opt.Since,
		MainModels: map[string]int64{}, SubModels: map[string]int64{},
		AgentsByType: map[string]int{}, AgentsByModel: map[string]int{},
		SkillsBy: map[string]int{}, SkillTok: map[string]Usage{},
	}
	sessions := map[string]*Session{}
	get := func(id string) *Session {
		s, ok := sessions[id]
		if !ok {
			s = &Session{ID: id, SkillsBy: map[string]int{}, reads: map[string]int{},
				models: map[string]int64{}, subModels: map[string]int64{}, seen: map[string]bool{},
				SkillTok: map[string]Usage{}}
			sessions[id] = s
		}
		return s
	}

	for _, path := range mains {
		id := strings.TrimSuffix(filepath.Base(path), ".jsonl")
		s := get(id)
		n, err := scanMain(path, s, r, opt.Since)
		if err != nil {
			return nil, err
		}
		r.Lines += n

		subs, _ := filepath.Glob(filepath.Join(root, id, "subagents", "agent-*.jsonl"))
		for _, sp := range subs {
			n, turns, idle, err := scanSub(sp, s, r, opt.Since)
			if err != nil {
				return nil, err
			}
			r.Lines += n
			if turns == 0 {
				continue // every turn of this run is outside the window
			}
			r.SubagentRuns++
			if idle {
				ia := IdleAgent{Session: id, Agent: strings.TrimSuffix(filepath.Base(sp), ".jsonl")}
				if b, err := os.ReadFile(strings.TrimSuffix(sp, ".jsonl") + ".meta.json"); err == nil { //nolint:gosec // G304 -- sibling of a transcript we already opened
					var m agentMeta
					if json.Unmarshal(b, &m) == nil {
						ia.Type, ia.Desc = m.AgentType, m.Description
					}
				}
				r.IdleAgents = append(r.IdleAgents, ia)
			}
		}
	}

	// Sessions with no turn inside the window are dropped from every count.
	var allCtx []int64
	for id, s := range sessions {
		if s.Main.Turns == 0 && s.Sub.Turns == 0 {
			delete(sessions, id)
			continue
		}
		r.Sessions++
		allCtx = append(allCtx, s.ctx...)
		addUsage(&r.Main, s.Main)
		addUsage(&r.Sub, s.Sub)
		for k, v := range s.SkillTok {
			b := r.SkillTok[k]
			addUsage(&b, v)
			r.SkillTok[k] = b
		}
		r.AgentDispatches += s.Agents
		r.SkillCalls += s.Skills
		for k, v := range s.models {
			r.MainModels[k] += v
		}
		for k, v := range s.subModels {
			r.SubModels[k] += v
		}
		if r.First.IsZero() || (!s.First.IsZero() && s.First.Before(r.First)) {
			r.First = s.First
		}
		if s.Last.After(r.Last) {
			r.Last = s.Last
		}
		for p, c := range s.reads {
			if c >= DupReadMin {
				r.DupReads = append(r.DupReads, DupRead{Session: id, Path: p, Count: c})
			}
		}
		for _, b := range s.largeRes {
			r.LargeRes = append(r.LargeRes, LargeRes{Session: id, Bytes: b})
		}
	}

	sort.Slice(allCtx, func(i, j int) bool { return allCtx[i] < allCtx[j] })
	r.CtxN = len(allCtx)
	r.CtxMedian = percentile(allCtx, 0.5)
	r.CtxP90 = percentile(allCtx, 0.9)

	rows := make([]SessionRow, 0, len(sessions))
	for _, s := range sessions {
		rows = append(rows, SessionRow{ID: s.ID, Tokens: s.Total(), Turns: s.Main.Turns + s.Sub.Turns,
			Agents: s.Agents, Skills: s.Skills, TopSkill: s.TopSkill()})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Tokens != rows[j].Tokens {
			return rows[i].Tokens > rows[j].Tokens
		}
		return rows[i].ID < rows[j].ID
	})
	if len(rows) > opt.Top {
		rows = rows[:opt.Top]
	}
	r.Top = rows
	sort.Slice(r.DupReads, func(i, j int) bool {
		if r.DupReads[i].Count != r.DupReads[j].Count {
			return r.DupReads[i].Count > r.DupReads[j].Count
		}
		return r.DupReads[i].Path < r.DupReads[j].Path
	})
	sort.Slice(r.LargeRes, func(i, j int) bool { return r.LargeRes[i].Bytes > r.LargeRes[j].Bytes })
	return r, nil
}

func addUsage(dst *Usage, src Usage) {
	dst.Input += src.Input
	dst.CacheCreation += src.CacheCreation
	dst.CacheRead += src.CacheRead
	dst.Output += src.Output
	dst.Turns += src.Turns
}

// percentile is nearest-rank on a sorted slice; 0 on empty input.
func percentile(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	k := int(float64(len(sorted)-1) * p)
	return sorted[k]
}

func inWindow(ts string, since time.Time) (time.Time, bool) {
	if ts == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return time.Time{}, false
	}
	if !since.IsZero() && t.Before(since) {
		return t, false
	}
	return t, true
}

func eachLine(path string, fn func(rawLine)) (int, error) {
	f, err := os.Open(path) //nolint:gosec // G304 -- path comes from a Glob under the caller's projects dir
	if err != nil {
		return 0, err
	}
	defer f.Close() //nolint:errcheck // read-only
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20) // tool results can be multi-MB on one line
	n := 0
	for sc.Scan() {
		n++
		var l rawLine
		if json.Unmarshal(sc.Bytes(), &l) != nil {
			continue // a torn or foreign line never aborts the scan
		}
		fn(l)
	}
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
		return n, err
	}
	return n, nil
}

func contentItems(raw json.RawMessage) []rawContent {
	var items []rawContent
	if len(raw) == 0 || raw[0] != '[' {
		return nil
	}
	_ = json.Unmarshal(raw, &items)
	return items
}

func scanMain(path string, s *Session, r *Report, since time.Time) (int, error) {
	return eachLine(path, func(l rawLine) {
		t, ok := inWindow(l.Timestamp, since)
		if !ok {
			return
		}
		if s.First.IsZero() || t.Before(s.First) {
			s.First = t
		}
		if t.After(s.Last) {
			s.Last = t
		}
		switch l.Type {
		case "assistant":
			if u := l.Message.Usage; u != nil {
				id := l.Message.ID
				if id == "" || !s.seen[id] {
					if id != "" {
						s.seen[id] = true
					}
					s.Main.add(*u)
					tot := u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens + u.OutputTokens
					// "<synthetic>" rows (harness-injected, zero usage) would only add noise.
					if l.Message.Model != "" && tot > 0 {
						s.models[l.Message.Model] += tot
					}
					s.ctx = append(s.ctx, u.CacheReadInputTokens+u.CacheCreationInputTokens+u.InputTokens)
					sk := l.AttributionSkill
					if sk == "" {
						sk = NoSkillKey
					}
					b := s.SkillTok[sk]
					b.add(*u)
					s.SkillTok[sk] = b
				}
			}
			for _, it := range contentItems(l.Message.Content) {
				if it.Type != "tool_use" {
					continue
				}
				switch it.Name {
				case "Agent", "Task":
					s.Agents++
					var in agentInput
					_ = json.Unmarshal(it.Input, &in)
					typ := in.SubagentType
					if typ == "" {
						typ = "general-purpose(default)"
					}
					r.AgentsByType[typ]++
					if in.Model == "" {
						r.AgentsNoModel++
					} else {
						r.AgentsByModel[in.Model]++
					}
				case "Skill":
					s.Skills++
					var in skillInput
					_ = json.Unmarshal(it.Input, &in)
					name := in.Skill
					if name == "" {
						name = "(unknown)"
					}
					s.SkillsBy[name]++
					r.SkillsBy[name]++
				case "Read":
					var in readInput
					_ = json.Unmarshal(it.Input, &in)
					if in.FilePath != "" {
						s.reads[in.FilePath]++
					}
				}
			}
		case "user":
			for _, it := range contentItems(l.Message.Content) {
				if it.Type == "tool_result" && len(it.Content) > LargeResultBytes {
					s.largeRes = append(s.largeRes, len(it.Content))
				}
			}
		}
	})
}

// scanSub returns lines read, assistant turns inside the window, and whether
// the run ended without producing text (an idle agent).
func scanSub(path string, s *Session, _ *Report, since time.Time) (int, int, bool, error) {
	text, turns := 0, 0
	n, err := eachLine(path, func(l rawLine) {
		if l.Type != "assistant" {
			return
		}
		if _, ok := inWindow(l.Timestamp, since); !ok {
			return
		}
		turns++
		if u := l.Message.Usage; u != nil {
			id := l.Message.ID
			if id == "" || !s.seen[id] {
				if id != "" {
					s.seen[id] = true
				}
				s.Sub.add(*u)
				if tot := u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens + u.OutputTokens; l.Message.Model != "" && tot > 0 {
					s.subModels[l.Message.Model] += tot
				}
			}
		}
		for _, it := range contentItems(l.Message.Content) {
			if it.Type == "text" {
				text += len(it.Text)
			}
		}
	})
	if err != nil {
		return n, 0, false, err
	}
	return n, turns, turns > 0 && text < IdleTextChars, nil
}
