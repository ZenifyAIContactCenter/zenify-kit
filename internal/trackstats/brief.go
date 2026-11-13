package trackstats

// briefTitleMax caps a backlog title in Brief; longer titles end in "…".
const briefTitleMax = 80

// BriefResult is the agent view of Result (`zenify track stats --brief`): only what is needed to
// pick the next work item or build a daily plan, so the agent reads a few KB instead of the pane's
// full JSON (timing, tokens, estimate and quality stay out).
type BriefResult struct {
	Enabled       bool             `json:"enabled"`
	Onboarded     bool             `json:"onboarded"`
	Date          string           `json:"date"`
	Scale         BriefScale       `json:"scale"`
	Carried       []BriefCarried   `json:"carried"`
	DailyPlan     *DailyPlan       `json:"dailyPlan"`
	PROpen        []BriefPR        `json:"prOpen"`
	PRStatusError string           `json:"prStatusError"`
	MergedError   string           `json:"mergedError"`
	Backlog       []BriefBacklog   `json:"backlog"`
	Running       []BriefRunning   `json:"running"`
	Elsewhere     []BriefElsewhere `json:"elsewhere"`
	GhError       string           `json:"ghError"`
	PlaneError    string           `json:"planeError"`
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

// BriefRunning is one open item, so the agent can find its own track id.
type BriefRunning struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Repo    string `json:"repo"`
	Est     string `json:"est"`
	Session string `json:"session"`
}

// BriefElsewhere is an open item that another live session is working on, so a daily plan must
// not pull it in.
type BriefElsewhere struct {
	ID      string `json:"id"`
	Code    string `json:"code"`
	From    string `json:"from"`
	Kind    string `json:"kind"`
	Repo    string `json:"repo"`
	Tab     string `json:"tab"`
	Session string `json:"session"`
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
// self is the caller's session id; an item live in any other session lands in Elsewhere and leaves
// Carried and PROpen.
func Brief(r Result, self string) BriefResult {
	b := BriefResult{
		Enabled:       r.Enabled,
		Onboarded:     r.Onboarded,
		Date:          r.Today.Date,
		Scale:         BriefScale{S: r.Scale.S, M: r.Scale.M, L: r.Scale.L},
		DailyPlan:     r.Today.DailyPlan,
		PRStatusError: r.Today.PRStatusError,
		MergedError:   r.Today.MergedError,
		GhError:       r.Backlog.GhError,
		PlaneError:    r.Backlog.PlaneError,
		Carried:       []BriefCarried{},
		PROpen:        []BriefPR{},
		Backlog:       []BriefBacklog{},
		Running:       []BriefRunning{},
		Elsewhere:     []BriefElsewhere{},
	}
	away, awayFrom := map[string]bool{}, map[string]bool{}
	// Carried items are open (running or stale), so Running holds their estimate.
	est := map[string]string{}
	for _, it := range r.Running {
		est[it.ID] = it.Est
	}
	for _, it := range r.Running {
		if it.Live && it.Session != self {
			e := BriefElsewhere{ID: it.ID, From: it.From, Kind: it.Kind, Repo: it.Repo, Tab: it.Tab, Session: it.Session}
			if it.From != "" {
				e.Code = Code(it.From)
				awayFrom[it.From] = true
			}
			b.Elsewhere = append(b.Elsewhere, e)
			away[it.ID] = true
		}
		b.Running = append(b.Running, BriefRunning{ID: it.ID, Kind: it.Kind, Repo: it.Repo, Est: it.Est, Session: it.Session})
	}
	for _, c := range r.Kitchen.Carried {
		if away[c.ID] {
			continue
		}
		e := est[c.ID]
		if e == "" {
			e = "-"
		}
		b.Carried = append(b.Carried, BriefCarried{ID: c.ID, Brief: c.Brief, Kind: c.Kind, Repo: c.Repo, Est: e, Phase: c.Phase})
	}
	for _, p := range r.Today.PROpen {
		if awayFrom[p.URL] {
			continue
		}
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
