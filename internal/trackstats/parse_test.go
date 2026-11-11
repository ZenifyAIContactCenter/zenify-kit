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

func TestParseLogEndPlanField(t *testing.T) {
	log := "2026-11-02T01:00:00Z · start · cook-261102-aaaa · cook · zenify-kit · M\n" +
		"2026-11-02T02:00:00Z · end · cook-261102-aaaa · https://github.com/o/r/pull/1 · 3 · 1 · /ws/docs/plans/zenify-kit/p.md\n" +
		"2026-11-02T03:00:00Z · end · cook-261102-aaaa · cancelled · 0 · 0 · a · b\n"
	ev, bad := ParseLog(log)
	// the 4-field end parses with F[3] = path; a 5-field end is malformed
	if bad != 1 || len(ev) != 2 || len(ev[1].F) != 4 || ev[1].F[3] != "/ws/docs/plans/zenify-kit/p.md" {
		t.Fatalf("bad=%d ev=%+v", bad, ev)
	}
}

func TestParseLogPlanRowWithoutIDs(t *testing.T) {
	ev, bad := ParseLog("2026-11-05T01:00:00Z · plan · 2026-11-05\n")
	if bad != 0 || len(ev) != 1 || ev[0].ID != "2026-11-05" || ev[0].F == nil || len(ev[0].F) != 0 {
		t.Fatalf("bad=%d ev=%+v", bad, ev)
	}
}
