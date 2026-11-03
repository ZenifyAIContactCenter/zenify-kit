package trackstats

import (
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

// BacklogItem is one row of the Backlog tab; Source is manual|github|sdd|stale.
type BacklogItem struct {
	Source  string `json:"source"`
	ID      string `json:"id"`
	Repo    string `json:"repo"`
	Est     string `json:"est"`
	Ref     string `json:"ref"`
	Title   string `json:"title"`
	AgeDays int    `json:"ageDays"`
}

type Backlog struct {
	Items   []BacklogItem `json:"items"`
	GhError string        `json:"ghError"`
}

func ageDays(now, t time.Time) int {
	d := int(now.Sub(t) / (24 * time.Hour))
	if d < 0 {
		return 0
	}
	return d
}

// computeBacklog gathers manual, github, sdd and stale items. from holds the
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
		b.Items = append(b.Items, BacklogItem{Source: "manual", ID: e.ID, Repo: e.F[0], Est: e.F[1], Ref: e.F[2], Title: e.F[3], AgeDays: ageDays(in.Now, e.At)})
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
			if seen[is.URL] || from[is.URL] {
				continue
			}
			seen[is.URL] = true
			b.Items = append(b.Items, BacklogItem{Source: "github", ID: is.URL, Repo: is.Repo, Est: "-", Ref: is.URL, Title: is.Title, AgeDays: ageDays(in.Now, is.CreatedAt)})
		}
	}
	for _, s := range in.SDD {
		if dropped[s.ID] {
			continue
		}
		b.Items = append(b.Items, BacklogItem{Source: "sdd", ID: s.ID, Repo: s.Repo, Est: "-", Ref: "-", Title: s.Title, AgeDays: ageDays(in.Now, s.ModTime)})
	}
	for _, it := range items {
		if it.Status != "stale" {
			continue
		}
		started, _ := time.Parse(time.RFC3339, it.StartedAt) // written by Compute from a parsed time
		b.Items = append(b.Items, BacklogItem{Source: "stale", ID: it.ID, Repo: it.Repo, Est: it.Est, Ref: "-", Title: it.Kind + " " + it.Repo + " (" + it.ID + ")", AgeDays: ageDays(in.Now, started)})
	}
	return b, issues
}
