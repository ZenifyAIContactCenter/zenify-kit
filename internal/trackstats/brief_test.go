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
		Backlog: Backlog{Items: []BacklogItem{{Source: "github", ID: "https://github.com/o/r/issues/3", Repo: "r", Est: "-", Ref: "https://github.com/o/r/issues/3", Title: long, AgeDays: 4, Code: "#3"}}, GhError: "gh down"},
	}
	b := Brief(r, "")
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
		`"prStatusError":"boom"`, `"ghError":"gh down"`, `"running":[{"id":"cook-261104-bbbb","kind":"","repo":"","est":"L","session":""}]`, `"source":"github"`, `"code":"#3"`, `"ageDays":4`} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in %s", want, s)
		}
	}
	for _, gone := range []string{`"quality"`, `"estimate"`, `"items":[{"id":"cook-261105-aaaa"`, `"cwd"`, `"createdAt"`, `"ref"`} {
		if strings.Contains(s, gone) {
			t.Errorf("brief must not carry %s: %s", gone, s)
		}
	}
}

// Empty lists stay arrays so an agent never has to handle null.
func TestBriefEmpty(t *testing.T) {
	out, err := json.Marshal(Brief(Result{Enabled: true}, ""))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"carried":[]`, `"prOpen":[]`, `"backlog":[]`, `"running":[]`, `"dailyPlan":null`, `"elsewhere":[]`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %s in %s", want, out)
		}
	}
}

func elsewhereResult(run Item) Result {
	return Result{
		Running: []Item{run},
		Kitchen: Kitchen{Carried: []CarriedItem{{ID: run.ID}}},
	}
}

func TestBriefElsewhere(t *testing.T) {
	const id = "cook-261112-aaaa"
	live := func(session string) Item {
		return Item{ID: id, Kind: "cook", Repo: "r", Session: session, Live: true}
	}
	t.Run("SC-1 live in another session", func(t *testing.T) {
		b := Brief(elsewhereResult(live("B")), "A")
		if len(b.Carried) != 0 {
			t.Fatalf("carried = %+v", b.Carried)
		}
		want := BriefElsewhere{ID: id, Kind: "cook", Repo: "r", Session: "B"}
		if len(b.Elsewhere) != 1 || b.Elsewhere[0] != want {
			t.Fatalf("elsewhere = %+v", b.Elsewhere)
		}
	})
	t.Run("SC-2 beat expired", func(t *testing.T) {
		it := live("B")
		it.Live = false
		b := Brief(elsewhereResult(it), "A")
		if len(b.Carried) != 1 || len(b.Elsewhere) != 0 {
			t.Fatalf("carried %+v elsewhere %+v", b.Carried, b.Elsewhere)
		}
	})
	t.Run("SC-3 own session", func(t *testing.T) {
		b := Brief(elsewhereResult(live("A")), "A")
		if len(b.Carried) != 1 || len(b.Elsewhere) != 0 {
			t.Fatalf("carried %+v elsewhere %+v", b.Carried, b.Elsewhere)
		}
	})
	t.Run("FR-1.4 empty self", func(t *testing.T) {
		b := Brief(elsewhereResult(live("A")), "")
		if len(b.Elsewhere) != 1 {
			t.Fatalf("elsewhere = %+v", b.Elsewhere)
		}
	})
	t.Run("SC-5 code only for a trackable From", func(t *testing.T) {
		b := Brief(elsewhereResult(live("B")), "A")
		if e := b.Elsewhere[0]; e.From != "" || e.Code != "" {
			t.Fatalf("no from: %+v", e)
		}
		it := live("B")
		it.From = "repos/x/.znf/sdd/p/progress.md:3"
		b = Brief(elsewhereResult(it), "A")
		if e := b.Elsewhere[0]; e.From != it.From || e.Code != "" {
			t.Fatalf("ledger from: %+v", e)
		}
	})
	t.Run("SC-8 stale but live", func(t *testing.T) {
		it := live("B")
		it.Status = "stale"
		if b := Brief(elsewhereResult(it), "A"); len(b.Elsewhere) != 1 {
			t.Fatalf("elsewhere = %+v", b.Elsewhere)
		}
	})
	t.Run("SC-9 PR of away item leaves prOpen", func(t *testing.T) {
		it := live("B")
		it.From = "https://github.com/o/r/pull/9"
		r := elsewhereResult(it)
		r.Today.PROpen = []PRStatus{{URL: "https://github.com/o/r/pull/9"}, {URL: "https://github.com/o/r/pull/10"}}
		b := Brief(r, "A")
		if len(b.PROpen) != 1 || b.PROpen[0].URL != "https://github.com/o/r/pull/10" {
			t.Fatalf("prOpen = %+v", b.PROpen)
		}
		if b.Elsewhere[0].Code != "#9" {
			t.Fatalf("code = %q", b.Elsewhere[0].Code)
		}
	})
}
