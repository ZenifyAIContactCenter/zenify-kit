package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

// Expected values come from the read-action table and the decision rules in
// the task brief, not from the implementation's own set.
func TestPlaneGuardDecision(t *testing.T) {
	const p = "mcp__zenify-plane__"
	st := map[string]json.RawMessage{"state": json.RawMessage(`"0b1f6f0e-0000-4000-8000-000000000001"`)}
	emptyState := map[string]json.RawMessage{"state": json.RawMessage(`""`)}
	assignees := map[string]json.RawMessage{"assignees": json.RawMessage(`["u1"]`)}
	cases := []struct {
		name         string
		tool, action string
		has          bool
		input        map[string]json.RawMessage
		readOnly     bool
		want         string
	}{
		{"list", p + "workitem", "list", true, nil, false, "allow"},
		{"retrieve_by_identifier", p + "workitem", "retrieve_by_identifier", true, nil, false, "allow"},
		{"create", p + "workitem", "create", true, nil, false, "ask"},
		{"manage_assignee", p + "workitem", "manage_assignee", true, nil, false, "ask"},
		{"comment create", p + "workitem_comment", "create", true, nil, false, "ask"},
		{"type resolve", p + "workitem_type", "resolve", true, nil, false, "ask"},
		{"no action", p + "workitem", "", false, nil, false, "ask"},
		{"pql ref", p + "get_pql_reference", "", false, nil, false, "allow"},
		{"upload", p + "workitem_attachment", "upload_from_url", true, nil, false, "ask"},
		{"update state", p + "workitem", "update", true, st, false, "deny"},
		{"create state", p + "workitem", "create", true, st, false, "deny"},
		{"update assignees", p + "workitem", "update", true, assignees, false, "ask"},
		{"update empty state", p + "workitem", "update", true, emptyState, false, "ask"},
		{"other tool state", p + "workitem_comment", "update", true, st, false, "ask"},
		{"ro update", p + "workitem", "update", true, nil, true, "deny"},
		{"ro comment create", p + "workitem_comment", "create", true, nil, true, "deny"},
		{"ro retrieve", p + "workitem", "retrieve", true, nil, true, "allow"},
		{"ro type resolve", p + "workitem_type", "resolve", true, nil, true, "deny"},
		{"ro type list", p + "workitem_type", "list", true, nil, true, "allow"},
		{"ro pql ref", p + "get_pql_reference", "", false, nil, true, "allow"},
		{"ro other no action", p + "workitem", "", false, nil, true, "deny"},
	}
	for _, c := range cases {
		if got := planeGuardDecision(c.tool, c.action, c.has, c.input, c.readOnly); got != c.want {
			t.Errorf("%s: planeGuardDecision(%q, %q, %v, ro=%v) = %q, want %q", c.name, c.tool, c.action, c.has, c.readOnly, got, c.want)
		}
	}
}

func planeGuardRunOut(t *testing.T, in string) planeGuardOut {
	t.Helper()
	var out bytes.Buffer
	if code := runPlaneGuard(strings.NewReader(in), &out); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var got planeGuardOut
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("stdout not JSON: %q (%v)", out.String(), err)
	}
	return got
}

func TestRunPlaneGuard_ReadMode(t *testing.T) {
	t.Setenv("ZENIFY_PLANE_GUARD", "read")
	const tool = `"tool_name":"mcp__zenify-plane__workitem"`
	for _, in := range []string{
		`{` + tool + `,"tool_input":[1]}`,
		`{` + tool + `,"tool_input":"x"}`,
		`{` + tool + `,"tool_input":{"action":1}}`,
		`{` + tool + `,"tool_input":{"action":null}}`,
		`{` + tool + `,"tool_input":{"action":"create"}}`,
		`{` + tool + `}`,
	} {
		h := planeGuardRunOut(t, in).HookSpecificOutput
		if h.PermissionDecision != "deny" {
			t.Errorf("%s: want deny, got %+v", in, h)
		}
		if !strings.Contains(h.PermissionDecisionReason, "zenify await handoff") || !strings.Contains(h.PermissionDecisionReason, "needs-human") {
			t.Errorf("%s: reason lacks handoff hint: %q", in, h.PermissionDecisionReason)
		}
	}
	if h := planeGuardRunOut(t, `{`+tool+`,"tool_input":{"action":"list"}}`).HookSpecificOutput; h.PermissionDecision != "allow" {
		t.Errorf("list: %+v", h)
	}
	// Unreadable payload: read mode denies instead of staying silent.
	for _, in := range []string{`garbage`, `{"tool_name":1}`} {
		if h := planeGuardRunOut(t, in).HookSpecificOutput; h.PermissionDecision != "deny" {
			t.Errorf("%s: want deny, got %+v", in, h)
		}
	}
	// A non-Plane tool still prints nothing.
	var out bytes.Buffer
	runPlaneGuard(strings.NewReader(`{"tool_name":"Bash","tool_input":{}}`), &out)
	if out.Len() != 0 {
		t.Errorf("Bash: stdout must be empty, got %q", out.String())
	}
}

// The env value is matched case-insensitively and trimmed, so a near-miss
// spelling never silently re-opens writes.
func TestRunPlaneGuard_ReadModeSpellings(t *testing.T) {
	in := `{"tool_name":"mcp__zenify-plane__workitem","tool_input":{"action":"create"}}`
	for env, want := range map[string]string{"Read": "deny", "READ": "deny", " read ": "deny", "": "ask"} {
		t.Setenv("ZENIFY_PLANE_GUARD", env)
		if h := planeGuardRunOut(t, in).HookSpecificOutput; h.PermissionDecision != want {
			t.Errorf("env %q: want %s, got %+v", env, want, h)
		}
	}
}

func TestRunPlaneGuard_DenyStateWrite(t *testing.T) {
	t.Setenv("ZENIFY_PLANE_GUARD", "")
	for _, st := range []string{`123`, `{"id":"x"}`, `["x"]`} {
		in := `{"tool_name":"mcp__zenify-plane__workitem","tool_input":{"action":"update","state":` + st + `}}`
		if h := planeGuardRunOut(t, in).HookSpecificOutput; h.PermissionDecision != "deny" {
			t.Errorf("state %s: want deny, got %+v", st, h)
		}
	}
	for _, st := range []string{`null`, `""`} {
		in := `{"tool_name":"mcp__zenify-plane__workitem","tool_input":{"action":"update","state":` + st + `}}`
		if h := planeGuardRunOut(t, in).HookSpecificOutput; h.PermissionDecision != "ask" {
			t.Errorf("state %s: want ask, got %+v", st, h)
		}
	}
	h := planeGuardRunOut(t, `{"tool_name":"mcp__zenify-plane__workitem","tool_input":{"action":"update","state":"abc"}}`).HookSpecificOutput
	if h.PermissionDecision != "deny" || !strings.Contains(h.PermissionDecisionReason, "state do Zenie chuyển") { //znf:allow-lang
		t.Fatalf("got %+v", h)
	}
}

type planeGuardOut struct {
	HookSpecificOutput struct {
		HookEventName            string `json:"hookEventName"`
		PermissionDecision       string `json:"permissionDecision"`
		PermissionDecisionReason string `json:"permissionDecisionReason"`
	} `json:"hookSpecificOutput"`
}

func TestRunPlaneGuard_Output(t *testing.T) {
	t.Setenv("ZENIFY_PLANE_GUARD", "")
	var out bytes.Buffer
	code := runPlaneGuard(strings.NewReader(`{"tool_name":"mcp__zenify-plane__workitem","tool_input":{"action":"create"}}`), &out)
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var got planeGuardOut
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("stdout not JSON: %q (%v)", out.String(), err)
	}
	h := got.HookSpecificOutput
	if h.HookEventName != "PreToolUse" || h.PermissionDecision != "ask" {
		t.Fatalf("got %+v", h)
	}
	if h.PermissionDecisionReason != "Plane: mcp__zenify-plane__workitem create ghi bằng tài khoản của bạn" { //znf:allow-lang
		t.Fatalf("reason = %q", h.PermissionDecisionReason)
	}

	out.Reset()
	if code := runPlaneGuard(strings.NewReader(`{"tool_name":"mcp__zenify-plane__workitem","tool_input":{"action":"list"}}`), &out); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	got = planeGuardOut{}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("stdout not JSON: %q", out.String())
	}
	if got.HookSpecificOutput.PermissionDecision != "allow" || got.HookSpecificOutput.PermissionDecisionReason != "Plane: chỉ đọc" { //znf:allow-lang
		t.Fatalf("list: %+v", got.HookSpecificOutput)
	}

	for _, in := range []string{`{"tool_name":"Bash","tool_input":{"command":"ls"}}`, `not json`, ``} {
		out.Reset()
		if code := runPlaneGuard(strings.NewReader(in), &out); code != 0 {
			t.Fatalf("%q: exit = %d", in, code)
		}
		if out.Len() != 0 {
			t.Fatalf("%q: stdout must be empty, got %q", in, out.String())
		}
	}
}

// A Plane tool whose tool_input cannot be decoded (or whose action is not a
// readable string) must ask; input whose tool_name cannot be read stays silent.
func TestRunPlaneGuard_UndecodableInput(t *testing.T) {
	t.Setenv("ZENIFY_PLANE_GUARD", "")
	const tool = `"tool_name":"mcp__zenify-plane__workitem"`
	for in, want := range map[string]string{
		`{` + tool + `,"tool_input":[1]}`:                      "ask",
		`{` + tool + `,"tool_input":"x"}`:                      "ask",
		`{` + tool + `,"tool_input":{"action":1}}`:             "ask",
		`{` + tool + `,"tool_input":{"action":null}}`:          "ask",
		`{"tool_name":"mcp__zenify-plane__get_pql_reference"}`: "allow",
		`{"tool_name":"Bash","tool_input":[1]}`:                "",
		`{"tool_name":1,"tool_input":{}}`:                      "",
		`garbage`:                                              "",
	} {
		var out bytes.Buffer
		if code := runPlaneGuard(strings.NewReader(in), &out); code != 0 {
			t.Fatalf("%s: exit = %d", in, code)
		}
		if want == "" {
			if out.Len() != 0 {
				t.Errorf("%s: stdout must be empty, got %q", in, out.String())
			}
			continue
		}
		var got planeGuardOut
		if err := json.Unmarshal(out.Bytes(), &got); err != nil || got.HookSpecificOutput.PermissionDecision != want {
			t.Errorf("%s: want %s, got %q", in, want, out.String())
		}
	}
}

func TestDispatchHook_PlaneGuardOutsideWorkspace(t *testing.T) {
	old := planeGuardRun
	planeGuardRun = func(w io.Writer) int {
		return runPlaneGuard(strings.NewReader(`{"tool_name":"mcp__zenify-plane__workitem","tool_input":{"action":"create"}}`), w)
	}
	t.Cleanup(func() { planeGuardRun = old })
	var buf bytes.Buffer
	if code := dispatchHook("plane-guard", "", &buf); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(buf.String(), `"ask"`) || strings.TrimSpace(buf.String()) == "{}" {
		t.Fatalf("plane-guard must run outside a workspace: %q", buf.String())
	}
}
