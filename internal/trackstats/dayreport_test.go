package trackstats

import (
	"strings"
	"testing"
	"time"
)

func TestDayReport(t *testing.T) {
	d := "2026-11-09"
	at := func(date string) string {
		ts, _ := time.ParseInLocation("2006-01-02 15:04", date+" 12:00", time.Local)
		return ts.Format(time.RFC3339)
	}
	day := Result{Scale: Scale{S: 30, M: 90, L: 240}}
	day.Today = Today{
		Date:       d,
		Done:       map[string]int{"cook": 2},
		Cancelled:  map[string]int{"fix": 1},
		RolledBack: map[string]int{},
		WallMin:    135,
		AgentMin:   60,
		Rounds:     3,
		PRsOpened:  2,
		Items: []Item{
			{ID: "a", Kind: "cook", Est: "M", Status: "done", EndedAt: at(d), WallMin: 45},  // M hit
			{ID: "b", Kind: "cook", Est: "S", Status: "done", EndedAt: at(d), WallMin: 100}, // L miss
			{ID: "c", Kind: "fix", Est: "S", Status: "cancelled", EndedAt: at(d), WallMin: 10},
		},
		DailyPlan: &DailyPlan{Items: []DailyPlanItem{
			{ID: "plan-todo", Status: "todo", Title: "left over"},
			{ID: "plan-done", Status: "done", Title: "finished"},
		}},
	}
	now := Result{}
	now.Today = Today{
		Date:       "2026-11-10",
		PRsWaiting: 4,
		PROpen: []PRStatus{
			{URL: "https://github.com/o/r/pull/1", Ready: true},
			{URL: "https://github.com/o/r/pull/2", CI: "failure"},
		},
	}
	now.Kitchen.Carried = []CarriedItem{{ID: "old", Kind: "cook", Repo: "kit", Phase: "impl", StartedAt: at("2026-11-04")}}
	cache := map[string]string{
		"https://github.com/o/r/pull/10": at(d),
		"https://github.com/o/r/pull/11": at("2026-11-08"),
		"https://github.com/o/r/pull/12": ClosedUnmerged,
	}
	got := DayReport(day, now, cache, "gh was down")
	for _, w := range []string{
		"# Track day close: 2026-11-09", "cook 2 done", "fix 1 cancelled",
		"agent time: 1h0m", "- merged: 1\n  - https://github.com/o/r/pull/10", "fix rounds 3", "PRs opened 2",
		"pull/10", "estimate: 1/2 hit", "old cook kit impl age 6d (stale soon)",
		"ready to merge", "CI red", "+2 more PRs waiting", "plan-todo", "note: gh was down",
	} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
	for _, w := range []string{"pull/11", "pull/12", "plan-done", "wall"} {
		if strings.Contains(got, w) {
			t.Errorf("unexpected %q in:\n%s", w, got)
		}
	}
	t.Log("\n" + got)
}

func TestDayReportEmpty(t *testing.T) {
	got := DayReport(Result{Today: Today{Date: "2026-11-09"}}, Result{}, nil, "")
	if strings.Count(got, "none") < 3 || strings.Contains(got, "agent time") || !strings.Contains(got, "- merged: none") || strings.Contains(got, "note:") {
		t.Errorf("empty report:\n%s", got)
	}
}

func TestDayReportStaleCarried(t *testing.T) {
	start := time.Now().AddDate(0, 0, -7).Format(time.RFC3339)
	now := Result{Today: Today{Date: time.Now().Format(dateLayout)}}
	now.Kitchen.Carried = []CarriedItem{{ID: "old", Kind: "cook", Repo: "kit", Phase: "impl", StartedAt: start, Stale: true}}
	got := DayReport(Result{Today: Today{Date: "2026-11-09"}}, now, nil, "")
	if !strings.Contains(got, " (stale)\n") || strings.Contains(got, "stale soon") {
		t.Errorf("want only (stale):\n%s", got)
	}
}

// Titles and the note reach the session context: one line each, no control characters, capped.
func TestDayReportSanitizes(t *testing.T) {
	day := Result{Today: Today{Date: "2026-11-09", DailyPlan: &DailyPlan{Items: []DailyPlanItem{
		{ID: "p1", Status: "todo", Title: "a\r\nb\x1b[31m"},
		{ID: "p2", Status: "todo", Title: strings.Repeat("x", 200)},
	}}}}
	got := DayReport(day, Result{}, nil, "gh\nfailed\x07"+strings.Repeat("y", 200))
	if strings.ContainsAny(got, "\r\x1b\x07") {
		t.Errorf("control characters left:\n%q", got)
	}
	for _, w := range []string{"plan: p1 a b [31m (todo)", "note: gh failed"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%q", w, got)
		}
	}
	if !strings.Contains(got, "plan: p2 "+strings.Repeat("x", 120)+" (todo)") {
		t.Errorf("title not capped at 120 runes:\n%q", got)
	}
	if !strings.Contains(got, "note: gh failed "+strings.Repeat("y", 110)+"\n") {
		t.Errorf("note not capped at 120 runes:\n%q", got)
	}
}
