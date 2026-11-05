package trackstats

import (
	"testing"
	"time"
)

const planDay = "2026-10-30"

func planRow(ts time.Time, day string, ids ...string) string {
	return row(ts, append([]string{"plan", day}, ids...)...)
}

func TestParsePlanRow(t *testing.T) {
	ev, bad := ParseLog(lines(row(now, "plan", planDay, "b-261105-ab12", "https://github.com/o/r/pull/3")))
	if bad != 0 || len(ev) != 1 || ev[0].Type != "plan" || ev[0].ID != planDay ||
		len(ev[0].F) != 2 || ev[0].F[0] != "b-261105-ab12" || ev[0].F[1] != "https://github.com/o/r/pull/3" {
		t.Fatalf("ev=%+v bad=%d", ev, bad)
	}
	for _, l := range []string{
		row(now, "plan", "2026-13-40", "b-261105-ab12"),
		row(now, "plan", planDay),
		row(now, "plan", planDay, "foo"),
	} {
		if ev, bad := ParseLog(lines(l)); bad != 1 || len(ev) != 0 {
			t.Fatalf("%q: ev=%+v bad=%d", l, ev, bad)
		}
	}
}

func TestDailyPlanLatestToday(t *testing.T) { // SC-4
	a, b := "b-261030-aaaa", "b-261030-bbbb"
	r, _ := Compute(base(lines(planRow(now.Add(-2*time.Hour), planDay, a), planRow(now.Add(-time.Hour), planDay, b))))
	p := r.Today.DailyPlan
	if p == nil || len(p.Items) != 1 || p.Items[0].ID != b {
		t.Fatalf("plan = %+v", p)
	}
	r, _ = Compute(base(lines(planRow(now.Add(-24*time.Hour), "2026-10-29", a))))
	if r.Today.DailyPlan != nil {
		t.Fatalf("yesterday plan leaked: %+v", r.Today.DailyPlan)
	}
}

func TestDailyPlanStatus(t *testing.T) { // SC-5
	a, b, c := "b-261030-aaaa", "b-261030-bbbb", "b-261030-cccc"
	ia, ib := cid("cook", 1), cid("cook", 2)
	r, _ := Compute(base(lines(
		row(now.Add(-5*time.Hour), "backlog", a, "kit", "S", "-", "task a"),
		row(now.Add(-5*time.Hour), "backlog", b, "kit", "M", "-", "task b"),
		row(now.Add(-5*time.Hour), "backlog", c, "web", "L", "-", "task c"),
		row(now.Add(-4*time.Hour), "start", ib, "cook", "kit", "M", b),
		row(now.Add(-4*time.Hour).Add(40*time.Minute), "end", ib, "https://x/pr/1", "1", "0"),
		row(now.Add(-10*time.Minute), "start", ia, "cook", "kit", "S", a),
		planRow(now.Add(-time.Minute), planDay, a, b, c),
	)))
	p := r.Today.DailyPlan
	if p == nil || len(p.Items) != 3 {
		t.Fatalf("plan = %+v", p)
	}
	for i, want := range []string{"running", "done", "todo"} {
		if p.Items[i].Status != want {
			t.Fatalf("item %d status = %q, want %q", i, p.Items[i].Status, want)
		}
	}
	if p.EstMin != 180 || p.WallMin != 50 {
		t.Fatalf("estMin=%d wallMin=%d", p.EstMin, p.WallMin)
	}
	if p.Items[2].Title != "task c" || p.Items[2].Repo != "web" || p.Items[2].Est != "L" {
		t.Fatalf("item c = %+v", p.Items[2])
	}
}

func TestDailyPlanTrackID(t *testing.T) {
	id := cid("fix", 7)
	r, _ := Compute(base(lines(
		row(now.Add(-3*time.Hour), "start", id, "fix", "hub", "S"),
		row(now.Add(-2*time.Hour), "end", id, "cancelled", "-", "-"),
		planRow(now.Add(-time.Minute), planDay, id),
	)))
	p := r.Today.DailyPlan
	if p == nil || len(p.Items) != 1 || p.Items[0].Status != "cancelled" || p.Items[0].Repo != "hub" || p.Items[0].Est != "S" {
		t.Fatalf("plan = %+v", p)
	}
}

func TestDailyPlanPRMerged(t *testing.T) { // SC-5b
	u := "https://github.com/o/r/pull/9"
	in := base(lines(planRow(now.Add(-time.Minute), planDay, u)))
	asked := 0
	in.Merged = func(url string) (string, error) {
		asked++
		return "2026-10-30T02:00:00Z", nil
	}
	r, cache := Compute(in)
	p := r.Today.DailyPlan
	if p == nil || p.Items[0].Status != "done" || p.Items[0].URL != u {
		t.Fatalf("plan = %+v", p)
	}
	if asked != 1 || cache[u] == "" {
		t.Fatalf("asked=%d cache=%v", asked, cache)
	}
	if r.Today.PRsWaiting != 0 || r.Today.Merged != 0 {
		t.Fatalf("plan PR counted: waiting=%d merged=%d", r.Today.PRsWaiting, r.Today.Merged)
	}
	if got := planPRURLs(p); len(got) != 1 || got[0] != u {
		t.Fatalf("planPRURLs = %v", got)
	}
}

func TestValidPlanID(t *testing.T) {
	for id, want := range map[string]bool{
		"b-261105-ab12": true, "https://x/pull/1": true, "repos/k/progress.md:3": true, "cook-261105-ab12": true,
		"foo": false, "http://x y": false, "b-261105-AB12": false, "": false,
		"https://github.com/o/r/pull/3": true, "httpfoo:x": false, "https://x/\x1b]8;;": false, "http://": false,
		"repos/a b/progress.md:1": false, "repos/a\tb/progress.md:1": false,
	} {
		if ValidPlanID(id) != want {
			t.Fatalf("ValidPlanID(%q) != %v", id, want)
		}
	}
}

func TestDailyPlanTrackTitle(t *testing.T) {
	a, b := cid("cook", 1), cid("cook", 2)
	bid := "b-261030-aaaa"
	in := base(lines(
		row(now.Add(-4*time.Hour), "start", a, "cook", "kit", "M"),
		row(now.Add(-3*time.Hour), "end", a, "branch:x", "1", "0", "docs/plans/kit/p.md"),
		row(now.Add(-2*time.Hour), "backlog", bid, "kit", "S", "-", "from backlog"),
		row(now.Add(-time.Hour), "start", b, "cook", "kit", "S", bid),
		planRow(now.Add(-time.Minute), planDay, a, b, a),
	))
	in.PlanTitle = func(string) (string, error) { return "Plan A" + planSuffix, nil }
	r, _ := Compute(in)
	p := r.Today.DailyPlan
	if p == nil || len(p.Items) != 2 {
		t.Fatalf("duplicate id kept: %+v", p)
	}
	if p.Items[0].Title != "Plan A" || p.Items[1].Title != "from backlog" {
		t.Fatalf("titles = %q, %q", p.Items[0].Title, p.Items[1].Title)
	}
	if p.EstMin != 45+15 {
		t.Fatalf("estMin = %d, want 60", p.EstMin)
	}
}

func TestPlanStatusMapping(t *testing.T) {
	for in, want := range map[string]string{
		"running": "running", "stale": "running", "done": "done", "pushed": "done",
		"cancelled": "cancelled", "rollback": "cancelled",
	} {
		if got := planStatus(in); got != want {
			t.Fatalf("planStatus(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDailyPlanEstFallback(t *testing.T) {
	a := "b-261030-aaaa"
	ia := cid("cook", 1)
	r, _ := Compute(base(lines(
		row(now.Add(-5*time.Hour), "backlog", a, "kit", "S", "-", "task a"),
		row(now.Add(-10*time.Minute), "start", ia, "cook", "kit", "-", a),
		planRow(now.Add(-time.Minute), planDay, a),
	)))
	p := r.Today.DailyPlan
	if p == nil || len(p.Items) != 1 || p.Items[0].Est != "S" || p.EstMin != 15 {
		t.Fatalf("plan = %+v", p)
	}
}

func TestDailyPlanEstCountedOnce(t *testing.T) {
	a := "b-261030-aaaa"
	ia := cid("cook", 1)
	r, _ := Compute(base(lines(
		row(now.Add(-5*time.Hour), "backlog", a, "kit", "S", "-", "task a"),
		row(now.Add(-10*time.Minute), "start", ia, "cook", "kit", "S", a),
		planRow(now.Add(-time.Minute), planDay, ia, a),
	)))
	p := r.Today.DailyPlan
	if p == nil || p.EstMin != 15 {
		t.Fatalf("plan = %+v", p)
	}
}

func TestMergedAskedOnce(t *testing.T) {
	u := "https://github.com/o/r/pull/9"
	id := cid("cook", 1)
	in := base(lines(
		row(now.Add(-3*time.Hour), "start", id, "cook", "kit", "S"),
		row(now.Add(-2*time.Hour), "end", id, u, "1", "0"),
		planRow(now.Add(-time.Minute), planDay, u),
	))
	asked := 0
	in.Merged = func(string) (string, error) { asked++; return "", nil }
	Compute(in)
	if asked != 1 {
		t.Fatalf("asked = %d", asked)
	}
}

// A backlog row added with --ref <url> gives its plan item that url, so the pane renders a link.
func TestDailyPlanBacklogRefURL(t *testing.T) {
	a := "b-261030-aaaa"
	r, _ := Compute(base(lines(
		row(now.Add(-5*time.Hour), "backlog", a, "kit", "S", "https://github.com/o/r/issues/3", "task a"),
		planRow(now.Add(-time.Minute), planDay, a),
	)))
	p := r.Today.DailyPlan
	if p == nil || len(p.Items) != 1 || p.Items[0].URL != "https://github.com/o/r/issues/3" {
		t.Fatalf("plan = %+v", p)
	}
}
