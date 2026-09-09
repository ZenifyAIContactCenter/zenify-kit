package dbperf

import "testing"

func mk(text string) []AddedLine { return []AddedLine{{File: "f.js", Line: 1, Text: text}} }

func hasSignal(r Result, sig string, tier Tier) bool {
	for _, f := range r.Findings {
		if f.Signal == sig && f.Tier == tier {
			return true
		}
	}
	return false
}

func TestUnboundedListIsBlocking(t *testing.T) {
	r := ScanStatic(mk("Model.find({s:1}).sort({t:-1})"), Defaults())
	if r.SitesScanned != 1 {
		t.Fatalf("want 1 site, got %d", r.SitesScanned)
	}
	if !hasSignal(r, "unbounded-list", Blocking) {
		t.Fatalf("want unbounded-list BLOCKING: %+v", r.Findings)
	}
}

func TestSkipDeepIsBlocking(t *testing.T) {
	r := ScanStatic(mk("q.find(x).skip(5000).limit(20)"), Defaults())
	if !hasSignal(r, "skip-deep", Blocking) {
		t.Fatalf("want skip-deep BLOCKING: %+v", r.Findings)
	}
}

func TestSmallSkipNotFlagged(t *testing.T) {
	r := ScanStatic(mk("q.find(x).skip(20).limit(20)"), Defaults())
	if hasSignal(r, "skip-deep", Blocking) {
		t.Fatal("skip(20) must not be flagged")
	}
}

func TestRegexAndInAndNegationAreAdvisory(t *testing.T) {
	r := ScanStatic(mk("c.find({name:{$regex:'foo',$options:'i'},k:{$nin:[1,2]}})"), Defaults())
	if !hasSignal(r, "regex-unanchored", Advisory) || !hasSignal(r, "negation", Advisory) {
		t.Fatalf("want regex + negation ADVISORY: %+v", r.Findings)
	}
}

func TestSortWithLimitNotUnbounded(t *testing.T) {} // documents that .limit() suppresses unbounded-list; see TestSortWithLimitSuppressesUnbounded for the real assertion
func TestSortWithLimitSuppressesUnbounded(t *testing.T) {
	r := ScanStatic(mk("Model.find({s:1}).sort({t:-1}).limit(50)"), Defaults())
	if hasSignal(r, "unbounded-list", Blocking) {
		t.Fatal(".limit() present must suppress unbounded-list")
	}
}
