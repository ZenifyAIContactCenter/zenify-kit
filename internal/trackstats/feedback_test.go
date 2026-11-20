package trackstats

import (
	"strings"
	"testing"
	"time"
)

const fbURL = "https://github.com/o/r/pull/7"
const fbID = "cook-261119-aaaa"

func fbEvents(t *testing.T, ship ...string) []Event {
	t.Helper()
	rows := []string{"2026-11-19T01:00:00Z · start · " + fbID + " · cook · repo · M"}
	for i, s := range ship {
		rows = append(rows, "2026-11-19T02:0"+string(rune('0'+i))+":00Z · ship · "+fbID+" · "+s)
	}
	rows = append(rows, "2026-11-19T03:00:00Z · end · "+fbID+" · "+fbURL+" · 1 · 0")
	ev, bad := ParseLog(strings.Join(rows, "\n"))
	if bad != 0 {
		t.Fatalf("%d malformed rows", bad)
	}
	return ev
}

func fbThread(first string, replies ...string) ReviewThread {
	th := ReviewThread{Comments: []ReviewComment{{Body: first, AuthorType: "Bot"}}}
	for _, r := range replies {
		th.Comments = append(th.Comments, ReviewComment{Body: r})
	}
	return th
}

func fbInput(t *testing.T, snap *ReviewSnapshot, ship ...string) FeedbackInput {
	t.Helper()
	in := FeedbackInput{
		Day:       "2026-11-19",
		Merged:    []string{fbURL},
		Events:    fbEvents(t, ship...),
		Items:     []Item{{ID: fbID, Kind: "cook", Est: "M", Status: "done", WallMin: 60}},
		Scale:     Scale{S: 30, M: 90, L: 240},
		Ledgers:   map[string]string{},
		Snapshots: map[string]*ReviewSnapshot{},
	}
	if snap != nil {
		in.Snapshots[fbURL] = snap
	}
	return in
}

func wantLines(t *testing.T, got string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
}

// SC-6
func TestFeedback_EscapedHigh(t *testing.T) {
	snap := &ReviewSnapshot{Threads: []ReviewThread{
		fbThread("**HIGH** nil map write\nmore text"),
		fbThread("Severity: HIGH — race"),
	}}
	got := Feedback(fbInput(t, snap, "shipped · - · 0,0,1,0 · 1"))
	wantLines(t, got, "## Feedback", "### "+fbURL, "- internal review: 0,0,1,0 rounds 1",
		"threads CRITICAL 0 HIGH 2 MEDIUM 0 LOW 0", "escaped:")
}

// SC-7
func TestFeedback_NoMergedIsEmpty(t *testing.T) {
	in := fbInput(t, nil, "shipped · - · 0,0,0,0 · 0")
	in.Merged = nil
	if got := Feedback(in); got != "" {
		t.Fatalf("Feedback = %q, want empty", got)
	}
}

// SC-8, SC-12 (missing side)
func TestFeedback_MissingData(t *testing.T) {
	got := Feedback(fbInput(t, nil, "shipped · - · 0,0,0,0 · 0"))
	wantLines(t, got, "- ledger: none", "- review data: missing (.zenify/track/reviews/o__r__7.json)", "- run: "+fbID+" cook est M wall 60m")
	for _, no := range []string{"escaped:", "bot review:", "ledger counts:"} {
		if strings.Contains(got, no) {
			t.Errorf("unexpected %q in:\n%s", no, got)
		}
	}
}

// SC-11: the last ship row counts; findings "-" never raise escaped.
func TestFeedback_LastShipRow(t *testing.T) {
	snap := &ReviewSnapshot{Threads: []ReviewThread{fbThread("HIGH: leak")}}
	got := Feedback(fbInput(t, snap, "blocked · review · 1,0,0,0 · 1", "shipped · - · 0,0,0,0 · 2"))
	wantLines(t, got, "- internal review: 0,0,0,0 rounds 2", "escaped:")
	got = Feedback(fbInput(t, snap, "shipped · - · - · 0"))
	wantLines(t, got, "- internal review: -")
	if strings.Contains(got, "escaped:") {
		t.Errorf("escaped with findings -:\n%s", got)
	}
	// Internal review already caught a HIGH: nothing escaped.
	got = Feedback(fbInput(t, snap, "shipped · - · 0,1,0,0 · 1"))
	if strings.Contains(got, "escaped:") {
		t.Errorf("escaped though internal review had a HIGH:\n%s", got)
	}
}

// SC-12 (present side)
func TestFeedback_FilePaths(t *testing.T) {
	in := fbInput(t, &ReviewSnapshot{}, "shipped · - · 0,0,0,0 · 0")
	in.Ledgers[fbID] = "# SDD ledger\n"
	got := Feedback(in)
	wantLines(t, got, "- ledger: .zenify/track/ledgers/"+fbID+".md", "- review data: .zenify/track/reviews/o__r__7.json")
}

func TestFeedback_LedgerCounts(t *testing.T) {
	in := fbInput(t, nil, "shipped · - · 0,0,0,0 · 0")
	in.Ledgers[fbID] = strings.Join([]string{
		"# SDD ledger — plan: x",
		"Task 1 · fix round 1 · spec gap",
		"Task 1 · fix round 2 · test",
		"Task 2 · parked — waits for the API",
		"Ruling: keep the old envelope",
		"Task 3 · minor (deferred) naming",
	}, "\n")
	got := Feedback(in)
	wantLines(t, got, "- ledger counts: fix rounds 2, parked 1, rulings 1, deferred 1", "parked:", "ruling:")
}

func TestFeedback_EstimateMiss(t *testing.T) {
	in := fbInput(t, nil, "shipped · - · 0,0,0,0 · 0")
	in.Items[0].Est = "S" // 60m is M on a 30/90/240 scale
	wantLines(t, Feedback(in), "estimate miss:")
	in.Items[0].Est = "M"
	if got := Feedback(in); strings.Contains(got, "estimate miss:") {
		t.Errorf("estimate miss on a hit:\n%s", got)
	}
}

func TestFeedback_BotReview(t *testing.T) {
	created := time.Date(2026, 11, 19, 3, 0, 0, 0, time.UTC)
	snap := &ReviewSnapshot{
		CreatedAt: created.Format(time.RFC3339),
		Commits: []ReviewCommit{
			{OID: "a", CommittedDate: created.Add(-time.Hour).Format(time.RFC3339)},
			{OID: "b", CommittedDate: created.Add(time.Hour).Format(time.RFC3339)},
			{OID: "c", CommittedDate: created.Add(2 * time.Hour).Format(time.RFC3339)},
		},
		Threads: []ReviewThread{
			fbThread("MEDIUM naming", "Declined: naming — matches the repo"),
			fbThread("LOW typo\nHIGH on the second line does not count", "fixed"),
			fbThread("CRITICAL then HIGH"),
		},
	}
	got := Feedback(fbInput(t, snap, "shipped · - · 0,0,0,0 · 0"))
	wantLines(t, got, "- bot review: push rounds 2, threads CRITICAL 1 HIGH 0 MEDIUM 1 LOW 1, Declined: 1")
}

// A ledger keyed by an id that is not a track id is never named as a path.
func TestFeedback_LedgerIDRevalidated(t *testing.T) {
	bad := "../../etc/passwd"
	in := fbInput(t, nil, "shipped · - · 0,0,0,0 · 0")
	in.Events = append(in.Events, Event{Type: "end", ID: bad, F: []string{fbURL, "1", "0"}})
	in.Ledgers[bad] = "Ruling: x\n"
	got := Feedback(in)
	wantLines(t, got, "- ledger: none")
	for _, no := range []string{"ledgers/" + bad, "ledger counts:"} {
		if strings.Contains(got, no) {
			t.Errorf("unexpected %q in:\n%s", no, got)
		}
	}
}

// A thread a person opened is not bot review: no escaped:, counted on its own line.
func TestFeedback_HumanThreadNotBot(t *testing.T) {
	human := ReviewThread{Comments: []ReviewComment{{Body: "HIGH: race", AuthorType: "User"}}}
	snap := &ReviewSnapshot{Threads: []ReviewThread{human, fbThread("LOW typo")}}
	got := Feedback(fbInput(t, snap, "shipped · - · 0,0,0,0 · 0"))
	wantLines(t, got, "threads CRITICAL 0 HIGH 0 MEDIUM 0 LOW 1", "- human review: threads 1")
	if strings.Contains(got, "escaped:") {
		t.Errorf("escaped from a human thread:\n%s", got)
	}
	if got := Feedback(fbInput(t, &ReviewSnapshot{Threads: []ReviewThread{fbThread("LOW x")}}, "shipped · - · 0,0,0,0 · 0")); strings.Contains(got, "human review:") {
		t.Errorf("human line without human threads:\n%s", got)
	}
}

// A run that ended before the merge day is not in the day's items: its wall comes from the log.
func TestFeedback_EstimateFromLogOnLaterMerge(t *testing.T) {
	rows := []string{
		"2026-11-18T01:00:00Z · start · " + fbID + " · cook · repo · M",
		"2026-11-18T03:00:00Z · end · " + fbID + " · " + fbURL + " · 1 · 0",
	}
	ev, bad := ParseLog(strings.Join(rows, "\n"))
	if bad != 0 {
		t.Fatalf("%d malformed rows", bad)
	}
	in := fbInput(t, nil)
	in.Events, in.Items = ev, nil
	wantLines(t, Feedback(in), "- run: "+fbID+" cook est M wall 120m", "estimate miss: est M, wall 120m is L")
}

// The log-recovered wall rounds up like Compute: 14m30s is 15m, M on the default scale.
func TestFeedback_LogWallRoundsLikeCompute(t *testing.T) {
	rows := []string{
		"2026-11-18T01:00:00Z · start · " + fbID + " · cook · repo · S",
		"2026-11-18T01:14:30Z · end · " + fbID + " · " + fbURL + " · 1 · 0",
	}
	ev, bad := ParseLog(strings.Join(rows, "\n"))
	if bad != 0 {
		t.Fatalf("%d malformed rows", bad)
	}
	in := fbInput(t, nil)
	in.Events, in.Items = ev, nil
	in.Scale = Scale{S: defaultS, M: defaultM, L: defaultL}
	wantLines(t, Feedback(in), "wall 15m", "estimate miss: est S, wall 15m is M")
}
