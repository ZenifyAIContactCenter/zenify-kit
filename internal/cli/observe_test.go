package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/observe"
)

// stubBump warns once, on the 2nd call, to exercise the warn-print path.
func stubBump() func(string, int, time.Time) observe.Decision {
	n := 0
	return func(string, int, time.Time) observe.Decision {
		n++
		if n == 2 {
			return observe.Decision{Warn: true, Message: "WARN-MARKER"}
		}
		return observe.Decision{}
	}
}

func run(t *testing.T, in string, getenv func(string) string, bump func(string, int, time.Time) observe.Decision) (string, int) {
	t.Helper()
	var out bytes.Buffer
	code := runObserveCount(strings.NewReader(in), &out, getenv, bump)
	return out.String(), code
}

func env(kv map[string]string) func(string) string {
	return func(k string) string { return kv[k] }
}

func TestObserveCount_TaskWarnsWithAdditionalContext(t *testing.T) {
	b := stubBump()
	_, _ = run(t, `{"session_id":"s","tool_name":"Task"}`, env(nil), b) // 1st: silent
	out, code := run(t, `{"session_id":"s","tool_name":"Task"}`, env(nil), b) // 2nd: warn
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if !strings.Contains(out, "additionalContext") || !strings.Contains(out, "WARN-MARKER") ||
		!strings.Contains(out, "PreToolUse") {
		t.Fatalf("warn output malformed: %q", out)
	}
}

func TestObserveCount_NonTaskIsNoop(t *testing.T) {
	out, code := run(t, `{"session_id":"s","tool_name":"Bash"}`, env(nil),
		func(string, int, time.Time) observe.Decision { t.Fatal("bump must not be called"); return observe.Decision{} })
	if code != 0 || out != "" {
		t.Fatalf("want silent exit 0, got out=%q code=%d", out, code)
	}
}

func TestObserveCount_MalformedJSONExit0Silent(t *testing.T) {
	out, code := run(t, `{not json`, env(nil),
		func(string, int, time.Time) observe.Decision { t.Fatal("bump must not be called"); return observe.Decision{} })
	if code != 0 || out != "" {
		t.Fatalf("want silent exit 0 on bad json, got out=%q code=%d", out, code)
	}
}

func TestObserveCount_DisabledCapIsNoop(t *testing.T) {
	out, code := run(t, `{"session_id":"s","tool_name":"Task"}`,
		env(map[string]string{"ZENIFY_DISPATCH_SOFTCAP": "0"}),
		func(string, int, time.Time) observe.Decision { t.Fatal("bump must not be called when disabled"); return observe.Decision{} })
	if code != 0 || out != "" {
		t.Fatalf("want silent exit 0 when disabled, got out=%q code=%d", out, code)
	}
}

// recArgs captures what runObserveMeter forwarded to record.
type recArgs struct {
	sess, tool string
	bytes      int64
	called     bool
}

func meterRun(t *testing.T, in string) (recArgs, int) {
	t.Helper()
	var got recArgs
	code := runObserveMeter(strings.NewReader(in),
		func(sess, tool string, b int64, _ time.Time) {
			got = recArgs{sess: sess, tool: tool, bytes: b, called: true}
		})
	return got, code
}

func TestObserveMeter_RecordsResponseBytes(t *testing.T) {
	// tool_response is a JSON string of 5 chars → raw form "\"hello\"" = 7 bytes.
	got, code := meterRun(t, `{"session_id":"s","tool_name":"Bash","tool_response":"hello"}`)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if !got.called || got.tool != "Bash" || got.sess != "s" || got.bytes != 7 {
		t.Fatalf("record args = %+v, want sess=s tool=Bash bytes=7", got)
	}
}

func TestObserveMeter_ObjectResponseBytes(t *testing.T) {
	// Object tool_response is metered by its raw serialized length.
	body := `{"a":1,"b":"xy"}`
	got, code := meterRun(t, `{"session_id":"s","tool_name":"Task","tool_response":`+body+`}`)
	if code != 0 || !got.called || got.bytes != int64(len(body)) {
		t.Fatalf("object meter = %+v code=%d, want bytes=%d", got, code, len(body))
	}
}

func TestObserveMeter_MissingToolNameIsNoop(t *testing.T) {
	got, code := meterRun(t, `{"session_id":"s","tool_response":"x"}`)
	if code != 0 || got.called {
		t.Fatalf("missing tool_name must not record, got %+v code=%d", got, code)
	}
}

func TestObserveMeter_MalformedJSONExit0Silent(t *testing.T) {
	got, code := meterRun(t, `{not json`)
	if code != 0 || got.called {
		t.Fatalf("bad json must be silent no-op, got %+v code=%d", got, code)
	}
}
