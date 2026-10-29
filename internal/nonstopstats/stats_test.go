package nonstopstats

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func ln(date, id, repo, kind, est, actual, tm string, files int) string {
	return fmt.Sprintf("%s · %s · %s · %s · %s · %s · %s · %d · 0 · -", date, id, repo, kind, est, actual, tm, files)
}

func TestParse(t *testing.T) {
	text := strings.Join([]string{
		"# a comment",
		"",
		"2026-10-27 · t1 · repo-a · fix · S · M · ~16m · 5 · 1 · slow review",
		"2026-10-27 · t2 · repo-a · fix · S · - · ~3m · 0 · 0 · cancelled",
		"2026-10-27 · t3 · repo-b · chore · M · S · 45s · 0 · 0 · -",
		"2026-10-27 · t4 · repo-b · chore · M · S · 3m · 0 · 0",
		"2026-10-27 · t5 · repo-b · chore · M · S · - · 0 · 0 · -",
		"not a line at all",
	}, "\n")
	lines, malformed := Parse(text)
	if malformed != 2 {
		t.Fatalf("malformed = %d, want 2", malformed)
	}
	if len(lines) != 4 {
		t.Fatalf("lines = %d, want 4", len(lines))
	}
	if l := lines[0]; l.ID != "t1" || l.Repo != "repo-a" || l.Kind != "fix" || l.Est != "S" || l.Actual != "M" || l.Minutes != 16 || l.Files != 5 {
		t.Errorf("line 0 = %+v", l)
	}
	if lines[1].Actual != "" {
		t.Errorf("actual '-' must parse as empty, got %q", lines[1].Actual)
	}
	if lines[2].Minutes != 1 {
		t.Errorf("45s must round up to 1 minute, got %d", lines[2].Minutes)
	}
	if lines[3].Minutes != 0 {
		t.Errorf("'-' time must have no minutes, got %d", lines[3].Minutes)
	}
}

func TestLearnScale(t *testing.T) {
	mk := func(mins []int) []Line {
		var out []Line
		for _, m := range mins {
			out = append(out, Line{Est: "S", Actual: "S", Minutes: m})
		}
		return out
	}
	seq := func(n int) []int {
		var s []int
		for i := 1; i <= n; i++ {
			s = append(s, i)
		}
		return s
	}
	dup := make([]int, 20)
	for i := range dup {
		dup[i] = 3
	}
	cases := []struct {
		name string
		in   []int
		want Scale
	}{
		{"19 lines stay default", seq(19), Scale{Source: "default", N: 19, S: 5, M: 20, L: 60}},
		{"1..20", seq(20), Scale{Source: "learned", N: 20, S: 5, M: 10, L: 18}},
		{"duplicates forced increasing", dup, Scale{Source: "learned", N: 20, S: 3, M: 4, L: 5}},
	}
	for _, c := range cases {
		if got := LearnScale(mk(c.in)); got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
	// Only the 60 most recent lines count: 10 old 1-minute lines then 60 of 100.
	var recent []int
	for i := 0; i < 10; i++ {
		recent = append(recent, 1)
	}
	for i := 0; i < 60; i++ {
		recent = append(recent, 100)
	}
	if got := LearnScale(mk(recent)); got.N != 60 || got.S != 100 {
		t.Errorf("window: got %+v", got)
	}
}

func TestComputeToday(t *testing.T) {
	text := strings.Join([]string{
		ln("2026-10-28", "a", "r", "fix", "S", "S", "~2m", 1),  // hit
		ln("2026-10-28", "b", "r", "fix", "L", "M", "~10m", 2), // over
		ln("2026-10-28", "c", "r", "fix", "S", "M", "~10m", 2), // under
		ln("2026-10-28", "d", "r", "fix", "M", "L", "~30m", 2), // under
		ln("2026-10-28", "x", "r", "fix", "M", "-", "~30m", 2), // cancelled: id only
		ln("2026-10-27", "e", "r", "fix", "S", "S", "~1m", 1),
	}, "\n")
	lines, mal := Parse(text)
	r := Compute(lines, mal, "2026-10-28")
	if r.Missing || r.Scale.Source != "default" || r.Scale.N != 5 {
		t.Fatalf("scale/missing: %+v", r)
	}
	// points: S1 + M2 + M2 + L4 = 9
	want := Today{Date: "2026-10-28", Hit: 1, Total: 4, Over: 1, Under: 2, Points: 9, IDs: []string{"a", "b", "c", "d", "x"}}
	if !reflect.DeepEqual(r.Today, want) {
		t.Errorf("today = %+v, want %+v", r.Today, want)
	}
}

func TestComputeWeek(t *testing.T) {
	text := strings.Join([]string{
		ln("2026-10-21", "old", "r", "fix", "S", "S", "~1m", 1), // outside the 7-day window
		ln("2026-10-23", "a", "r", "fix", "S", "S", "~1m", 1),   // hit, 1 pt
		ln("2026-10-23", "b", "r", "fix", "S", "M", "~10m", 1),  // under, 2 pt
		ln("2026-10-28", "c", "r", "fix", "M", "M", "~10m", 1),  // hit, 2 pt
		ln("2026-10-28", "d", "r", "fix", "M", "L", "~30m", 1),  // under, 4 pt
	}, "\n")
	lines, mal := Parse(text)
	r := Compute(lines, mal, "2026-10-28")
	if r.Week.HitRate == nil || *r.Week.HitRate != 0.5 {
		t.Errorf("hitRate = %v, want 0.5", r.Week.HitRate)
	}
	// 9 points over 2 days
	if r.Week.PointsPerDay == nil || *r.Week.PointsPerDay != 4.5 {
		t.Errorf("pointsPerDay = %v, want 4.5", r.Week.PointsPerDay)
	}
	empty := Compute(nil, 0, "2026-10-28")
	if empty.Week.HitRate != nil || empty.Week.PointsPerDay != nil || empty.Rules == nil {
		t.Errorf("empty: %+v", empty)
	}
}

func TestComputeFilesCriterion(t *testing.T) {
	lines, _ := Parse(ln("2026-10-28", "a", "r", "fix", "L", "S", "~2m", 12))
	r := Compute(lines, 0, "2026-10-28")
	if r.Today.Hit != 1 || r.Today.Points != 4 {
		t.Errorf("2 min + 12 files must be L: %+v", r.Today)
	}
}

func TestComputeRules(t *testing.T) {
	under := func(n int, repo, kind string) []string {
		var out []string
		for i := 0; i < n; i++ {
			out = append(out, ln("2026-10-20", fmt.Sprintf("u%d", i), repo, kind, "S", "M", "~10m", 1))
		}
		return out
	}
	run := func(rows ...string) []string {
		lines, mal := Parse(strings.Join(rows, "\n"))
		return Compute(lines, mal, "2026-10-28").Rules
	}
	if got := run(under(3, "hub", "fix")...); !reflect.DeepEqual(got, []string{"hub fix: " + RuleUp}) {
		t.Errorf("3 under: %v", got)
	}
	if got := run(under(2, "hub", "fix")...); len(got) != 0 {
		t.Errorf("2 under: %v", got)
	}
	mixed := append(under(3, "hub", "fix"), ln("2026-10-20", "z", "hub", "fix", "M", "M", "~10m", 1))
	if got := run(mixed...); len(got) != 0 {
		t.Errorf("mixed: %v", got)
	}
	var over []string
	for i := 0; i < 3; i++ {
		over = append(over, ln("2026-10-20", fmt.Sprintf("o%d", i), "web", "chore", "L", "S", "~1m", 1))
	}
	if got := run(over...); !reflect.DeepEqual(got, []string{"web chore: " + RuleDown}) {
		t.Errorf("3 over: %v", got)
	}
}

func ln13(date, id, repo, kind, est, actual, work, review, path, tier string) string {
	return fmt.Sprintf("%s · %s · %s · %s · %s · %s · %s · %s · 1 · 0 · %s · %s · -", date, id, repo, kind, est, actual, work, review, path, tier)
}

func TestParseV2(t *testing.T) {
	text := strings.Join([]string{
		ln13("2026-10-28", "a", "r", "fix", "S", "S", "~4m", "~2m", "bounded", "T2"),
		ln13("2026-10-28", "b", "r", "fix", "S", "S", "-", "-", "-", "-"),
		ln13("2026-10-28", "c", "r", "fix", "S", "S", "~4m", "~2m", "huge", "T2"),
		ln13("2026-10-28", "d", "r", "fix", "S", "S", "~4m", "~2m", "spike", "T4"),
		ln("2026-10-28", "e", "r", "fix", "S", "S", "~3m", 2), // v1 still parses
	}, "\n")
	lines, malformed := Parse(text)
	if malformed != 2 || len(lines) != 3 {
		t.Fatalf("lines=%d malformed=%d, want 3 and 2", len(lines), malformed)
	}
	if l := lines[0]; l.Minutes != 4 || l.ReviewMin != 2 || l.Path != "bounded" || l.Tier != "T2" || l.Files != 1 {
		t.Errorf("v2 line = %+v", l)
	}
	if l := lines[1]; l.Minutes != 0 || l.ReviewMin != 0 || l.Path != "-" || l.Tier != "-" {
		t.Errorf("dash line = %+v", l)
	}
	if l := lines[2]; l.Minutes != 3 || l.Files != 2 || l.Path != "-" || l.Tier != "-" || l.ReviewMin != 0 {
		t.Errorf("v1 line = %+v", l)
	}
}

func TestParseDurations(t *testing.T) {
	text := strings.Join([]string{
		"2026-10-28T12:00:00.000Z · t1 · work · 90000",
		"2026-10-28T12:05:00.000Z · t1 · review · 30000",
		"",
		"garbage",
		"2026-10-28T12:08:00.000Z · t3 · other · 5",
		"2026-10-28T12:09:00.000Z · t3 · work · -5",
		"not-a-time · t3 · work · 5",
	}, "\n")
	d, malformed := ParseDurations(text)
	if malformed != 4 || len(d) != 2 {
		t.Fatalf("malformed=%d entries=%d, want 4 and 2", malformed, len(d))
	}
	if d[0].ID != "t1" || d[0].Kind != "work" || d[0].Ms != 90000 || d[1].Kind != "review" {
		t.Errorf("entries = %+v", d)
	}
}

func TestApplyDurations(t *testing.T) {
	at := func(day, id, kind string, ms int) string {
		return fmt.Sprintf("%sT12:00:00.000Z · %s · %s · %d", day, id, kind, ms)
	}
	dur := func(rows ...string) Durations {
		d, mal := ParseDurations(strings.Join(rows, "\n"))
		if mal != 0 {
			t.Fatalf("malformed %d", mal)
		}
		return d
	}
	// the log wins; no log keeps the ledger columns; sums round up (150001 ms -> 3)
	lines := []Line{{Date: "2026-10-28", ID: "a", Minutes: 9, ReviewMin: 9}, {Date: "2026-10-28", ID: "b", Minutes: 7, ReviewMin: 4}}
	ApplyDurations(lines, dur(at("2026-10-28", "a", "work", 90000), at("2026-10-28", "a", "work", 60001), at("2026-10-28", "a", "review", 30000)))
	if lines[0].Minutes != 3 || lines[0].ReviewMin != 1 || lines[1].Minutes != 7 || lines[1].ReviewMin != 4 {
		t.Errorf("log/columns: %+v", lines)
	}
	// an id reused on two days: each line gets only its own window
	lines = []Line{{Date: "2026-10-27", ID: "q1"}, {Date: "2026-10-28", ID: "q1"}}
	ApplyDurations(lines, dur(at("2026-10-27", "q1", "work", 120000), at("2026-10-28", "q1", "work", 300000)))
	if lines[0].Minutes != 2 || lines[1].Minutes != 5 {
		t.Errorf("reused id: %+v", lines)
	}
	// a task whose runs span two days, line on the second: both days summed
	lines = []Line{{Date: "2026-10-28", ID: "q2"}}
	ApplyDurations(lines, dur(at("2026-10-27", "q2", "work", 120000), at("2026-10-28", "q2", "work", 180000)))
	if lines[0].Minutes != 5 {
		t.Errorf("spanning task: %+v", lines)
	}
	// runs after the line's date belong to no window
	lines = []Line{{Date: "2026-10-27", ID: "q3", Minutes: 6}}
	ApplyDurations(lines, dur(at("2026-10-28", "q3", "work", 120000)))
	if lines[0].Minutes != 6 {
		t.Errorf("later run must not apply: %+v", lines)
	}
}

func TestComputeExcludesCancelled(t *testing.T) {
	text := strings.Join([]string{
		ln13("2026-10-28", "a", "r", "fix", "S", "S", "~2m", "~2m", "-", "T1"),
		ln13("2026-10-28", "x", "r", "fix", "S", "-", "~8m", "~6m", "-", "T3"),
	}, "\n")
	lines, mal := Parse(text)
	r := Compute(lines, mal, "2026-10-28")
	if r.Today.ReviewMinutes != 2 || r.Tiers != (Tiers{T1: 1}) || r.Week.ReviewShare == nil || *r.Week.ReviewShare != 0.5 {
		t.Errorf("cancelled must be excluded: %+v %+v", r.Today, r.Tiers)
	}
}

func TestTierRuleSkipsUnrecordedTier(t *testing.T) {
	rows := []string{
		ln13("2026-10-20", "x0", "hub", "fix", "S", "S", "~1m", "-", "-", "T1"),
		ln13("2026-10-20", "x1", "hub", "fix", "S", "S", "~1m", "-", "-", "T1"),
		ln13("2026-10-20", "x2", "hub", "fix", "S", "S", "~1m", "-", "-", "T1"),
		ln("2026-10-20", "v1", "hub", "fix", "S", "S", "~1m", 1),
		ln13("2026-10-20", "c", "hub", "fix", "S", "-", "~1m", "-", "-", "T2"),
	}
	lines, mal := Parse(strings.Join(rows, "\n"))
	got := Compute(lines, mal, "2026-10-28").Rules
	if !reflect.DeepEqual(got, []string{"hub fix: " + RuleTier + " T1"}) {
		t.Errorf("rules = %v", got)
	}
}

func TestComputeReviewAndTiers(t *testing.T) {
	text := strings.Join([]string{
		ln13("2026-10-21", "old", "r", "fix", "S", "S", "~5m", "~5m", "-", "T3"), // outside the week
		ln13("2026-10-24", "a", "r", "fix", "S", "S", "~1m", "~3m", "-", "T1"),
		ln13("2026-10-28", "b", "r", "fix", "S", "S", "~2m", "~1m", "-", "T2"),
		ln13("2026-10-28", "c", "r", "fix", "S", "S", "~1m", "~2m", "-", "T2"),
		ln13("2026-10-28", "d", "r", "fix", "S", "S", "~2m", "-", "-", "-"),
	}, "\n")
	lines, mal := Parse(text)
	r := Compute(lines, mal, "2026-10-28")
	if r.Today.ReviewMinutes != 3 {
		t.Errorf("today review = %d, want 3", r.Today.ReviewMinutes)
	}
	// week review 3+1+2 = 6, work 1+2+1+2 = 6 -> 0.5
	if r.Week.ReviewShare == nil || *r.Week.ReviewShare != 0.5 {
		t.Errorf("reviewShare = %v, want 0.5", r.Week.ReviewShare)
	}
	if r.Tiers != (Tiers{T1: 1, T2: 2, T3: 0}) {
		t.Errorf("tiers = %+v", r.Tiers)
	}
	if e := Compute(nil, 0, "2026-10-28"); e.Week.ReviewShare != nil {
		t.Errorf("empty reviewShare must be nil")
	}
}

func TestComputeTierRule(t *testing.T) {
	rows := func(tiers ...string) []string {
		var out []string
		for i, tr := range tiers {
			out = append(out, ln13("2026-10-20", fmt.Sprintf("x%d", i), "hub", "fix", "S", "S", "~1m", "-", "-", tr))
		}
		return out
	}
	run := func(rs []string) []string {
		lines, mal := Parse(strings.Join(rs, "\n"))
		return Compute(lines, mal, "2026-10-28").Rules
	}
	if got := run(rows("T1", "T1", "T1")); !reflect.DeepEqual(got, []string{"hub fix: " + RuleTier + " T1"}) {
		t.Errorf("3 same: %v", got)
	}
	if got := run(rows("T1", "T1")); len(got) != 0 {
		t.Errorf("2 same: %v", got)
	}
	if got := run(rows("T1", "T1", "T2")); len(got) != 0 {
		t.Errorf("mixed: %v", got)
	}
	if got := run(rows("-", "-", "-")); len(got) != 0 {
		t.Errorf("no tier: %v", got)
	}
}

func TestApplyDurationsCancelThenRedoSameDay(t *testing.T) {
	d, mal := ParseDurations(strings.Join([]string{
		"2026-10-28T09:00:00.000Z · t1 · work · 120000",
		"2026-10-28T10:00:00.000Z · t1 · work · 180000",
		"2026-10-28T10:30:00.000Z · t1 · review · 60000",
	}, "\n"))
	if mal != 0 {
		t.Fatalf("malformed %d", mal)
	}
	// the cancelled line, the redone line, then a later cancelled restart: the runs go to the
	// last line of that day that has an actual
	lines := []Line{
		{Date: "2026-10-28", ID: "t1", Actual: ""},
		{Date: "2026-10-28", ID: "t1", Actual: "S"},
		{Date: "2026-10-28", ID: "t1", Actual: ""},
	}
	ApplyDurations(lines, d)
	if lines[0].Minutes != 0 || lines[1].Minutes != 5 || lines[1].ReviewMin != 1 || lines[2].Minutes != 0 {
		t.Errorf("same-day redo: %+v", lines)
	}
}

func TestComputeWeekIgnoresCancelledOnlyDays(t *testing.T) {
	text := strings.Join([]string{
		ln("2026-10-27", "a", "r", "fix", "S", "S", "~1m", 1),
		ln("2026-10-28", "x", "r", "fix", "S", "-", "~1m", 1),
	}, "\n")
	lines, mal := Parse(text)
	r := Compute(lines, mal, "2026-10-28")
	if r.Week.PointsPerDay == nil || *r.Week.PointsPerDay != 1 {
		t.Errorf("pointsPerDay = %v, want 1", r.Week.PointsPerDay)
	}
	only, mal := Parse(ln("2026-10-28", "x", "r", "fix", "S", "-", "~1m", 1))
	if r := Compute(only, mal, "2026-10-28"); r.Week.PointsPerDay != nil {
		t.Errorf("cancelled only: pointsPerDay = %v, want nil", *r.Week.PointsPerDay)
	}
}

func TestComputeRulesUseRecentWindow(t *testing.T) {
	// the oldest line is dated today: outside the rule window, still in the day stats
	rows := []string{ln("2026-10-28", "old", "hub", "fix", "S", "S", "-", 1)}
	for i := 0; i < 30; i++ {
		rows = append(rows, ln("2026-10-20", fmt.Sprintf("u%d", i), "hub", "fix", "S", "M", "-", 1))
	}
	lines, mal := Parse(strings.Join(rows, "\n"))
	r := Compute(lines, mal, "2026-10-28")
	if !reflect.DeepEqual(r.Rules, []string{"hub fix: " + RuleUp}) {
		t.Errorf("rules = %v, want the up rule from the last 30 lines", r.Rules)
	}
	if r.Today.Total != 1 || r.Today.Hit != 1 {
		t.Errorf("today must count the line outside the window: %+v", r.Today)
	}
	// tier rule: an old differing tier outside the window does not block it
	trows := []string{ln13("2026-10-01", "old", "web", "fix", "S", "S", "-", "-", "-", "T3")}
	for i := 0; i < 30; i++ {
		trows = append(trows, ln13("2026-10-20", fmt.Sprintf("w%d", i), "web", "fix", "S", "S", "-", "-", "-", "T1"))
	}
	lines, mal = Parse(strings.Join(trows, "\n"))
	if got := Compute(lines, mal, "2026-10-28").Rules; !reflect.DeepEqual(got, []string{"web fix: " + RuleTier + " T1"}) {
		t.Errorf("tier rules = %v", got)
	}
}
