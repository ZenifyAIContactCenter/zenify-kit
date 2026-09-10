package dbperf

import (
	"strings"
	"testing"
)

func TestWaiveDowngradesBlockingWithReason(t *testing.T) {
	r := ScanStatic(mk("db.collection('tickets').find({_id:x}) // znf:db-perf-ok: tenant injected in middleware"), cfgLists())
	found := false
	for _, f := range r.Findings {
		if f.Signal == "missing-tenant-filter" {
			found = true
			if f.Tier != Waived || !strings.Contains(f.Hint, "middleware") {
				t.Fatalf("want Waived with reason: %+v", f)
			}
		}
	}
	if !found {
		t.Fatal("finding disappeared instead of being waived")
	}
}

func TestWaiveEmptyReasonStaysBlocking(t *testing.T) {
	r := ScanStatic(mk("db.collection('tickets').find({_id:x}) // znf:db-perf-ok:"), cfgLists())
	if !hasSignal(r, "missing-tenant-filter", Blocking) {
		t.Fatal("empty-reason waive must not downgrade")
	}
}
