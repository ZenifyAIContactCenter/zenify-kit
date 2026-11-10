package trackstats

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// DayReport renders the close-of-day markdown for day.Today.Date. day is Compute run
// with Today = that date, now is Compute run for the current day, cache is merged.json
// and prNote is an optional PR-status warning.
//
// Dates compare in time.Local, the location the track clock runs in (Compute uses
// in.Now.Location(), which is Local in the CLI).
func DayReport(day, now Result, cache map[string]string, prNote string) string {
	d := day.Today.Date
	var b strings.Builder
	fmt.Fprintf(&b, "# Track day close: %s\n\n## %s\n\n", d, d)

	var counts []string
	for _, c := range []struct {
		label string
		m     map[string]int
	}{{"done", day.Today.Done}, {"cancelled", day.Today.Cancelled}, {"rollback", day.Today.RolledBack}} {
		kinds := make([]string, 0, len(c.m))
		for k, n := range c.m {
			if n > 0 {
				kinds = append(kinds, k)
			}
		}
		sort.Strings(kinds)
		for _, k := range kinds {
			counts = append(counts, fmt.Sprintf("%s %d %s", k, c.m[k], c.label))
		}
	}
	fmt.Fprintf(&b, "- outcome: %s\n", orNone(strings.Join(counts, ", ")))
	if day.Today.AgentMin > 0 {
		fmt.Fprintf(&b, "- agent time: %s\n", hm(day.Today.AgentMin))
	}
	fmt.Fprintf(&b, "- fix rounds %d\n- PRs opened %d\n", day.Today.Rounds, day.Today.PRsOpened)

	var merged []string
	for url, at := range cache {
		if at == ClosedUnmerged {
			continue
		}
		if t, err := time.Parse(time.RFC3339, at); err == nil && t.In(time.Local).Format(dateLayout) == d {
			merged = append(merged, url)
		}
	}
	sort.Strings(merged)
	if len(merged) == 0 {
		b.WriteString("- merged: none\n")
	} else {
		fmt.Fprintf(&b, "- merged: %d\n", len(merged))
		for _, u := range merged {
			b.WriteString("  - " + u + "\n")
		}
	}

	hit, n := 0, 0
	for _, it := range day.Today.Items {
		if it.Status != "done" && it.Status != "pushed" || it.WallMin <= 0 {
			continue
		}
		if it.Est != "S" && it.Est != "M" && it.Est != "L" {
			continue
		}
		t, err := time.Parse(time.RFC3339, it.EndedAt)
		if err != nil || t.In(time.Local).Format(dateLayout) != d {
			continue
		}
		n++
		if it.Est == sizeOf(it.WallMin, day.Scale) {
			hit++
		}
	}
	if n == 0 {
		b.WriteString("- estimate: none\n")
	} else {
		fmt.Fprintf(&b, "- estimate: %d/%d hit\n", hit, n)
	}

	b.WriteString("\n## Open now\n\n")
	var open []string
	for _, c := range now.Kitchen.Carried {
		line := fmt.Sprintf("%s %s %s %s", c.ID, c.Kind, c.Repo, c.Phase)
		age := carriedAge(c.StartedAt, now.Today.Date)
		if age >= 0 {
			line += fmt.Sprintf(" age %dd", age)
		}
		if age >= 5 {
			line += " (stale soon)"
		}
		if c.Stale {
			line += " (stale)"
		}
		open = append(open, "carried: "+line)
	}
	for _, p := range now.Today.PROpen {
		step := "waiting for review"
		switch {
		case p.Ready:
			step = "ready to merge"
		case p.CI == "failure":
			step = "CI red"
		case p.Threads > 0:
			step = fmt.Sprintf("%d unresolved threads", p.Threads)
		}
		open = append(open, fmt.Sprintf("PR: %s %s", p.URL, step))
	}
	if more := now.Today.PRsWaiting - len(now.Today.PROpen); more > 0 {
		open = append(open, fmt.Sprintf("+%d more PRs waiting", more))
	}
	if day.Today.DailyPlan != nil {
		for _, p := range day.Today.DailyPlan.Items {
			if p.Status != "done" {
				open = append(open, fmt.Sprintf("plan: %s %s (%s)", p.ID, p.Title, p.Status))
			}
		}
	}
	if len(open) == 0 {
		b.WriteString("- none\n")
	}
	for _, l := range open {
		b.WriteString("- " + l + "\n")
	}
	if prNote != "" {
		b.WriteString("\nnote: " + prNote + "\n")
	}
	return b.String()
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

func hm(min int) string { return fmt.Sprintf("%dh%dm", min/60, min%60) }

// carriedAge is the whole days from startedAt's local date to today, -1 when unparsable.
func carriedAge(startedAt, today string) int {
	s, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return -1
	}
	t, err := time.ParseInLocation(dateLayout, today, time.Local)
	if err != nil {
		return -1
	}
	sd, _ := time.ParseInLocation(dateLayout, s.In(time.Local).Format(dateLayout), time.Local)
	return int(t.Sub(sd).Round(24*time.Hour) / (24 * time.Hour))
}
