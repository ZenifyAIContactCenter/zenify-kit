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
		t.Fatal("shared phải bật advise")
	}
	if !adviseContains(signals, "shared-contract touched") {
		t.Errorf("thiếu signal shared: %v", signals)
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
	// added == 200 (không > ngưỡng LargeCleanLOC) → không bật
	advise, _ := AdviseGate(AdviseInput{Added: 200})
	if advise {
		t.Error("diff == ngưỡng, sạch → không nên advise")
	}
}

func TestAdviseGate_CleanLargeIgnoredWhenFindingsPresent(t *testing.T) {
	// diff lớn nhưng CÓ finding → không phải "clean review", không bật signal clean-large
	advise, signals := AdviseGate(AdviseInput{Added: 500, Findings: []AdviseFinding{{Dimension: "bugs", Severity: "LOW"}}})
	if advise {
		t.Errorf("1 finding + diff lớn nhưng dưới ManyFindings → không nên advise: %v", signals)
	}
	if adviseContains(signals, "clean review on large diff") {
		t.Error("có finding thì không được coi là clean")
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
		t.Error("không tín hiệu → không advise")
	}
	if len(signals) != 0 {
		t.Errorf("signals phải rỗng: %v", signals)
	}
}
