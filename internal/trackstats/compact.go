package trackstats

import (
	"strings"
	"time"
)

// turnRows is what Compact needs to know about one track id's rows.
type turnRows struct {
	started  bool
	end      *time.Time // the first end with a valid outcome, as Compute reads it
	prsKnown bool       // every PR of every end row is in the merged cache
	recentPR bool       // a PR of an end row merged on or after the cut
	fromOK   bool       // every start row came from nothing or a backlog id
	from     []string   // the backlog ids its start rows came from
}

// Compact picks the log.md rows to archive: rows older than keepDays that no stat,
// backlog or brief still reads. keep is the new log.md text (order, blank and comment
// lines kept), moved the archived lines in file order, movedIDs the archived track ids.
// It reads in.Log, in.Sessions, in.Cache, in.Now and in.Today only.
func Compact(in Input, keepDays int) (keep string, moved []string, movedIDs map[string]bool) {
	res, _ := Compute(Input{Log: in.Log, Sessions: in.Sessions, Cache: in.Cache, Now: in.Now, Today: in.Today})
	cut := in.Now.Add(-time.Duration(keepDays) * 24 * time.Hour)
	cutDay := cut.In(in.Now.Location()).Format(dateLayout)

	lines := strings.Split(in.Log, "\n")
	evs := make([]*Event, len(lines))
	turns := map[string]*turnRows{}
	hasBacklog := map[string]bool{}
	oldDrop := map[string]bool{}
	planned := map[string]bool{} // ids a kept plan row names: computeDailyPlan still reads them
	for i, raw := range lines {
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		e, ok := parseLogLine(raw)
		if !ok {
			continue // malformed: kept
		}
		evs[i] = &e
		switch e.Type {
		case "backlog":
			hasBacklog[e.ID] = true
			continue
		case "drop":
			if e.At.Before(cut) {
				oldDrop[e.ID] = true
			}
			continue
		case "plan":
			if e.ID >= cutDay {
				for _, id := range e.F {
					planned[id] = true
				}
			}
			continue
		}
		if noIDRow[e.Type] {
			continue
		}
		t := turns[e.ID]
		if t == nil {
			t = &turnRows{prsKnown: true, fromOK: true}
			turns[e.ID] = t
		}
		switch e.Type {
		case "start":
			t.started = true
			if len(e.F) == 4 && e.F[3] != "-" {
				if bidRe.MatchString(e.F[3]) {
					t.from = append(t.from, e.F[3])
				} else {
					t.fromOK = false
				}
			}
		case "end":
			if t.end == nil && validOutcome(e.F[0]) {
				at := e.At
				t.end = &at
			}
			for _, p := range strings.Split(e.F[0], ",") {
				at, cached := in.Cache[p]
				if strings.HasPrefix(p, "http") && !cached {
					t.prsKnown = false
				}
				// mergedToday still counts a PR merged inside the window.
				if m, err := time.Parse(time.RFC3339, at); cached && at != ClosedUnmerged && err == nil && !m.Before(cut) {
					t.recentPR = true
				}
			}
		}
	}

	movedIDs = map[string]bool{}
	movedFrom := map[string]bool{}
	keptFrom := map[string]bool{}
	for id, t := range turns {
		if t.started && t.end != nil && t.end.Before(cut) && t.prsKnown && !t.recentPR && t.fromOK && !res.windowIDs[id] && !planned[id] && !anyPlanned(planned, t.from) {
			movedIDs[id] = true
			for _, b := range t.from {
				movedFrom[b] = true
			}
			continue
		}
		for _, b := range t.from {
			keptFrom[b] = true
		}
	}
	// A backlog row goes with its archived start or its old drop, unless a kept turn still
	// reads it (its from hides the row and titles the turn's brief).
	backlogMoved := func(bid string) bool {
		return hasBacklog[bid] && !keptFrom[bid] && (movedFrom[bid] || oldDrop[bid])
	}

	var kept []string
	for i, l := range lines {
		e := evs[i]
		move := false
		if e != nil {
			switch e.Type {
			case "history":
				move = true
			case "backlog", "drop":
				move = backlogMoved(e.ID)
			case "plan":
				move = e.ID < cutDay
			case "onboard":
			default:
				move = movedIDs[e.ID]
			}
		}
		if move {
			moved = append(moved, l)
		} else {
			kept = append(kept, l)
		}
	}
	return strings.Join(kept, "\n"), moved, movedIDs
}

func anyPlanned(planned map[string]bool, ids []string) bool {
	for _, id := range ids {
		if planned[id] {
			return true
		}
	}
	return false
}
