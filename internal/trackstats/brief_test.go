package trackstats

import (
	"encoding/json"
	"strings"
	"testing"
)

// Brief keeps only what an agent needs to pick or plan work, and shortens long titles.
func TestBrief(t *testing.T) {
	long := strings.Repeat("x", 120)
	r := Result{
		Enabled:   true,
		Onboarded: true,
		Scale:     Scale{Source: "default", N: 3, S: 15, M: 45, L: 120},
		Today: Today{
			Date:          "2026-11-05",
			DailyPlan:     &DailyPlan{Items: []DailyPlanItem{{ID: "b-261105-ab12", Repo: "kit", Est: "S", Title: "t", Status: "todo"}}, EstMin: 15},
			PROpen:        []PRStatus{{URL: "https://github.com/o/r/pull/1", Repo: "r", Number: 1, CreatedAt: "2026-11-01T00:00:00Z", CI: "failure", Review: "-", Threads: 2}},
			PRStatusError: "boom",
			MergedError:   "merged boom",
			Items:         []Item{{ID: "cook-261105-aaaa"}},
		},
		Running: []Item{{ID: "cook-261104-bbbb", Est: "L"}},
		Kitchen: Kitchen{Carried: []CarriedItem{{ID: "cook-261104-bbbb", Brief: "carry", Kind: "cook", Repo: "kit", Phase: "plan", Session: "s"}}},
		Backlog: Backlog{Items: []BacklogItem{{Source: "github", ID: "https://github.com/o/r/issues/3", Repo: "r", Est: "-", Ref: "https://github.com/o/r/issues/3", Title: long, AgeDays: 4}}, GhError: "gh down"},
	}
	b := Brief(r)
	if got := b.Backlog[0].Title; got != strings.Repeat("x", 80)+"…" {
		t.Fatalf("title = %q", got)
	}
	out, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{`"enabled":true`, `"onboarded":true`, `"date":"2026-11-05"`, `"scale":{"S":15,"M":45,"L":120}`,
		`"carried":[{"id":"cook-261104-bbbb","brief":"carry","kind":"cook","repo":"kit","est":"L","phase":"plan"}]`, `"mergedError":"merged boom"`,
		`"dailyPlan":{"items":[`, `"prOpen":[{"url":"https://github.com/o/r/pull/1","repo":"r","number":1,"ci":"failure","review":"-","threads":2,"ready":false}]`,
		`"prStatusError":"boom"`, `"ghError":"gh down"`, `"source":"github"`, `"ageDays":4`} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in %s", want, s)
		}
	}
	for _, gone := range []string{`"quality"`, `"estimate"`, `"items":[{"id":"cook-261105-aaaa"`, `"session"`, `"createdAt"`, `"ref"`} {
		if strings.Contains(s, gone) {
			t.Errorf("brief must not carry %s: %s", gone, s)
		}
	}
}

// Empty lists stay arrays so an agent never has to handle null.
func TestBriefEmpty(t *testing.T) {
	out, err := json.Marshal(Brief(Result{Enabled: true}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"carried":[]`, `"prOpen":[]`, `"backlog":[]`, `"dailyPlan":null`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %s in %s", want, out)
		}
	}
}
