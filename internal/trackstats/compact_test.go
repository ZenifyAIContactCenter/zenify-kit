package trackstats

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

var cNow = time.Date(2026, 11, 30, 12, 0, 0, 0, time.UTC)

const cToday = "2026-11-30"

func cAt(day, h int) time.Time { return time.Date(2026, 11, day, h, 0, 0, 0, time.UTC) }

func ccid(kind string, n int) string { return fmt.Sprintf("%s-261101-%04x", kind, n) }

// fillers are n recent ended items of kind (started and ended after the cut, PR cached), so
// the older items of a case fall out of every stat window of that kind.
func fillers(kind string, n int, cache map[string]string) []string {
	var out []string
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("%s-261120-%04x", kind, i)
		start := cAt(20, 0).Add(time.Duration(i) * time.Hour)
		pr := fmt.Sprintf("https://github.com/o/r/pull/f%d", i)
		cache[pr] = "2026-11-21T00:00:00Z"
		out = append(out, row(start, "start", id, kind, "repo", "S"), row(start.Add(10*time.Minute), "end", id, pr, "1", "1"))
	}
	return out
}

func TestCompactPicksRows(t *testing.T) {
	merged := "2026-11-02T00:00:00Z"
	a, b, c, d := ccid("cook", 1), ccid("cook", 2), ccid("cook", 3), ccid("cook", 4)
	e, f, g, h := ccid("cook", 5), ccid("cook", 6), ccid("cook", 7), ccid("cook", 8)
	cases := []struct {
		name   string
		filler string // kind of the 61 recent fillers
		rows   []string
		cache  map[string]string
		moved  []int // indexes into rows
	}{
		{"history moves", "cook", []string{row(cAt(1, 9), "history", "deadbeef", "cook", "repo", "3", "-")}, nil, []int{0}},
		{"old turn with cached PR moves", "cook", []string{
			row(cAt(1, 8), "start", a, "cook", "repo", "M"),
			row(cAt(1, 8), "est", a, "L"),
			row(cAt(1, 9), "ship", a, "shipped", "review", "0,1,0,0", "1"),
			row(cAt(1, 9), "ci", a, "green"),
			row(cAt(1, 10), "end", a, "https://github.com/o/r/pull/1", "2", "1"),
		}, map[string]string{"https://github.com/o/r/pull/1": merged}, []int{0, 1, 2, 3, 4}},
		{"old turn with uncached PR stays", "cook", []string{
			row(cAt(1, 8), "start", a, "cook", "repo", "M"),
			row(cAt(1, 10), "end", a, "https://github.com/o/r/pull/1", "2", "1"),
		}, nil, nil},
		{"old turn with closed PR moves", "cook", []string{
			row(cAt(1, 8), "start", a, "cook", "repo", "M"),
			row(cAt(1, 10), "end", a, "https://github.com/o/r/pull/1", "2", "1"),
		}, map[string]string{"https://github.com/o/r/pull/1": "closed"}, []int{0, 1}},
		{"from issue URL stays (SC-4)", "cook", []string{
			row(cAt(1, 8), "start", b, "cook", "repo", "M", "https://github.com/o/r/issues/1"),
			row(cAt(1, 10), "end", b, "branch:x", "1", "1"),
		}, nil, nil},
		{"from SDD ledger stays", "cook", []string{
			row(cAt(1, 8), "start", c, "cook", "repo", "M", "repos/x/progress.md:3"),
			row(cAt(1, 10), "end", c, "branch:x", "1", "1"),
		}, nil, nil},
		{"old turn from a bid moves with its backlog row", "cook", []string{
			row(cAt(1, 7), "backlog", "b-261101-aaaa", "repo", "M", "-", "do the thing"),
			row(cAt(1, 8), "start", d, "cook", "repo", "M", "b-261101-aaaa"),
			row(cAt(1, 10), "end", d, "branch:x", "1", "1"),
		}, nil, []int{0, 1, 2}},
		{"recent turn from a bid stays with its backlog row", "cook", []string{
			row(cAt(1, 7), "backlog", "b-261101-aaaa", "repo", "M", "-", "do the thing"),
			row(cAt(25, 8), "start", d, "cook", "repo", "M", "b-261101-aaaa"),
			row(cAt(25, 10), "end", d, "branch:x", "1", "1"),
		}, nil, nil},
		{"drop with no backlog row stays", "cook", []string{row(cAt(1, 9), "drop", "repos/x/progress.md:9")}, nil, nil},
		{"old drop moves with its backlog row", "cook", []string{
			row(cAt(1, 7), "backlog", "b-261101-bbbb", "repo", "S", "-", "other thing"),
			row(cAt(2, 7), "drop", "b-261101-bbbb"),
		}, nil, []int{0, 1}},
		{"recent drop keeps its backlog row", "cook", []string{
			row(cAt(1, 7), "backlog", "b-261101-bbbb", "repo", "S", "-", "other thing"),
			row(cAt(25, 7), "drop", "b-261101-bbbb"),
		}, nil, nil},
		{"old turn in the 30 newest cook by start stays", "fix", []string{
			row(cAt(1, 8), "start", e, "cook", "repo", "M"),
			row(cAt(1, 8), "end", e, "branch:x", "1", "1"),
		}, nil, nil},
		{"old cancelled turn moves", "cook", []string{
			row(cAt(1, 8), "start", f, "cook", "repo", "M"),
			row(cAt(1, 9), "end", f, "cancelled", "-", "-"),
		}, nil, []int{0, 1}},
		{"running old turn stays", "cook", []string{row(cAt(1, 8), "start", g, "cook", "repo", "M")}, nil, nil},
		{"recent end stays", "cook", []string{
			row(cAt(1, 8), "start", h, "cook", "repo", "M"),
			row(cAt(20, 8), "end", h, "branch:x", "1", "1"),
		}, nil, nil},
		{"plan before the cut day moves, cut day stays", "cook", []string{
			row(cAt(1, 7), "plan", "2026-11-01", "b-261101-cccc"),
			row(cAt(16, 7), "plan", "2026-11-16", "b-261101-cccc"),
		}, nil, []int{0}},
		{"onboard, malformed, comment and blank stay", "cook", []string{
			row(cAt(1, 7), "onboard", "memory"),
			row(cAt(1, 7), "nonsense", "x"),
			"# a comment",
			"",
		}, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cache := map[string]string{}
			for k, v := range tc.cache {
				cache[k] = v
			}
			all := append(append([]string(nil), tc.rows...), fillers(tc.filler, maxLearn+1, cache)...)
			log := strings.Join(all, "\n") + "\n"
			keep, moved, ids := Compact(Input{Log: log, Now: cNow, Today: cToday, Cache: cache, Sessions: map[string]string{}}, 14)

			var wantMoved, wantKeep []string
			isMoved := map[int]bool{}
			for _, i := range tc.moved {
				isMoved[i] = true
			}
			for i, r := range all {
				if isMoved[i] {
					wantMoved = append(wantMoved, r)
				} else {
					wantKeep = append(wantKeep, r)
				}
			}
			if !reflect.DeepEqual(moved, wantMoved) {
				t.Fatalf("moved =\n%s\nwant\n%s", strings.Join(moved, "\n"), strings.Join(wantMoved, "\n"))
			}
			if want := strings.Join(wantKeep, "\n") + "\n"; keep != want {
				t.Fatalf("keep =\n%s\nwant\n%s", keep, want)
			}
			for _, r := range wantMoved {
				p := strings.Split(r, sep)
				if idRe.MatchString(p[2]) && !ids[p[2]] {
					t.Fatalf("movedIDs %v lacks %s", ids, p[2])
				}
			}
		})
	}
}

// scItem is the fixture metadata of one generated turn, known to the test without Compute.
type scItem struct {
	id       string
	end      time.Time // zero: no end row
	cachedPR bool
	from     string
}

// scFixture builds 120 cook/fix/hotfix turns over 60 days (more measured ones than maxLearn)
// with session timing, plus backlog, drop, plan, history, onboard, comment, blank and
// malformed lines.
func scFixture() (log string, sessions map[string]string, cache map[string]string, meta []scItem) {
	cache = map[string]string{}
	sessions = map[string]string{}
	var lg []string
	lg = append(lg, "# zenify track log", "", row(cNow.Add(-59*24*time.Hour), "onboard", "memory"))
	t0 := cNow.Add(-60 * 24 * time.Hour)
	ests := []string{"S", "M", "L", "-"}
	for i := 0; i < 120; i++ {
		kind := "cook"
		switch i % 6 {
		case 3, 4:
			kind = "fix"
		case 5:
			kind = "hotfix"
		}
		id := fmt.Sprintf("%s-261001-%04x", kind, i)
		start := t0.Add(time.Duration(i) * 12 * time.Hour)
		dur := time.Duration(5+(i*37)%300) * time.Minute
		if i%11 == 0 {
			dur = 3 * 24 * time.Hour // a long turn: end order differs from start order
		}
		if i%17 == 0 {
			dur = 0 // zero wall time: no estimate sample
		}
		it := scItem{id: id}
		var from string
		switch {
		case i%9 == 0:
			from = fmt.Sprintf("b-261001-%04x", i)
			lg = append(lg, row(start.Add(-time.Hour), "backlog", from, "repo", "M", "-", "backlog "+from))
		case i%13 == 0:
			from = fmt.Sprintf("https://github.com/o/r/issues/%d", i)
		case i%19 == 0:
			from = fmt.Sprintf("repos/x/progress.md:%d", i)
		}
		it.from = from
		startRow := []string{"start", id, kind, "repo", ests[i%4]}
		if from != "" {
			startRow = append(startRow, from)
		}
		lg = append(lg, row(start, startRow...))
		if i%5 == 0 {
			lg = append(lg, row(start.Add(time.Minute), "est", id, "M"))
		}
		end := start.Add(dur)
		running := i == 20 || i == 41 || i == 118 // 20, 41: stale carried; 118: carried from yesterday
		if !running && end.Before(cNow) {
			lg = append(lg, row(end, "ship", id, []string{"shipped", "blocked"}[i%2], "review", fmt.Sprintf("%d,0,1,0", i%3), fmt.Sprint(i%3)))
			lg = append(lg, row(end, "ci", id, []string{"green", "red"}[i%3%2]))
			if kind != "cook" {
				lg = append(lg, row(end, "fix", id, "1", "narrow", "test"))
			}
			var outcome string
			switch {
			case i%8 == 0:
				outcome = "cancelled"
			case i%10 == 0:
				outcome = "branch:feat-x"
			default:
				pr := fmt.Sprintf("https://github.com/o/r/pull/%d", i)
				outcome = pr
				switch {
				case i%7 == 0: // not cached: still waiting
				case i%12 == 0:
					cache[pr] = ClosedUnmerged
					it.cachedPR = true
				default:
					cache[pr] = end.Add(time.Hour).UTC().Format(time.RFC3339)
					it.cachedPR = true
				}
			}
			if outcome != "cancelled" && !strings.HasPrefix(outcome, "http") {
				it.cachedPR = true // no PR to wait for
			}
			lg = append(lg, row(end, "end", id, outcome, "2", "1"))
			it.end = end
		}
		if i == 118 {
			lg = append(lg, row(start.Add(time.Hour), "backlog", "b-261001-ffff", "repo", "S", "-", "carried title"))
		}
		meta = append(meta, it)
		sessions[fmt.Sprintf("s%02d", i)] = lines(
			row(start, "cwd", "/ws/repos/repo/.worktrees/x"),
			row(start, "bind", id),
			agentTurn(start.Add(2*time.Minute), id, 1+i%4),
		)
	}
	// Backlog rows not taken: an old dropped one, a recently dropped one, an open old one.
	lg = append(lg,
		row(t0, "backlog", "b-261001-d001", "repo", "S", "-", "dropped long ago"),
		row(t0.Add(24*time.Hour), "drop", "b-261001-d001"),
		row(t0, "backlog", "b-261001-d002", "repo", "S", "-", "dropped lately"),
		row(cNow.Add(-2*24*time.Hour), "drop", "b-261001-d002"),
		row(t0, "backlog", "b-261001-d003", "repo", "L", "-", "still open"),
		row(t0, "drop", "repos/x/progress.md:999"),
		row(t0, "history", "0000beef", "cook", "repo", "4", "opus:10/20"),
		row(t0, "history", "0001beef", "-", "repo", "2", "-"),
		row(t0, "plan", "2026-10-01", "b-261001-d003"),
		row(cNow, "plan", cToday, "b-261001-d003"),
		"this line is malformed",
		"",
	)
	return strings.Join(lg, "\n") + "\n", sessions, cache, meta
}

func TestCompactKeepsStats(t *testing.T) { // SC-3
	log, sessions, cache, meta := scFixture()
	in := Input{Log: log, Sessions: sessions, Cache: cache, Now: cNow, Today: cToday, Workspace: "/ws"}
	keep, moved, ids := Compact(in, 14)

	before, _ := Compute(in)
	after := in
	after.Log = keep
	got, _ := Compute(after)
	for _, f := range []struct {
		name      string
		want, got any
	}{
		{"Scale", before.Scale, got.Scale},
		{"Estimate", before.Estimate, got.Estimate},
		{"Quality", before.Quality, got.Quality},
		{"Backlog.Items", before.Backlog.Items, got.Backlog.Items},
		{"Kitchen.Carried", before.Kitchen.Carried, got.Kitchen.Carried},
		{"Today.PRsWaiting", before.Today.PRsWaiting, got.Today.PRsWaiting},
	} {
		if !reflect.DeepEqual(f.want, f.got) {
			t.Errorf("%s changed:\nbefore %+v\nafter  %+v", f.name, f.want, f.got)
		}
	}

	orig := strings.Split(log, "\n")
	kept := strings.Split(keep, "\n")
	if len(kept)+len(moved) != len(orig) {
		t.Fatalf("keep %d + moved %d lines != original %d", len(kept), len(moved), len(orig))
	}
	// Every original line is in keep or moved, in order.
	ki, mi := 0, 0
	for _, l := range orig {
		switch {
		case ki < len(kept) && kept[ki] == l:
			ki++
		case mi < len(moved) && moved[mi] == l:
			mi++
		default:
			t.Fatalf("line %q is neither kept nor moved in order", l)
		}
	}

	var movedStart, movedHistory bool
	for _, l := range moved {
		p := strings.Split(l, sep)
		movedStart = movedStart || (p[1] == "start" && idRe.MatchString(p[2]))
		movedHistory = movedHistory || p[1] == "history"
	}
	if !movedStart || !movedHistory {
		t.Fatalf("moved lacks a track start or a history row: %v", moved)
	}
	// The fixture must have old, otherwise movable turns kept only by a stat window,
	// or the equality above proves nothing about the windows.
	cut := cNow.Add(-14 * 24 * time.Hour)
	var keptByWindow, movedOld int
	for _, it := range meta {
		if it.end.IsZero() || !it.end.Before(cut) || !it.cachedPR || strings.HasPrefix(it.from, "http") || strings.HasPrefix(it.from, "repos/") {
			continue
		}
		if ids[it.id] {
			movedOld++
		} else {
			keptByWindow++
		}
	}
	if keptByWindow == 0 || movedOld == 0 {
		t.Fatalf("fixture too weak: %d old turns kept by a window, %d moved", keptByWindow, movedOld)
	}
}
