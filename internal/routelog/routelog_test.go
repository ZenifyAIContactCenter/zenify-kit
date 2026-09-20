package routelog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteLoadSummarize(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rl")
	recs := []Record{
		{TS: "2026-09-21T01:00:00Z", Repo: "kit", Branch: "b1", Site: "architect", Features: map[string]string{"TIER": "architectural", "SHARED": "1"}, Gates: []string{"SHARED"}, Model: "fable", Strong: "fable", ChangedDecision: "yes"},
		{TS: "2026-09-21T01:05:00Z", Repo: "kit", Branch: "b2", Site: "architect", Features: map[string]string{"TIER": "architectural", "REPOS": "2"}, Gates: []string{"REPOS>=2"}, Model: "opus", Strong: "opus", ChangedDecision: "no"},
		{TS: "2026-09-21T01:06:00Z", Repo: "kit", Branch: "b3", Site: "architect", Features: map[string]string{"TIER": "architectural"}, Model: "none", Strong: "fable"},
		{TS: "2026-09-21T02:00:00Z", Repo: "kit", Branch: "b1", Site: "manual", Model: "fable", Strong: "fable", Trigger: "manual"},
		{TS: "2026-09-21T02:01:00Z", Repo: "kit", Branch: "b1", Site: "manual", Model: "fable", Strong: "fable"},
		{TS: "2026-09-21T03:00:00Z", Repo: "kit", Branch: "b1", Site: "plan-metrics", Tasks: 9, FilesPred: 12},
	}
	for _, r := range recs {
		if _, err := WriteRecord(dir, r); err != nil {
			t.Fatal(err)
		}
	}
	got, err := LoadRecords(dir)
	if err != nil || len(got) != 6 {
		t.Fatalf("load: %d %v", len(got), err)
	}
	s := Summarize(got)
	if s.Total != 6 || s.BySite["architect"] != 3 || s.BySite["manual"] != 2 {
		t.Fatalf("by site: %+v", s)
	}
	if s.ModelBySite["architect"]["fable"] != 1 || s.ModelBySite["architect"]["none"] != 1 {
		t.Fatalf("model by site: %+v", s.ModelBySite)
	}
	if s.StrongSent != 3 { // architect fable + 2 manual fable
		t.Fatalf("strong sent = %d", s.StrongSent)
	}
	if s.GateFires["SHARED"] != 1 || s.GateFires["REPOS>=2"] != 1 {
		t.Fatalf("gates: %+v", s.GateFires)
	}
	if s.ArchitectDecided != 2 || s.ArchitectChanged != 1 {
		t.Fatalf("changed_decision: %d/%d", s.ArchitectChanged, s.ArchitectDecided)
	}
	if s.ManualNoTrigger != 1 {
		t.Fatalf("manual without trigger = %d", s.ManualNoTrigger)
	}
}

func TestLoadRecords_MissingDirAndCorruptFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "none")
	recs, err := LoadRecords(dir)
	if err != nil || len(recs) != 0 {
		t.Fatalf("missing dir: %v %d", err, len(recs))
	}
	if _, err := WriteRecord(dir, Record{TS: "2026-09-21T00:00:00Z", Site: "manual"}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "zz-corrupt.json"), "{bad")
	recs, err = LoadRecords(dir)
	if err != nil || len(recs) != 1 {
		t.Fatalf("corrupt must be dropped: %v %d", err, len(recs))
	}
}

func TestPlanMetrics(t *testing.T) {
	plan := "# P\n\n### Task 1: A\n\n**Files:**\n- Create: `a/b.go`\n- Test: `a/b_test.go`\n\n### Task 2: B\n\n**Files:**\n- Modify: `a/b.go:10-20`\n- Create: `c/d.md`\n\n### Task 3: C\n\n**Files:**\n- Modify: `internal/x.go`\n"
	tasks, files := PlanMetrics([]byte(plan))
	if tasks != 3 || files != 4 { // a/b.go counted once (line suffix stripped)
		t.Fatalf("tasks=%d files=%d", tasks, files)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
