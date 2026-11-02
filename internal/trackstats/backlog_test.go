package trackstats

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const (
	bidA   = "b-261030-aaaa"
	issue1 = "https://github.com/o/r/issues/1"
	issue2 = "https://github.com/o/r/issues/2"
	sddID  = "repos/kit/.worktrees/x/.znf/sdd/p/progress.md:41"
)

func backlogRow(ts time.Time, bid, title string) string {
	return row(ts, "backlog", bid, "kit", "M", "-", title)
}

func sources(r Result) map[string]int {
	out := map[string]int{}
	for _, it := range r.Backlog.Items {
		out[it.Source]++
	}
	return out
}

func TestBacklogManualLifecycle(t *testing.T) { // SC-30 stats part
	add := backlogRow(now.Add(-48*time.Hour), bidA, "do x · with sep")
	r, _ := Compute(base(lines(add)))
	if len(r.Backlog.Items) != 1 {
		t.Fatalf("items = %+v", r.Backlog.Items)
	}
	it := r.Backlog.Items[0]
	if it.Source != "manual" || it.ID != bidA || it.Repo != "kit" || it.Est != "M" || it.Ref != "-" ||
		it.Title != "do x · with sep" || it.AgeDays != 2 {
		t.Fatalf("item = %+v", it)
	}
	if r.Malformed != 0 {
		t.Fatalf("malformed = %d", r.Malformed)
	}

	id := cid("cook", 1)
	started := lines(add, row(now.Add(-time.Hour), "start", id, "cook", "kit", "M", bidA))
	if r, _ = Compute(base(started)); len(r.Backlog.Items) != 0 {
		t.Fatalf("started from bid still listed: %+v", r.Backlog.Items)
	}
	dropped := lines(add, row(now.Add(-time.Hour), "drop", bidA))
	if r, _ = Compute(base(dropped)); len(r.Backlog.Items) != 0 || r.Malformed != 0 {
		t.Fatalf("dropped still listed: %+v malformed=%d", r.Backlog.Items, r.Malformed)
	}
}

func TestBacklogStartFiveAndSixFields(t *testing.T) {
	a, b, c := cid("cook", 1), cid("cook", 2), cid("cook", 3)
	r, _ := Compute(base(lines(
		row(now, "start", a, "cook", "r", "M"),
		row(now, "start", b, "cook", "r", "M", "-"),
		row(now, "start", c, "cook", "r", "M", "-", "extra"),
	)))
	if r.Malformed != 1 || len(r.Running) != 2 {
		t.Fatalf("malformed=%d running=%d", r.Malformed, len(r.Running))
	}
}

func TestBacklogGitHub(t *testing.T) { // SC-31
	in := base("")
	in.GitHub = func() ([]Issue, error) {
		return []Issue{
			{URL: issue1, Repo: "r", Title: "one", CreatedAt: now.Add(-72 * time.Hour)},
			{URL: issue2, Repo: "r", Title: "two", CreatedAt: now},
			{URL: issue1, Repo: "r", Title: "one", CreatedAt: now.Add(-72 * time.Hour)},
		}, nil
	}
	r, _ := Compute(in)
	if sources(r)["github"] != 2 || r.Backlog.GhError != "" {
		t.Fatalf("backlog = %+v", r.Backlog)
	}
	if it := r.Backlog.Items[0]; it.ID != issue1 || it.AgeDays != 3 || it.Title != "one" || it.Repo != "r" {
		t.Fatalf("item = %+v", it)
	}

	in = base(backlogRow(now, bidA, "x"))
	in.GitHub = func() ([]Issue, error) { return nil, errors.New("gh: boom") }
	r, _ = Compute(in)
	if r.Backlog.GhError == "" || sources(r)["manual"] != 1 {
		t.Fatalf("backlog = %+v", r.Backlog)
	}
}

func TestBacklogGitHubPartial(t *testing.T) {
	in := base("")
	in.GitHub = func() ([]Issue, error) {
		return []Issue{{URL: issue1, Repo: "r", Title: "one", CreatedAt: now}}, errors.New("gh search issues --author: timeout")
	}
	r, _ := Compute(in)
	if sources(r)["github"] != 1 || !strings.Contains(r.Backlog.GhError, "author") {
		t.Fatalf("backlog = %+v", r.Backlog)
	}
}

func TestBacklogGitHubHiddenByFrom(t *testing.T) {
	id := cid("fix", 1)
	in := base(lines(row(now, "start", id, "fix", "r", "S", issue1)))
	in.GitHub = func() ([]Issue, error) {
		return []Issue{{URL: issue1, Repo: "r", Title: "one", CreatedAt: now}, {URL: issue2, Repo: "r", Title: "two", CreatedAt: now}}, nil
	}
	r, _ := Compute(in)
	if len(r.Backlog.Items) != 1 || r.Backlog.Items[0].ID != issue2 {
		t.Fatalf("items = %+v", r.Backlog.Items)
	}
}

func TestBacklogSDDAndDrop(t *testing.T) { // SC-32
	in := base("")
	in.SDD = []SDDItem{
		{ID: sddID, Repo: "kit", Title: "a", ModTime: now.Add(-24 * time.Hour)},
		{ID: "repos/kit/.znf/sdd/p/progress.md:7", Repo: "kit", Title: "b", ModTime: now},
	}
	r, _ := Compute(in)
	if sources(r)["sdd"] != 2 || r.Backlog.Items[0].AgeDays != 1 || r.Backlog.Items[0].Repo != "kit" {
		t.Fatalf("items = %+v", r.Backlog.Items)
	}
	in.Log = lines(row(now, "drop", sddID))
	r, _ = Compute(in)
	if len(r.Backlog.Items) != 1 || r.Backlog.Items[0].Title != "b" || r.Malformed != 0 {
		t.Fatalf("items = %+v malformed=%d", r.Backlog.Items, r.Malformed)
	}
}

func TestBacklogNilSourcesSkipped(t *testing.T) {
	r, _ := Compute(base(""))
	if len(r.Backlog.Items) != 0 || r.Backlog.GhError != "" {
		t.Fatalf("backlog = %+v", r.Backlog)
	}
}

func TestBacklogStale(t *testing.T) {
	id := cid("cook", 1)
	r, _ := Compute(base(lines(row(now.Add(-10*24*time.Hour), "start", id, "cook", "kit", "L"))))
	if len(r.Backlog.Items) != 1 {
		t.Fatalf("items = %+v", r.Backlog.Items)
	}
	it := r.Backlog.Items[0]
	if it.Source != "stale" || it.ID != id || it.Title != "cook kit ("+id+")" || it.AgeDays != 10 || it.Est != "L" {
		t.Fatalf("item = %+v", it)
	}
}

func TestBacklogMalformedAndOnboard(t *testing.T) {
	r, _ := Compute(base(lines(
		row(now, "backlog", bidA, "kit", "M", "-"),       // missing title
		row(now, "backlog", "bad", "kit", "M", "-", "t"), // bad bid
		row(now, "drop"),                                   // no id
		row(now, "onboard", "other"),                       // wrong arg
		row(now, "history", "abcd1234", "fix", "kit", "3"), // wrong field count
	)))
	if r.Malformed != 5 || r.Onboarded {
		t.Fatalf("malformed=%d onboarded=%v", r.Malformed, r.Onboarded)
	}
	r, _ = Compute(base(lines(row(now, "onboard", "memory"))))
	if !r.Onboarded || r.Malformed != 0 {
		t.Fatalf("onboarded=%v malformed=%d", r.Onboarded, r.Malformed)
	}
}
