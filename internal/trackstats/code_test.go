package trackstats

import (
	"testing"
	"time"
)

func TestCode(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"b-261111-b81e", "b81e"},
		{"cook-261111-5e3c", "5e3c"},
		{"task-261111-0a1b", "0a1b"},
		{"https://github.com/o/chatting/issues/581", "#581"},
		{"https://github.com/o/be/pull/2906", "#2906"},
		{"https://plane.example/zenify/browse/ZEN-288/", "ZEN-288"},
		{"repos/x/progress.md:7", ""},
		{"https://example.com/a", ""},
		{"https://example.com/issues/581", ""},
	} {
		if got := Code(c.in); got != c.want {
			t.Errorf("Code(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestURLRepo(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"https://github.com/o/chatting/issues/581", "chatting"},
		{"https://github.com/o/be/pull/2906", "be"},
		{"https://example.com/a", ""},
	} {
		if got := URLRepo(c.in); got != c.want {
			t.Errorf("URLRepo(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestItemsCarryCode(t *testing.T) {
	log := lines(
		backlogRow(now.Add(-time.Hour), bidA, "x"),
		planRow(now.Add(-time.Minute), planDay, bidA, issue1),
	)
	r, _ := Compute(base(log))
	if r.Today.DailyPlan == nil || len(r.Today.DailyPlan.Items) != 2 {
		t.Fatalf("plan = %+v", r.Today.DailyPlan)
	}
	for _, it := range r.Today.DailyPlan.Items {
		if it.Code == "" || it.Code != Code(it.ID) {
			t.Errorf("plan item %+v", it)
		}
	}
	for _, it := range r.Backlog.Items {
		if it.Code != Code(it.ID) {
			t.Errorf("backlog item %+v", it)
		}
	}
	if len(r.Backlog.Items) == 0 || r.Backlog.Items[0].Code != "aaaa" {
		t.Errorf("backlog = %+v", r.Backlog.Items)
	}
}
