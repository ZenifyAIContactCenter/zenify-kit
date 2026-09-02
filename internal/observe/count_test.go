package observe

import (
	"strings"
	"testing"
)

func TestEvaluate_WarnsAtCapThenSilentThenWarnsAgain(t *testing.T) {
	softCap := 3
	st := State{}
	var warns []int // counts at which a warn fired
	for i := 0; i < 7; i++ {
		var d Decision
		st, d = Evaluate(st, softCap)
		if d.Warn {
			warns = append(warns, st.Count)
		}
	}
	// softCap=3 → warn at 3 and 6, silent at 1,2,4,5,7
	if len(warns) != 2 || warns[0] != 3 || warns[1] != 6 {
		t.Fatalf("want warns at [3 6], got %v", warns)
	}
	if st.Count != 7 {
		t.Fatalf("want Count 7, got %d", st.Count)
	}
}

func TestEvaluate_MessageMentionsCountAndCap(t *testing.T) {
	st, d := Evaluate(State{Count: 9}, 10)
	if !d.Warn {
		t.Fatal("want warn at count 10")
	}
	if !strings.Contains(d.Message, "10") || !strings.Contains(d.Message, "#10") {
		t.Fatalf("message missing count/rule ref: %q", d.Message)
	}
	if st.LastWarnedMultiple != 1 {
		t.Fatalf("want LastWarnedMultiple 1, got %d", st.LastWarnedMultiple)
	}
}
