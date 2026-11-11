package trackstats

import (
	"sort"
	"strings"
	"time"
)

// Issue is one open GitHub issue handed to Compute by the CLI.
type Issue struct {
	URL       string
	Repo      string
	Title     string
	CreatedAt time.Time
}

// SDDItem is one deferred or parked finding from an SDD ledger. ID is the
// ledger path relative to the workspace plus ":<line>".
type SDDItem struct {
	ID      string
	Repo    string
	Title   string
	ModTime time.Time
}

// PlaneItem is one open Plane work item assigned to the developer. URL is the
// browse URL <plane>/<workspace>/browse/<Key>/.
type PlaneItem struct {
	Key       string
	URL       string
	Title     string
	CreatedAt time.Time
}

// BacklogItem is one row of the Backlog tab; Source is manual|plane|github|sdd|stale.
type BacklogItem struct {
	Source  string `json:"source"`
	ID      string `json:"id"`
	Repo    string `json:"repo"`
	Est     string `json:"est"`
	Ref     string `json:"ref"`
	Title   string `json:"title"`
	AgeDays int    `json:"ageDays"`
	Code    string `json:"code"`
}

type Backlog struct {
	Items      []BacklogItem `json:"items"`
	GhError    string        `json:"ghError"`
	PlaneError string        `json:"planeError"`
}

func ageDays(now, t time.Time) int {
	d := int(now.Sub(t) / (24 * time.Hour))
	if d < 0 {
		return 0
	}
	return d
}

// computeBacklog gathers manual, plane, github, sdd and stale items. from holds the
// ids and URLs already taken by a `start --from`, dropped the `drop` rows. It
// also returns the issues GitHub gave, so the source is asked once.
func computeBacklog(in Input, logEv []Event, from map[string]bool, items []Item) (Backlog, []Issue) {
	b := Backlog{Items: []BacklogItem{}}
	dropped := map[string]bool{}
	for _, e := range logEv {
		if e.Type == "drop" {
			dropped[e.ID] = true
		}
	}
	for _, e := range logEv {
		if e.Type != "backlog" || dropped[e.ID] || from[e.ID] {
			continue
		}
		b.Items = append(b.Items, BacklogItem{Source: "manual", ID: e.ID, Code: Code(e.ID), Repo: e.F[0], Est: e.F[1], Ref: e.F[2], Title: e.F[3], AgeDays: ageDays(in.Now, e.At)})
	}
	if in.Plane != nil {
		got, err := in.Plane()
		if err != nil {
			b.PlaneError = err.Error()
		}
		seen := map[string]bool{}
		for _, p := range got {
			if seen[p.URL] || from[p.URL] || dropped[p.URL] {
				continue
			}
			seen[p.URL] = true
			b.Items = append(b.Items, BacklogItem{Source: "plane", ID: p.URL, Code: Code(p.URL), Repo: "-", Est: "-", Ref: p.URL, Title: p.Title, AgeDays: ageDays(in.Now, p.CreatedAt)})
		}
	}
	var issues []Issue
	if in.GitHub != nil {
		var err error
		issues, err = in.GitHub()
		if err != nil {
			b.GhError = err.Error()
		}
		seen := map[string]bool{}
		for _, is := range issues {
			if seen[is.URL] || from[is.URL] || dropped[is.URL] {
				continue
			}
			seen[is.URL] = true
			b.Items = append(b.Items, BacklogItem{Source: "github", ID: is.URL, Code: Code(is.URL), Repo: is.Repo, Est: "-", Ref: is.URL, Title: is.Title, AgeDays: ageDays(in.Now, is.CreatedAt)})
		}
	}
	for _, s := range in.SDD {
		if dropped[s.ID] || from[s.ID] {
			continue
		}
		b.Items = append(b.Items, BacklogItem{Source: "sdd", ID: s.ID, Code: Code(s.ID), Repo: s.Repo, Est: "-", Ref: "-", Title: s.Title, AgeDays: ageDays(in.Now, s.ModTime)})
	}
	for _, it := range items {
		if it.Status != "stale" {
			continue
		}
		if beat, ok := in.Beats[it.Session]; ok && in.Now.Sub(beat) <= LiveTTL {
			continue // a live session still holds this item
		}
		started, _ := time.Parse(time.RFC3339, it.StartedAt) // written by Compute from a parsed time
		b.Items = append(b.Items, BacklogItem{Source: "stale", ID: it.ID, Code: Code(it.ID), Repo: it.Repo, Est: it.Est, Ref: "-", Title: it.Kind + " " + it.Repo + " (" + it.ID + ")", AgeDays: ageDays(in.Now, started)})
	}
	b.Items = dedupeAndSort(b.Items)
	return b, issues
}

// normTitle folds case and whitespace so "Fix login" matches "fix  Login".
func normTitle(t string) string {
	return strings.Join(strings.Fields(strings.ToLower(t)), " ")
}

// dedupeAndSort drops the manual items a github issue already covers, then
// orders each source (kept in its gathering order) by est S, M, L, "-" and, within
// one est, by age, oldest first.
func dedupeAndSort(items []BacklogItem) []BacklogItem {
	gh := map[string]bool{}
	for _, it := range items {
		if it.Source == "github" {
			gh[normTitle(it.Title)] = true
		}
	}
	out := items[:0]
	for _, it := range items {
		if it.Source == "manual" && gh[normTitle(it.Title)] {
			continue
		}
		out = append(out, it)
	}
	srcRank := map[string]int{"manual": 0, "plane": 1, "github": 2, "sdd": 3, "stale": 4}
	estRank := map[string]int{"S": 0, "M": 1, "L": 2}
	rank := func(e string) int {
		if r, ok := estRank[e]; ok {
			return r
		}
		return len(estRank)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Source != b.Source {
			return srcRank[a.Source] < srcRank[b.Source]
		}
		if rank(a.Est) != rank(b.Est) {
			return rank(a.Est) < rank(b.Est)
		}
		return a.AgeDays > b.AgeDays
	})
	return out
}
