package trackstats

import "strings"

// DailyPlanItem is one planned id with its progress. Status is todo, running, done or cancelled.
type DailyPlanItem struct {
	ID     string `json:"id"`
	Repo   string `json:"repo"`
	Est    string `json:"est"`
	Title  string `json:"title"`
	Status string `json:"status"`
	URL    string `json:"url"`
}

// DailyPlan is today's plan: EstMin sums the scale bounds of the estimated items, WallMin the
// wall minutes of the track items attached to the plan.
type DailyPlan struct {
	Items   []DailyPlanItem `json:"items"`
	EstMin  int             `json:"estMin"`
	WallMin int             `json:"wallMin"`
}

// todayPlanIDs returns the ids of the latest `plan` row dated today, nil when there is none.
func todayPlanIDs(logEv []Event, today string) []string {
	var ids []string
	for _, e := range logEv {
		if e.Type == "plan" && e.ID == today {
			ids = e.F
		}
	}
	return ids
}

// planPRIDs returns the PR URLs in today's plan, so mergedToday can ask about them.
func planPRIDs(logEv []Event, today string) []string {
	var out []string
	for _, id := range todayPlanIDs(logEv, today) {
		if isPRURL(id) {
			out = append(out, id)
		}
	}
	return out
}

func isPRURL(u string) bool { return strings.HasPrefix(u, "http") && strings.Contains(u, "/pull/") }

// planPRURLs returns the PR URLs of a computed plan.
func planPRURLs(p *DailyPlan) []string {
	var out []string
	if p == nil {
		return out
	}
	for _, it := range p.Items {
		if isPRURL(it.URL) {
			out = append(out, it.URL)
		}
	}
	return out
}

func planStatus(trackStatus string) string {
	switch trackStatus {
	case "running", "stale":
		return "running"
	case "done", "pushed":
		return "done"
	case "cancelled", "rollback":
		return "cancelled"
	}
	return "todo"
}

// computeDailyPlan builds today's plan from the latest plan row, or returns nil. items are the
// computed track items; brief is the kitchen title of a track item.
func computeDailyPlan(in Input, logEv []Event, items []Item, works map[string]*work, backlog []BacklogItem, mergedCache map[string]string, scale Scale, brief func(Item, bool) string) *DailyPlan {
	ids := todayPlanIDs(logEv, in.Today)
	if ids == nil {
		return nil
	}
	itemOf := map[string]Item{}
	from := map[string]Item{} // the first item started from a ref wins
	for _, it := range items {
		itemOf[it.ID] = it
		if f := works[it.ID].from; f != "" {
			if _, ok := from[f]; !ok {
				from[f] = it
			}
		}
	}
	type brow struct{ repo, est, title string }
	rows := map[string]brow{}
	for _, e := range logEv {
		if e.Type == "backlog" {
			rows[e.ID] = brow{e.F[0], e.F[1], e.F[3]}
		}
	}
	for _, b := range backlog {
		for _, k := range []string{b.ID, b.Ref} {
			if _, ok := rows[k]; !ok && k != "" && k != "-" {
				rows[k] = brow{b.Repo, b.Est, b.Title}
			}
		}
	}
	bound := map[string]int{"S": scale.S, "M": scale.M, "L": scale.L}
	plan := &DailyPlan{Items: []DailyPlanItem{}}
	counted := map[string]bool{}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		pi := DailyPlanItem{ID: id, Repo: "-", Est: "-", Title: id, Status: "todo"}
		if strings.HasPrefix(id, "http") {
			pi.URL = id
		}
		tr, tracked := itemOf[id]
		if !tracked {
			tr, tracked = from[id]
		}
		if r, ok := rows[id]; ok {
			pi.Repo, pi.Est, pi.Title = r.repo, r.est, r.title
		} else if t := in.PRTitles[id]; t != "" && !strings.HasPrefix(t, titleFailedPrefix) {
			pi.Title = t
		}
		if tracked {
			pi.Repo, pi.Status = tr.Repo, planStatus(tr.Status)
			if _, real := bound[tr.Est]; real {
				pi.Est = tr.Est
			}
			pi.Title = brief(tr, false)
			if !counted[tr.ID] {
				counted[tr.ID] = true
				plan.WallMin += tr.WallMin
				plan.EstMin += bound[pi.Est]
			}
		} else {
			if at, ok := mergedCache[id]; ok && isPRURL(id) && at != ClosedUnmerged {
				pi.Status = "done"
			}
			plan.EstMin += bound[pi.Est]
		}
		plan.Items = append(plan.Items, pi)
	}
	return plan
}
