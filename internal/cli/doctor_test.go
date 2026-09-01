package cli

import "testing"

func TestRunChecksAggregatesHealth(t *testing.T) {
	saved := checks
	defer func() { checks = saved }()
	checks = []Check{
		{Name: "a", Run: func() (bool, string) { return true, "ok" }},
		{Name: "b", Run: func() (bool, string) { return false, "bad" }},
	}
	results, healthy := runChecks()
	if healthy {
		t.Fatal("healthy should be false when any check fails")
	}
	if len(results) != 2 || results[0].Name != "a" || results[1].OK {
		t.Fatalf("unexpected results: %+v", results)
	}
}
