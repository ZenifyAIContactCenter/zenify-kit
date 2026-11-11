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

func TestBacklogGitHubHiddenByDrop(t *testing.T) { // SC-8 stats part
	in := base(lines(row(now, "drop", issue1)))
	in.GitHub = func() ([]Issue, error) {
		return []Issue{{URL: issue1, Repo: "r", Title: "one", CreatedAt: now}, {URL: issue2, Repo: "r", Title: "two", CreatedAt: now}}, nil
	}
	r, _ := Compute(in)
	if len(r.Backlog.Items) != 1 || r.Backlog.Items[0].ID != issue2 || r.Malformed != 0 {
		t.Fatalf("items = %+v malformed=%d", r.Backlog.Items, r.Malformed)
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

// SC-7
func TestBacklogSDDHiddenByFrom(t *testing.T) {
	id := cid("cook", 1)
	in := base(lines(row(now, "start", id, "cook", "kit", "M", sddID)))
	in.SDD = []SDDItem{
		{ID: sddID, Repo: "kit", Title: "a", ModTime: now},
		{ID: "repos/kit/.znf/sdd/p/progress.md:7", Repo: "kit", Title: "b", ModTime: now},
	}
	r, _ := Compute(in)
	if r.Malformed != 0 || len(r.Backlog.Items) != 1 || r.Backlog.Items[0].Title != "b" {
		t.Fatalf("malformed=%d items=%+v", r.Malformed, r.Backlog.Items)
	}
}

// SC-8
func TestBacklogStaleHiddenWhileSessionAlive(t *testing.T) {
	id := cid("cook", 1)
	const sess = "sess-1"
	log := lines(row(now.Add(-8*24*time.Hour), "start", id, "cook", "kit", "L"))
	mk := func(beats map[string]time.Time) Input {
		in := base(log)
		in.Sessions = map[string]string{sess: row(now.Add(-8*24*time.Hour), "bind", id) + "\n"}
		in.Beats = beats
		return in
	}
	for _, tc := range []struct {
		name  string
		beats map[string]time.Time
		want  int
	}{
		{"fresh beat", map[string]time.Time{sess: now.Add(-10 * time.Second)}, 0},
		{"old beat", map[string]time.Time{sess: now.Add(-60 * time.Second)}, 1},
		{"no beat", nil, 1},
	} {
		r, _ := Compute(mk(tc.beats))
		if got := sources(r)["stale"]; got != tc.want {
			t.Errorf("%s: stale items = %d, want %d (%+v)", tc.name, got, tc.want, r.Backlog.Items)
		}
	}
}

// SC-9
func TestBacklogSortAndDedupe(t *testing.T) {
	mk := func(n int, est string, age int) string {
		return row(now.Add(-time.Duration(age)*24*time.Hour), "backlog", "b-261030-"+string(rune('a'+n))+"aaa", "kit", est, "-", "t"+string(rune('a'+n)))
	}
	in := base(lines(
		mk(0, "-", 9),
		mk(1, "L", 1),
		mk(2, "S", 2),
		mk(3, "M", 3),
		mk(4, "S", 5),
	))
	r, _ := Compute(in)
	var got []string
	for _, it := range r.Backlog.Items {
		got = append(got, it.Title)
	}
	// S (older first), M, L, then "-".
	want := []string{"te", "tc", "td", "tb", "ta"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", got, want)
	}

	in = base(lines(backlogRow(now, bidA, "Fix login")))
	in.GitHub = func() ([]Issue, error) {
		return []Issue{{URL: issue1, Repo: "r", Title: "fix  Login", CreatedAt: now}}, nil
	}
	r, _ = Compute(in)
	if len(r.Backlog.Items) != 1 || r.Backlog.Items[0].Source != "github" {
		t.Fatalf("items = %+v", r.Backlog.Items)
	}
}

const (
	plane288 = "https://plane.example/zenify/browse/ZEN-288/"
	plane256 = "https://plane.example/zenify/browse/ZEN-256/"
)

// FR-5.2, FR-5.3: Plane items enter the backlog as source plane, right after manual.
func TestBacklogPlane(t *testing.T) {
	in := base(lines(backlogRow(now, bidA, "manual one")))
	in.GitHub = func() ([]Issue, error) {
		return []Issue{{URL: issue1, Repo: "r", Title: "gh one", CreatedAt: now}}, nil
	}
	in.SDD = []SDDItem{{ID: sddID, Repo: "kit", Title: "sdd one", ModTime: now}}
	in.Plane = func() ([]PlaneItem, error) {
		return []PlaneItem{
			{Key: "ZEN-288", URL: plane288, Title: "plane one", CreatedAt: now.Add(-48 * time.Hour)},
			{Key: "ZEN-288", URL: plane288, Title: "plane one", CreatedAt: now.Add(-48 * time.Hour)},
		}, nil
	}
	r, _ := Compute(in)
	var got []string
	for _, it := range r.Backlog.Items {
		got = append(got, it.Source)
	}
	if strings.Join(got, ",") != "manual,plane,github,sdd" || r.Backlog.PlaneError != "" {
		t.Fatalf("sources = %v, backlog = %+v", got, r.Backlog)
	}
	want := BacklogItem{Source: "plane", ID: plane288, Repo: "-", Est: "-", Ref: plane288, Title: "plane one", AgeDays: 2, Code: "ZEN-288"}
	if r.Backlog.Items[1] != want {
		t.Fatalf("plane item = %+v, want %+v", r.Backlog.Items[1], want)
	}
}

// FR-5.3: a Plane error lands in planeError and leaves GitHub alone.
func TestBacklogPlaneErrorIsolated(t *testing.T) {
	in := base("")
	in.GitHub = func() ([]Issue, error) {
		return []Issue{{URL: issue1, Repo: "r", Title: "gh one", CreatedAt: now}}, nil
	}
	in.Plane = func() ([]PlaneItem, error) { return nil, errors.New("plane answered 401 Unauthorized") }
	r, _ := Compute(in)
	if r.Backlog.PlaneError != "plane answered 401 Unauthorized" || r.Backlog.GhError != "" || sources(r)["github"] != 1 {
		t.Fatalf("backlog = %+v", r.Backlog)
	}
	if b := Brief(r); b.PlaneError != "plane answered 401 Unauthorized" {
		t.Fatalf("brief planeError = %q", b.PlaneError)
	}
}

// FR-5.3 the other way: a GitHub error leaves Plane items alone.
func TestBacklogGitHubErrorKeepsPlane(t *testing.T) {
	in := base("")
	in.GitHub = func() ([]Issue, error) { return nil, errors.New("gh down") }
	in.Plane = func() ([]PlaneItem, error) {
		return []PlaneItem{{Key: "ZEN-288", URL: plane288, Title: "a", CreatedAt: now}}, nil
	}
	r, _ := Compute(in)
	if r.Backlog.GhError == "" || r.Backlog.PlaneError != "" || sources(r)["plane"] != 1 {
		t.Fatalf("backlog = %+v", r.Backlog)
	}
}

// SC-12 stats part: a Plane item taken by start --from or dropped leaves the backlog.
func TestBacklogPlaneHiddenByFromAndDrop(t *testing.T) {
	in := base(lines(
		row(now, "start", cid("fix", 1), "fix", "be", "S", plane288),
		row(now, "drop", plane256),
	))
	in.Plane = func() ([]PlaneItem, error) {
		return []PlaneItem{
			{Key: "ZEN-288", URL: plane288, Title: "a", CreatedAt: now},
			{Key: "ZEN-256", URL: plane256, Title: "b", CreatedAt: now},
			{Key: "ZEN-227", URL: "https://plane.example/zenify/browse/ZEN-227/", Title: "c", CreatedAt: now},
		}, nil
	}
	r, _ := Compute(in)
	if len(r.Backlog.Items) != 1 || r.Backlog.Items[0].Code != "ZEN-227" || r.Malformed != 0 {
		t.Fatalf("items = %+v malformed=%d", r.Backlog.Items, r.Malformed)
	}
}
