package observe

import (
	"os"
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
