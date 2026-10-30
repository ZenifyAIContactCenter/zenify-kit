package trackstats

import (
	"fmt"
	"testing"
	"time"
)

const tid = "cook-261030-ab12"

var t0 = time.Date(2026, 10, 30, 1, 0, 0, 0, time.UTC)

func at(min int) time.Time { return t0.Add(time.Duration(min) * time.Minute) }

func turn(endMin, durMin int, role, model, in, out string) Event {
	return Event{At: at(endMin), Type: "turn", ID: tid,
		F: []string{fmt.Sprint(durMin * 60000), role, model, in, out, "-", "-"}}
}

func wait(endMin, durMin int, kind string) Event {
	return Event{At: at(endMin), Type: "wait", ID: tid, F: []string{fmt.Sprint(durMin * 60000), kind}}
}

func TestAgentUnion(t *testing.T) {
	got := ComputeTiming(tid, "cook", t0, []Event{turn(10, 10, "main", "-", "-", "-"), turn(15, 10, "main", "-", "-", "-")})
	if got.AgentMin != 15 {
		t.Fatalf("AgentMin = %d, want 15", got.AgentMin)
	}
}

func TestAgentMinusAsk(t *testing.T) {
	got := ComputeTiming(tid, "cook", t0, []Event{
		turn(20, 20, "main", "-", "-", "-"), wait(12, 7, "ask"), wait(60, 30, "idle"),
	})
	w := got.Wait
	if got.AgentMin != 13 || w.Ask != 7 || w.Idle != 30 || w.Total != 37 {
		t.Fatalf("agent=%d wait=%+v", got.AgentMin, w)
	}
}

func TestSubNotMinusAsk(t *testing.T) {
	got := ComputeTiming(tid, "cook", t0, []Event{
		turn(20, 20, "main", "-", "-", "-"), wait(12, 7, "ask"), turn(12, 7, "sub", "-", "-", "-"),
	})
	if got.AgentMin != 20 {
		t.Fatalf("AgentMin = %d, want 20", got.AgentMin)
	}
}

func TestPhaseSplit(t *testing.T) {
	got := ComputeTiming(tid, "cook", t0, []Event{
		turn(20, 20, "main", "-", "-", "-"),
		{At: at(10), Type: "phase", ID: tid, F: []string{"scout"}},
		{At: at(15), Type: "phase", ID: tid, F: []string{"plan"}},
	})
	if got.Phases["design"].AgentMin != 10 || got.Phases["scout"].AgentMin != 5 || got.Phases["plan"].AgentMin != 5 {
		t.Fatalf("phases = %+v", got.Phases)
	}
	if got.Phase != "plan" {
		t.Fatalf("Phase = %q", got.Phase)
	}
	if f := ComputeTiming(tid, "fix", t0, nil); f.Phase != "diagnose" {
		t.Fatalf("fix start phase = %q", f.Phase)
	}
}

func TestTokensByModel(t *testing.T) {
	text := "" +
		"2026-10-30T01:10:00Z · turn · " + tid + " · 60000 · main · A · 100 · 10 · - · -\n" +
		"2026-10-30T01:11:00Z · turn · " + tid + " · 60000 · main · B · 50 · 5 · - · -\n" +
		"2026-10-30T01:12:00Z · turn · " + tid + " · 60000 · main · - · - · - · - · -\n"
	ev, _, bad := ParseSession("s", text)
	if bad != 0 {
		t.Fatalf("malformed = %d, want 0", bad)
	}
	got := ComputeTiming(tid, "cook", t0, ev).Tokens
	if len(got) != 2 || got["A"].In != 100 || got["A"].Out != 10 || got["B"].In != 50 || got["B"].Out != 5 {
		t.Fatalf("tokens = %+v", got)
	}
}
