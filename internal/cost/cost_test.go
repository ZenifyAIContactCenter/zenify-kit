package cost

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func line(t *testing.T, typ, ts string, msg map[string]any) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{"type": typ, "timestamp": ts, "message": msg})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// lineSkill dựng một dòng assistant có attributionSkill ở TOP LEVEL (không nằm trong message).
func lineSkill(t *testing.T, ts, skill string, msg map[string]any) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{"type": "assistant", "timestamp": ts, "attributionSkill": skill, "message": msg})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func usage(in, cc, cr, out int64) map[string]any {
	return map[string]any{"input_tokens": in, "cache_creation_input_tokens": cc, "cache_read_input_tokens": cr, "output_tokens": out}
}

func write(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fixture: one session with 3 main turns (one outside the window), one Agent
// dispatch without model, one Skill call, MEMORY.md read 5×, one large
// tool_result, one subagent with two turns and an empty text output.
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	sid := "aaaaaaaa-0000-0000-0000-000000000001"
	old := "2026-09-01T00:00:00.000Z"
	now := "2026-09-18T10:00:00.000Z"
	reads := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		reads = append(reads, line(t, "assistant", now, map[string]any{
			"model": "claude-opus-4-8", "usage": usage(10, 0, 100, 5),
			"content": []any{map[string]any{"type": "tool_use", "name": "Read", "input": map[string]any{"file_path": "/w/MEMORY.md"}}},
		}))
	}
	lines := []string{
		`{"type":"ai-title","aiTitle":"x"}`,
		`not json at all`,
		line(t, "assistant", old, map[string]any{"model": "claude-opus-4-8", "usage": usage(1, 1, 1, 1)}),
		line(t, "assistant", now, map[string]any{
			"model": "claude-opus-4-8", "usage": usage(100, 1000, 180_000, 500),
			"content": []any{
				map[string]any{"type": "tool_use", "name": "Agent", "input": map[string]any{"subagent_type": "znf:code-reviewer", "description": "review"}},
				map[string]any{"type": "tool_use", "name": "Skill", "input": map[string]any{"skill": "znf:ground"}},
			},
		}),
		line(t, "assistant", now, map[string]any{
			"model": "claude-opus-4-8", "usage": usage(50, 0, 200_000, 100),
			"content": []any{map[string]any{"type": "tool_use", "name": "Agent", "input": map[string]any{"subagent_type": "Explore", "model": "sonnet"}}},
		}),
		line(t, "user", now, map[string]any{"content": []any{map[string]any{"type": "tool_result", "content": strings.Repeat("x", LargeResultBytes+10)}}}),
	}
	lines = append(lines, reads...)
	write(t, filepath.Join(root, sid+".jsonl"), lines...)
	write(t, filepath.Join(root, sid, "subagents", "agent-a1.jsonl"),
		line(t, "assistant", now, map[string]any{"model": "claude-sonnet-5", "usage": usage(20, 300, 5000, 40), "content": []any{map[string]any{"type": "text", "text": "ok"}}}),
		line(t, "assistant", now, map[string]any{"model": "claude-sonnet-5", "usage": usage(20, 0, 6000, 10)}),
	)
	write(t, filepath.Join(root, sid, "subagents", "agent-a1.meta.json"), `{"agentType":"znf:code-reviewer","description":"review"}`)
	// a subagent run whose every turn predates the window must not be counted
	write(t, filepath.Join(root, sid, "subagents", "agent-a0.jsonl"),
		line(t, "assistant", old, map[string]any{"model": "claude-sonnet-5", "usage": usage(1, 1, 1, 1),
			"content": []any{map[string]any{"type": "text", "text": strings.Repeat("report ", 20)}}}))
	// a second session entirely outside the window must vanish from every count
	write(t, filepath.Join(root, "bbbbbbbb-0000-0000-0000-000000000002.jsonl"),
		line(t, "assistant", old, map[string]any{"model": "claude-opus-4-8", "usage": usage(9, 9, 9, 9)}))
	return root
}

func TestScan_TotalsFilterByLineTimestamp(t *testing.T) {
	r, err := Scan(fixture(t), Options{Since: time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if r.Sessions != 1 {
		t.Fatalf("sessions = %d, want 1 (old-only session dropped)", r.Sessions)
	}
	// main: 2 big turns + 5 read turns; the old turn is excluded
	if r.Main.Turns != 7 {
		t.Fatalf("main turns = %d, want 7", r.Main.Turns)
	}
	wantIn := int64(100 + 50 + 5*10)
	if r.Main.Input != wantIn {
		t.Fatalf("main input = %d, want %d", r.Main.Input, wantIn)
	}
	if r.Sub.Turns != 2 || r.Sub.CacheRead != 11_000 {
		t.Fatalf("sub = %+v, want 2 turns / 11000 cache_read", r.Sub)
	}
	if r.SubagentRuns != 1 {
		t.Fatalf("subagent runs = %d, want 1 (old-only run dropped)", r.SubagentRuns)
	}
	if got := r.MainModels["claude-opus-4-8"]; got == 0 {
		t.Fatalf("main model mix missing opus: %v", r.MainModels)
	}
	if got := r.SubModels["claude-sonnet-5"]; got != 20+300+5000+40+20+6000+10 {
		t.Fatalf("sub model tokens = %d", got)
	}
}

func TestScan_ContextPerTurnAndDispatchCounts(t *testing.T) {
	r, err := Scan(fixture(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	// ctx sizes (no window): old 3, old-session 27, 181100, 200050, five × 110 → sorted:
	// 3,27,110×5,181100,200050 (n=9) → median index 4 (=110), p90 index 7
	if r.CtxN != 9 || r.CtxMedian != 110 || r.CtxP90 != 181_100 {
		t.Fatalf("ctx n/median/p90 = %d/%d/%d", r.CtxN, r.CtxMedian, r.CtxP90)
	}
	if r.SubagentRuns != 2 {
		t.Fatalf("subagent runs without window = %d, want 2", r.SubagentRuns)
	}
	if r.AgentDispatches != 2 || r.AgentsNoModel != 1 || r.AgentsByModel["sonnet"] != 1 {
		t.Fatalf("agents = %d noModel=%d byModel=%v", r.AgentDispatches, r.AgentsNoModel, r.AgentsByModel)
	}
	if r.AgentsByType["znf:code-reviewer"] != 1 || r.AgentsByType["Explore"] != 1 {
		t.Fatalf("agents by type = %v", r.AgentsByType)
	}
	if r.SkillCalls != 1 || r.SkillsBy["znf:ground"] != 1 {
		t.Fatalf("skills = %d %v", r.SkillCalls, r.SkillsBy)
	}
	if len(r.Top) != 2 || r.Top[0].TopSkill != "znf:ground" || r.Top[0].Agents != 2 {
		t.Fatalf("top = %+v", r.Top)
	}
}

func TestScan_WasteSignals(t *testing.T) {
	r, err := Scan(fixture(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.DupReads) != 1 || r.DupReads[0].Path != "/w/MEMORY.md" || r.DupReads[0].Count != 5 {
		t.Fatalf("dup reads = %+v", r.DupReads)
	}
	if len(r.LargeRes) != 1 || r.LargeRes[0].Bytes <= LargeResultBytes {
		t.Fatalf("large results = %+v", r.LargeRes)
	}
	if len(r.IdleAgents) != 1 || r.IdleAgents[0].Type != "znf:code-reviewer" || r.IdleAgents[0].Agent != "agent-a1" {
		t.Fatalf("idle agents = %+v", r.IdleAgents)
	}
}

func TestScan_NoTranscriptsIsAnError(t *testing.T) {
	if _, err := Scan(t.TempDir(), Options{}); err == nil {
		t.Fatal("expected an error on an empty root")
	}
}

func TestScan_DedupesUsageByMessageID(t *testing.T) {
	root := t.TempDir()
	sid := "cccccccc-0000-0000-0000-000000000003"
	now := "2026-09-18T10:00:00.000Z"
	// một message.id trên 3 dòng (3 content block), usage lặp y hệt; một dòng mang block Skill
	msg := func(content []any) map[string]any {
		return map[string]any{"id": "msg_dup", "model": "claude-opus-4-8", "usage": usage(100, 0, 1000, 50), "content": content}
	}
	write(t, filepath.Join(root, sid+".jsonl"),
		line(t, "assistant", now, msg([]any{map[string]any{"type": "thinking"}})),
		line(t, "assistant", now, msg([]any{map[string]any{"type": "tool_use", "name": "Skill", "input": map[string]any{"skill": "znf:cook"}}})),
		line(t, "assistant", now, msg([]any{map[string]any{"type": "text", "text": "done"}})),
	)
	r, err := Scan(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Main.Turns != 1 {
		t.Fatalf("main turns = %d, want 1 (deduped by message.id)", r.Main.Turns)
	}
	if got := r.Main.Total(); got != 1150 {
		t.Fatalf("main total = %d, want 1150 (usage counted once)", got)
	}
	if r.SkillsBy["znf:cook"] != 1 {
		t.Fatalf("skill calls = %d, want 1 (content block counted, NOT deduped away)", r.SkillsBy["znf:cook"])
	}
}

func TestScan_SkillTokBucketsByAttribution(t *testing.T) {
	root := t.TempDir()
	sid := "dddddddd-0000-0000-0000-000000000004"
	now := "2026-09-18T10:00:00.000Z"
	write(t, filepath.Join(root, sid+".jsonl"),
		lineSkill(t, now, "znf:cook", map[string]any{"id": "m1", "model": "claude-opus-4-8", "usage": usage(10, 0, 100, 5)}),
		lineSkill(t, now, "znf:ground", map[string]any{"id": "m2", "model": "claude-opus-4-8", "usage": usage(20, 0, 200, 10)}),
		line(t, "assistant", now, map[string]any{"id": "m3", "model": "claude-opus-4-8", "usage": usage(1, 0, 9, 0)}), // không attributionSkill
	)
	r, err := Scan(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := r.SkillTok["znf:cook"].Total(); got != 115 {
		t.Fatalf("cook = %d, want 115", got)
	}
	if got := r.SkillTok["znf:ground"].Total(); got != 230 {
		t.Fatalf("ground = %d, want 230", got)
	}
	if got := r.SkillTok[NoSkillKey].Total(); got != 10 {
		t.Fatalf("(no skill) = %d, want 10", got)
	}
	var sum int64
	for _, v := range r.SkillTok {
		sum += v.Total()
	}
	if sum != r.Main.Total() {
		t.Fatalf("sum(SkillTok)=%d != Main.Total()=%d", sum, r.Main.Total())
	}
}

func TestProjectSlug(t *testing.T) {
	if got := ProjectSlug("/Users/x/WorkingSpace/zenify"); got != "-Users-x-WorkingSpace-zenify" {
		t.Fatalf("slug = %q", got)
	}
}

func TestPercentile(t *testing.T) {
	if percentile(nil, 0.5) != 0 {
		t.Fatal("empty → 0")
	}
	s := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	if percentile(s, 0.5) != 5 || percentile(s, 0.9) != 9 {
		t.Fatalf("p50=%d p90=%d", percentile(s, 0.5), percentile(s, 0.9))
	}
}
