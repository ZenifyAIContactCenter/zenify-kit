package review

import "testing"

func adviseContains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func TestAdviseGate_Shared(t *testing.T) {
	advise, signals := AdviseGate(AdviseInput{Shared: true})
	if !advise {
		t.Fatal("shared must turn advise on")
	}
	if !adviseContains(signals, "shared-contract touched") {
		t.Errorf("missing shared signal: %v", signals)
	}
}

func TestAdviseGate_Critical(t *testing.T) {
	advise, signals := AdviseGate(AdviseInput{Critical: true})
	if !advise || !adviseContains(signals, "critical-flagged change") {
		t.Errorf("critical: advise=%v signals=%v", advise, signals)
	}
}

func TestAdviseGate_CleanLargeDiff(t *testing.T) {
	advise, signals := AdviseGate(AdviseInput{Added: 201})
	if !advise || !adviseContains(signals, "clean review on large diff") {
		t.Errorf("clean-large: advise=%v signals=%v", advise, signals)
	}
}

func TestAdviseGate_CleanAtThreshold_NoAdvise(t *testing.T) {
	// added == 200 (not > LargeCleanLOC threshold) → should not fire
	advise, _ := AdviseGate(AdviseInput{Added: 200})
	if advise {
		t.Error("diff == threshold, clean → should not advise")
	}
}

func TestAdviseGate_CleanLargeIgnoredWhenFindingsPresent(t *testing.T) {
	// large diff but HAS findings → not a "clean review", clean-large signal should not fire
	advise, signals := AdviseGate(AdviseInput{Added: 500, Findings: []AdviseFinding{{Dimension: "bugs", Severity: "LOW"}}})
	if advise {
		t.Errorf("1 finding + large diff but below ManyFindings → should not advise: %v", signals)
	}
	if adviseContains(signals, "clean review on large diff") {
		t.Error("a diff with findings must not be treated as clean")
	}
}

func TestAdviseGate_ManyFindings(t *testing.T) {
	f := []AdviseFinding{
		{Dimension: "bugs", Severity: "LOW"},
		{Dimension: "bugs", Severity: "LOW"},
		{Dimension: "bugs", Severity: "LOW"},
		{Dimension: "bugs", Severity: "LOW"},
	}
	advise, signals := AdviseGate(AdviseInput{Findings: f})
	if !advise || !adviseContains(signals, "enough findings to assess a pattern") {
		t.Errorf("many: advise=%v signals=%v", advise, signals)
	}
}

func TestAdviseGate_None(t *testing.T) {
	advise, signals := AdviseGate(AdviseInput{Added: 50, Findings: []AdviseFinding{{Dimension: "bugs", Severity: "LOW"}}})
	if advise {
		t.Error("no signals → should not advise")
	}
	if len(signals) != 0 {
		t.Errorf("signals must be empty: %v", signals)
	}
}
