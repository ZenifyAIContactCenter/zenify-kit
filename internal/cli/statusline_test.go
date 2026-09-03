package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/observe"
)

func stateLoader(count int, ok bool) func(string) (observe.State, bool) {
	return func(string) (observe.State, bool) { return observe.State{Count: count}, ok }
}

func meterLoader(bytesByTool map[string]int64, callsByTool map[string]int, ok bool) func(string) (observe.Meter, bool) {
	return func(string) (observe.Meter, bool) {
		return observe.Meter{Calls: callsByTool, Bytes: bytesByTool}, ok
	}
}

func statusRun(t *testing.T, in string, ls func(string) (observe.State, bool), lm func(string) (observe.Meter, bool)) (string, int) {
	t.Helper()
	var out bytes.Buffer
	code := runObserveStatusline(strings.NewReader(in), &out, false, ls, lm)
	return strings.TrimRight(out.String(), "\n"), code
}

func TestStatusline_FullPayloadAllSegments(t *testing.T) {
	in := `{"session_id":"s","model":{"display_name":"Opus"},"context_window":{"used_percentage":8},"cost":{"total_cost_usd":0.0123}}`
	line, code := statusRun(t, in,
		stateLoader(3, true),
		meterLoader(map[string]int64{"Bash": 2048, "Task": 1024}, map[string]int{"Bash": 4, "Task": 1}, true))
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	for _, want := range []string{"Opus", "ctx 8%", "⟳3", "↓3KB/5", "$0.0123"} {
		if !strings.Contains(line, want) {
			t.Fatalf("line %q missing segment %q", line, want)
		}
	}
	if strings.Count(line, " · ") != 4 {
		t.Fatalf("expected 5 segments joined by ' · ', got %q", line)
	}
}

func TestStatusline_MissingFieldsHideSegments(t *testing.T) {
	// Only a model; no context, no cost; state/meter both empty.
	line, code := statusRun(t, `{"session_id":"s","model":{"display_name":"Sonnet"}}`,
		stateLoader(0, true),
		meterLoader(nil, nil, true))
	if code != 0 || line != "Sonnet" {
		t.Fatalf("want just 'Sonnet', got %q code=%d", line, code)
	}
}

func TestStatusline_ZeroCountNoDispatchSegment(t *testing.T) {
	line, _ := statusRun(t, `{"session_id":"s","model":{"display_name":"Opus"}}`,
		stateLoader(0, true), meterLoader(nil, nil, true))
	if strings.Contains(line, "⟳") {
		t.Fatalf("count 0 must not render a dispatch segment, got %q", line)
	}
}

func TestStatusline_StateNotOkOmitsDispatch(t *testing.T) {
	// loader reports not-ok (torn read) → omit rather than show a wrong number.
	line, _ := statusRun(t, `{"session_id":"s","model":{"display_name":"Opus"}}`,
		stateLoader(99, false), meterLoader(nil, nil, false))
	if strings.Contains(line, "⟳") || line != "Opus" {
		t.Fatalf("not-ok loaders must omit their segments, got %q", line)
	}
}

func TestStatusline_BadJSONExit0OneLine(t *testing.T) {
	var out bytes.Buffer
	code := runObserveStatusline(strings.NewReader(`{not json`), &out, false,
		stateLoader(5, true), meterLoader(map[string]int64{"Bash": 100}, map[string]int{"Bash": 1}, true))
	if code != 0 {
		t.Fatalf("bad json must exit 0, got %d", code)
	}
	// Exactly one line printed; segments driven only by loaders (session_id empty).
	if strings.Count(out.String(), "\n") != 1 {
		t.Fatalf("must print exactly one line, got %q", out.String())
	}
}

func TestStatusline_SegmentModeDropsStdinSegments(t *testing.T) {
	// Full payload, but --segment must drop model/ctx/cost and keep only ⟳ · ↓.
	in := `{"session_id":"s","model":{"display_name":"Opus"},"context_window":{"used_percentage":8},"cost":{"total_cost_usd":0.0123}}`
	var out bytes.Buffer
	code := runObserveStatusline(strings.NewReader(in), &out, true,
		stateLoader(3, true),
		meterLoader(map[string]int64{"Bash": 2048}, map[string]int{"Bash": 4}, true))
	line := strings.TrimRight(out.String(), "\n")
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	for _, drop := range []string{"Opus", "ctx", "$0.0123"} {
		if strings.Contains(line, drop) {
			t.Fatalf("--segment must drop %q, got %q", drop, line)
		}
	}
	if line != "⟳3 · ↓2KB/4" {
		t.Fatalf("segment line = %q, want %q", line, "⟳3 · ↓2KB/4")
	}
}

func TestStatusline_SegmentModeEmptyWhenNoState(t *testing.T) {
	// No dispatches, no tool-output → segment mode prints an empty line (caller
	// guards with [ -n "$seg" ]), never a stray separator.
	var out bytes.Buffer
	code := runObserveStatusline(strings.NewReader(`{"session_id":"s","model":{"display_name":"Opus"}}`), &out, true,
		stateLoader(0, true), meterLoader(nil, nil, true))
	if code != 0 || strings.TrimRight(out.String(), "\n") != "" {
		t.Fatalf("empty segment mode must print blank line, got %q code=%d", out.String(), code)
	}
}

func TestEnsureStatusline_InstallsIntoEmpty(t *testing.T) {
	out, action, err := ensureStatusline(nil, false)
	if err != nil || action != "installed" {
		t.Fatalf("action=%q err=%v, want installed", action, err)
	}
	if !strings.Contains(string(out), `"command": "zenify observe statusline"`) {
		t.Fatalf("out missing command: %s", out)
	}
}

func TestEnsureStatusline_IdempotentWhenAlreadyOurs(t *testing.T) {
	in := `{"statusLine":{"type":"command","command":"zenify observe statusline"}}`
	out, action, err := ensureStatusline([]byte(in), false)
	if err != nil || action != "already" || out != nil {
		t.Fatalf("action=%q out=%v err=%v, want already/nil", action, out, err)
	}
}

func TestEnsureStatusline_RefusesToClobberWithoutForce(t *testing.T) {
	in := `{"statusLine":{"type":"command","command":"~/.claude/statusline.sh"},"theme":"dark"}`
	out, action, err := ensureStatusline([]byte(in), false)
	if err != nil || action != "occupied" || out != nil {
		t.Fatalf("action=%q err=%v, want occupied and no write", action, err)
	}
}

func TestEnsureStatusline_ForceOverwritesButKeepsOtherKeys(t *testing.T) {
	in := `{"statusLine":{"type":"command","command":"~/.claude/statusline.sh"},"theme":"dark"}`
	out, action, err := ensureStatusline([]byte(in), true)
	if err != nil || action != "forced" {
		t.Fatalf("action=%q err=%v, want forced", action, err)
	}
	s := string(out)
	if !strings.Contains(s, `"command": "zenify observe statusline"`) {
		t.Fatalf("force did not set our command: %s", s)
	}
	if !strings.Contains(s, `"theme": "dark"`) {
		t.Fatalf("force dropped an unrelated key: %s", s)
	}
}

func TestEnsureStatusline_BrokenJSONErrors(t *testing.T) {
	if _, _, err := ensureStatusline([]byte(`{not json`), false); err == nil {
		t.Fatal("broken JSON must error, not silently overwrite")
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{0: "0B", 512: "512B", 2048: "2KB", 1048576: "1.0MB", 1572864: "1.5MB"}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}
