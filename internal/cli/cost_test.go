package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/cost"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	cases := map[string]time.Time{
		"":           {},
		"7d":         now.Add(-7 * 24 * time.Hour),
		"36h":        now.Add(-36 * time.Hour),
		"2w":         now.Add(-14 * 24 * time.Hour),
		"2026-09-01": time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	}
	for in, want := range cases {
		got, err := parseSince(in, now)
		if err != nil || !got.Equal(want) {
			t.Errorf("parseSince(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"7", "d7", "0d", "soon"} {
		if _, err := parseSince(bad, now); err == nil {
			t.Errorf("parseSince(%q) accepted", bad)
		}
	}
}

// fakeHome lays out ~/.claude/projects/<slug>/ for project with one tiny
// transcript, and returns (home, project).
func fakeHome(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	project := filepath.Join(t.TempDir(), "ws")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(project)
	root := filepath.Join(home, ".claude", "projects", cost.ProjectSlug(filepath.Clean(abs)))
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"assistant","timestamp":"2026-09-18T10:00:00.000Z","message":{"model":"claude-sonnet-5","usage":{"input_tokens":10,"cache_creation_input_tokens":20,"cache_read_input_tokens":3000,"output_tokens":40},"content":[{"type":"tool_use","name":"Skill","input":{"skill":"znf:ground"}}]}}` + "\n"
	if err := os.WriteFile(filepath.Join(root, "s1.jsonl"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	return home, project
}

func TestRunCost_HumanAndJSON(t *testing.T) {
	costNow = func() time.Time { return time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { costNow = time.Now })
	home, project := fakeHome(t)

	var out bytes.Buffer
	if err := runCost(&out, home, project, "7d", 5, false, false); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"zenify cost", "cache_read", "3.0k", "znf:ground", "median"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("human output missing %q:\n%s", want, out.String())
		}
	}

	out.Reset()
	if err := runCost(&out, home, project, "", 5, true, false); err != nil {
		t.Fatal(err)
	}
	var env struct {
		SchemaVersion string      `json:"schema_version"`
		Data          cost.Report `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if env.SchemaVersion == "" || env.Data.Main.CacheRead != 3000 || env.Data.SkillsBy["znf:ground"] != 1 {
		t.Fatalf("json data = %+v", env.Data)
	}
}

func TestRunCost_BadArgs(t *testing.T) {
	home, project := fakeHome(t)
	var out bytes.Buffer
	err := runCost(&out, home, project, "yesterday", 5, false, false)
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Fatalf("bad --since: code %d err %v", exitcode.Code(err), err)
	}
	err = runCost(&out, home, filepath.Join(project, "nope"), "7d", 5, false, false)
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Fatalf("missing transcripts: code %d err %v", exitcode.Code(err), err)
	}
}

func TestMtokPerCall(t *testing.T) {
	if got := mtokPerCall(2_000_000, 2); got != "1.000" {
		t.Fatalf("mtokPerCall(2M,2) = %q, want 1.000", got)
	}
	if got := mtokPerCall(500, 0); got != "—" {
		t.Fatalf("mtokPerCall(_,0) = %q, want — (no divide-by-zero)", got)
	}
}

func TestBySkillRows_SortAndCalls(t *testing.T) {
	r := &cost.Report{
		SkillsBy: map[string]int{"znf:cook": 2, "znf:ground": 1, cost.NoSkillKey: 0},
		SkillTok: map[string]cost.Usage{
			"znf:cook":      {Input: 1_000_000},
			"znf:ground":    {Input: 4_000_000},
			cost.NoSkillKey: {Input: 500},
		},
	}
	rows := bySkillRows(r)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	if rows[0][0] != "znf:ground" {
		t.Fatalf("first row = %s, want znf:ground (highest token)", rows[0][0])
	}
	// cook: 1_000_000 / 2 = 0.500
	var cook []string
	for _, row := range rows {
		if row[0] == "znf:cook" {
			cook = row
		}
	}
	if cook[1] != "2" || cook[3] != "0.500" {
		t.Fatalf("cook row = %v, want calls=2 mtok=0.500", cook)
	}
	// (no skill): 0 calls → "—"
	for _, row := range rows {
		if row[0] == cost.NoSkillKey && row[3] != "—" {
			t.Fatalf("(no skill) mtok = %s, want —", row[3])
		}
	}
}
