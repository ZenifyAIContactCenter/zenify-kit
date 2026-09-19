package observe

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func readMeterForTest(t *testing.T, sessionID string) Meter {
	t.Helper()
	d, err := dir()
	if err != nil {
		t.Fatal(err)
	}
	m, _ := readMeter(meterPath(d, sanitizeSession(sessionID)))
	return m
}

func TestRecord_AccumulatesPerTool(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	sid := "sess-meter"
	now := time.Now()
	Record(sid, "Bash", 100, now)
	Record(sid, "Bash", 50, now)
	Record(sid, "Task", 200, now)

	m := readMeterForTest(t, sid)
	if m.Calls["Bash"] != 2 || m.Bytes["Bash"] != 150 {
		t.Fatalf("Bash: calls=%d bytes=%d, want 2/150", m.Calls["Bash"], m.Bytes["Bash"])
	}
	if m.Calls["Task"] != 1 || m.Bytes["Task"] != 200 {
		t.Fatalf("Task: calls=%d bytes=%d, want 1/200", m.Calls["Task"], m.Bytes["Task"])
	}
}

func TestRecord_IgnoresEmptyToolAndNegativeBytes(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	sid := "sess-guard"
	now := time.Now()
	Record(sid, "", 100, now)
	Record(sid, "Bash", -5, now)

	m := readMeterForTest(t, sid)
	if len(m.Calls) != 0 || len(m.Bytes) != 0 {
		t.Fatalf("guarded inputs must not be recorded, got %+v", m)
	}
}

func TestRecord_ConcurrentNoCorruptionNoCrash(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	sid := "sess-meter-race"
	now := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); Record(sid, "Bash", 10, now) }()
	}
	wg.Wait()

	m := readMeterForTest(t, sid)
	// File must be valid JSON; counts must not exceed the number of records.
	if m.Calls["Bash"] < 0 || m.Calls["Bash"] > 20 {
		t.Fatalf("Bash call count out of range after 20 concurrent records: %d", m.Calls["Bash"])
	}
}

func TestRecord_CorruptMeterFailsOpen(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	sid := "sess-meter-corrupt"
	now := time.Now()

	base, err := dir()
	if err != nil {
		t.Fatal(err)
	}
	sd := sessDir(base, sanitizeSession(sid))
	if err := os.MkdirAll(sd, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(meterPath(base, sanitizeSession(sid)), []byte("{not valid json"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Must not panic and must not overwrite the corrupt file with a zero value.
	Record(sid, "Bash", 100, now)
	b, _ := os.ReadFile(meterPath(base, sanitizeSession(sid)))
	if string(b) != "{not valid json" {
		t.Fatalf("corrupt meter should be left untouched (fail open), got %q", string(b))
	}
}

func TestAdvise_ThresholdsAndDebounce(t *testing.T) {
	m := Meter{Calls: map[string]int{"Bash": 1}, Bytes: map[string]int64{"Bash": 10}}
	if a := advise(&m, "Bash", 10); a.Message != "" {
		t.Fatalf("small result must be silent: %q", a.Message)
	}
	// one large result speaks and records the call index
	m.Calls["Bash"] = 2
	if a := advise(&m, "Bash", LargeResultBytes+1); a.Message == "" || m.LastWarnCall != 2 {
		t.Fatalf("large result: %+v lastWarn=%d", a, m.LastWarnCall)
	}
	// the next four large results are debounced, the fifth speaks again
	for i := 3; i <= 6; i++ {
		m.Calls["Bash"] = i
		if a := advise(&m, "Bash", LargeResultBytes+1); a.Message != "" {
			t.Fatalf("call %d must be debounced: %q", i, a.Message)
		}
	}
	m.Calls["Bash"] = 7
	if a := advise(&m, "Bash", LargeResultBytes+1); a.Message == "" {
		t.Fatal("call 7 must speak again")
	}
	// heavy session, small result → the session-level message
	h := Meter{Calls: map[string]int{"Read": 40}, Bytes: map[string]int64{"Read": HeavySessionBytes + 1}}
	if a := advise(&h, "Read", 100); a.Message == "" || h.LastWarnCall != 40 {
		t.Fatalf("heavy session: %+v", a)
	}
}

func TestAdvise_HeavyAsksOnceThenSilent(t *testing.T) {
	h := Meter{Calls: map[string]int{"Read": 40}, Bytes: map[string]int64{"Read": HeavySessionBytes + 1}}
	a := advise(&h, "Read", 100)
	if a.Message == "" || !strings.Contains(a.Message, "AskUserQuestion") || !h.HeavyAsked {
		t.Fatalf("first heavy call must ask once: %+v asked=%v", a, h.HeavyAsked)
	}
	for i := 41; i <= 60; i++ {
		h.Calls["Read"] = i
		if a := advise(&h, "Read", 100); a.Message != "" {
			t.Fatalf("call %d: heavy branch must stay silent after asking: %q", i, a.Message)
		}
	}
	// a large single result still speaks (its own branch, own debounce)
	h.Calls["Read"] = 61
	if a := advise(&h, "Read", LargeResultBytes+1); a.Message == "" {
		t.Fatal("large-result branch must be unaffected by HeavyAsked")
	}
}

func TestReadMeter_OldFileWithoutHeavyAsked(t *testing.T) {
	p := filepath.Join(t.TempDir(), "meter.json")
	if err := os.WriteFile(p, []byte(`{"calls":{"Bash":3},"bytes":{"Bash":30},"last_warn_call":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	m, ok := readMeter(p)
	if !ok || m.HeavyAsked || m.TotalCalls() != 3 {
		t.Fatalf("old meter must load with HeavyAsked=false: %+v ok=%v", m, ok)
	}
}

func TestRecord_DeniedBytesExcludedFromTotalsAndAdvice(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	sid := "sess-denied"
	now := time.Now()
	if a := Record(sid, "Read:denied", 3_000_000, now); a.Message != "" {
		t.Fatalf("denied record must return empty advice, got %q", a.Message)
	}

	m := readMeterForTest(t, sid)
	if m.HeavyAsked {
		t.Fatal("denied bytes must not flip HeavyAsked")
	}
	if m.Bytes["Read:denied"] != 3_000_000 {
		t.Fatalf("denied bytes must still be recorded, got %d", m.Bytes["Read:denied"])
	}
	if m.TotalBytes() != 0 {
		t.Fatalf("TotalBytes must exclude denied keys, got %d", m.TotalBytes())
	}
}

func TestMeter_TotalCallsExcludesDeniedDeniedCallsCountsIt(t *testing.T) {
	m := Meter{
		Calls: map[string]int{"Bash": 2, "Read:denied": 3},
		Bytes: map[string]int64{"Bash": 20, "Read:denied": 300},
	}
	if m.TotalCalls() != 2 {
		t.Fatalf("TotalCalls must exclude denied, got %d", m.TotalCalls())
	}
	if m.DeniedCalls() != 3 {
		t.Fatalf("DeniedCalls must count denied, got %d", m.DeniedCalls())
	}
}

func TestRecord_ReturnsAdviceOnLargeResult(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if a := Record("sess-adv", "Bash", 10, time.Now()); a.Message != "" {
		t.Fatalf("small: %q", a.Message)
	}
	if a := Record("sess-adv", "Bash", LargeResultBytes+1, time.Now()); a.Message == "" {
		t.Fatal("large result must return advice")
	}
	if m := readMeterForTest(t, "sess-adv"); m.LastWarnCall != 2 {
		t.Fatalf("last_warn_call persisted = %d, want 2", m.LastWarnCall)
	}
}
