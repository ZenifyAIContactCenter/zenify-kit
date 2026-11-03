package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

// Expected values come from the read-action table in the task brief, not from
// the implementation's own set.
func TestPlaneGuardDecision(t *testing.T) {
	const p = "mcp__zenify-plane__"
	cases := []struct {
		tool, action string
		has          bool
		want         string
	}{
		{p + "workitem", "list", true, "allow"},
		{p + "workitem", "retrieve_by_identifier", true, "allow"},
		{p + "workitem", "create", true, "ask"},
		{p + "workitem", "manage_assignee", true, "ask"},
		{p + "workitem_comment", "create", true, "ask"},
		{p + "workitem_type", "resolve", true, "ask"},
		{p + "workitem", "", false, "ask"},
		{p + "get_pql_reference", "", false, "allow"},
		{p + "workitem_attachment", "upload_from_url", true, "ask"},
	}
	for _, c := range cases {
		if got := planeGuardDecision(c.tool, c.action, c.has); got != c.want {
			t.Errorf("planeGuardDecision(%q, %q, %v) = %q, want %q", c.tool, c.action, c.has, got, c.want)
		}
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
