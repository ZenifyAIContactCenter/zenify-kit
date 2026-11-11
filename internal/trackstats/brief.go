package trackstats

// briefTitleMax caps a backlog title in Brief; longer titles end in "…".
const briefTitleMax = 80

// BriefResult is the agent view of Result (`zenify track stats --brief`): only what is needed to
// pick the next work item or build a daily plan, so the agent reads a few KB instead of the pane's
// full JSON (timing, tokens, estimate and quality stay out).
type BriefResult struct {
	Enabled       bool           `json:"enabled"`
	Onboarded     bool           `json:"onboarded"`
	Date          string         `json:"date"`
	Scale         BriefScale     `json:"scale"`
	Carried       []BriefCarried `json:"carried"`
	DailyPlan     *DailyPlan     `json:"dailyPlan"`
	PROpen        []BriefPR      `json:"prOpen"`
	PRStatusError string         `json:"prStatusError"`
	MergedError   string         `json:"mergedError"`
	Backlog       []BriefBacklog `json:"backlog"`
	GhError       string         `json:"ghError"`
}

// BriefScale holds the S/M/L upper bounds in minutes, used to size a daily plan.
type BriefScale struct {
	S int `json:"S"`
	M int `json:"M"`
	L int `json:"L"`
}

type BriefCarried struct {
	ID    string `json:"id"`
	Brief string `json:"brief"`
	Kind  string `json:"kind"`
	Repo  string `json:"repo"`
	Est   string `json:"est"`
	Phase string `json:"phase"`
}

type BriefPR struct {
	URL     string `json:"url"`
	Repo    string `json:"repo"`
	Number  int    `json:"number"`
	CI      string `json:"ci"`
	Review  string `json:"review"`
	Threads int    `json:"threads"`
	Ready   bool   `json:"ready"`
}

type BriefBacklog struct {
	ID      string `json:"id"`
	Source  string `json:"source"`
	Code    string `json:"code"`
	Repo    string `json:"repo"`
	Est     string `json:"est"`
	Title   string `json:"title"`
	AgeDays int    `json:"ageDays"`
}

// Brief projects r onto the agent view. Lists are never nil, so they marshal as [].
func Brief(r Result) BriefResult {
	b := BriefResult{
		Enabled:       r.Enabled,
		Onboarded:     r.Onboarded,
		Date:          r.Today.Date,
		Scale:         BriefScale{S: r.Scale.S, M: r.Scale.M, L: r.Scale.L},
		DailyPlan:     r.Today.DailyPlan,
		PRStatusError: r.Today.PRStatusError,
		MergedError:   r.Today.MergedError,
		GhError:       r.Backlog.GhError,
		Carried:       []BriefCarried{},
		PROpen:        []BriefPR{},
		Backlog:       []BriefBacklog{},
	}
	// Carried items are open (running or stale), so Running holds their estimate.
	est := map[string]string{}
	for _, it := range r.Running {
		est[it.ID] = it.Est
	}
	for _, c := range r.Kitchen.Carried {
		e := est[c.ID]
		if e == "" {
			e = "-"
		}
		b.Carried = append(b.Carried, BriefCarried{ID: c.ID, Brief: c.Brief, Kind: c.Kind, Repo: c.Repo, Est: e, Phase: c.Phase})
	}
	for _, p := range r.Today.PROpen {
		b.PROpen = append(b.PROpen, BriefPR{URL: p.URL, Repo: p.Repo, Number: p.Number, CI: p.CI, Review: p.Review, Threads: p.Threads, Ready: p.Ready})
	}
	for _, it := range r.Backlog.Items {
		b.Backlog = append(b.Backlog, BriefBacklog{ID: it.ID, Source: it.Source, Code: it.Code, Repo: it.Repo, Est: it.Est, Title: shorten(it.Title), AgeDays: it.AgeDays})
	}
	return b
}

func shorten(s string) string {
	r := []rune(s)
	if len(r) <= briefTitleMax {
		return s
	}
	return string(r[:briefTitleMax]) + "…"
}
