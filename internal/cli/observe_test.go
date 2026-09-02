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
