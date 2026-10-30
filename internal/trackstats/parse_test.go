package trackstats

import (
	"strings"
	"testing"
)

func TestParseLogMalformed(t *testing.T) {
	text := strings.Join([]string{
		"# comment",
		"2026-10-30T01:00:00Z · start · cook-261030-ab12 · cook · zenify-kit · M",
		"2026-10-30T01:01:00Z · est · cook-261030-ab12",
		"2026-10-30T01:02:00Z · bogus · cook-261030-ab12 · x",
		"2026-10-30T01:03:00Z · ci · cook-261030-ab12 · green",
		"",
	}, "\n")
	ev, bad := ParseLog(text)
	if bad != 2 {
		t.Fatalf("malformed = %d, want 2", bad)
	}
	if len(ev) != 2 || ev[0].Type != "start" || ev[1].Type != "ci" || ev[1].ID != "cook-261030-ab12" {
		t.Fatalf("events = %+v", ev)
	}
}

func TestParseSession(t *testing.T) {
	text := strings.Join([]string{
		"2026-10-30T01:00:00Z · cwd · /w/repo",
		"2026-10-30T01:00:01Z · bind · cook-261030-ab12",
		"2026-10-30T01:10:00Z · turn · cook-261030-ab12 · 60000 · main · opus · 1 · 2 · - · -",
		"2026-10-30T01:10:00Z · turn · cook-261030-ab12 · 0 · main · opus · 1 · 2 · - · -",
		"2026-10-30T01:10:00Z · wait · cook-261030-ab12 · 5 · nap",
		"not a date · phase · cook-261030-ab12 · plan",
	}, "\n")
	ev, cwd, bad := ParseSession("s1", text)
	if cwd != "/w/repo" || bad != 3 || len(ev) != 2 {
		t.Fatalf("cwd=%q bad=%d ev=%+v", cwd, bad, ev)
	}
}
