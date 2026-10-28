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
		{"1..20", seq(20), Scale{Source: "learned", N: 20, S: 5, M: 10, L: 15}},
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
