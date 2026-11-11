package cli

import (
	"encoding/json"
	"io"
	"os"
	"strings"
)

// planeToolPrefix is the Claude Code tool-name prefix of the zenify-plane MCP
// server registered by `zenify up`.
const planeToolPrefix = "mcp__zenify-plane__"

// planeReadActions are the Plane MCP `action` values that only read, taken
// from the tool table in the Plane MCP README. Every other action (and an
// unknown or missing one) writes or might, so it asks.
var planeReadActions = map[string]bool{
	"list": true, "list_archived": true, "retrieve": true, "retrieve_by_identifier": true,
	"search": true, "count": true, "me": true, "list_workspace": true, "list_project": true,
	"list_roles": true, "retrieve_role": true, "list_workitems": true, "list_pages": true,
	"search_pages": true, "list_members": true, "list_projects": true, "list_points": true,
	"list_options": true, "retrieve_option": true, "list_definitions": true,
	"list_workitem_pages": true, "get_values": true, "get_value": true, "get_features": true,
	"get_changelog": true, "worklog_summary": true, "read": true, "download_url": true,
}

// planeGuardDecision returns "allow" for a read-only Plane MCP call. In
// readOnly mode every other call is denied. In the default mode a workitem
// create/update carrying a non-empty state is denied (Zenie moves state), and
// everything else asks. get_pql_reference takes no action and only reads.
func planeGuardDecision(tool, action string, hasAction bool, input map[string]json.RawMessage, readOnly bool) string {
	isRead := planeReadActions[action] && hasAction
	if !hasAction && tool == planeToolPrefix+"get_pql_reference" {
		isRead = true
	}
	if isRead {
		return "allow"
	}
	if readOnly {
		return "deny"
	}
	if hasAction && tool == planeToolPrefix+"workitem" && (action == "create" || action == "update") {
		// Any state value other than null or "" denies, whatever its JSON type.
		if raw, ok := input["state"]; ok {
			if v := strings.TrimSpace(string(raw)); v != "null" && v != `""` {
				return "deny"
			}
		}
	}
	return "ask"
}

type planeGuardPayload struct {
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
}

// planeAskOut is the ask decision printed when a Plane tool's input cannot be read.
const planeAskOut = `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask","permissionDecisionReason":"Plane: không đọc được tham số, hỏi bạn trước"}}` + "\n" //znf:allow-lang

// planeDenyReadOut is the deny decision printed in read mode when a Plane
// tool's input cannot be read.
const planeDenyReadOut = `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"Plane: chế độ chỉ đọc, không đọc được tham số. Cần ghi thì chạy zenify await handoff hoặc gắn nhãn needs-human"}}` + "\n" //znf:allow-lang

// planeReadOnlyReason / planeStateReason are the deny reasons for a blocked write.
const (
	planeReadOnlyReason = "Plane: chế độ chỉ đọc, không được ghi. Cần ghi thì chạy zenify await handoff hoặc gắn nhãn needs-human" //znf:allow-lang
	planeStateReason    = "Plane: state do Zenie chuyển, không đặt state qua MCP"                                                  //znf:allow-lang
)

// runPlaneGuard is the plane-guard PreToolUse hook. It always returns 0: a
// decision is printed as hookSpecificOutput JSON. Input whose tool_name cannot
// be read, or a non-Plane tool, prints nothing (the harness's normal prompt).
// Once the tool is known to be Plane, every later failure (undecodable
// tool_input, a panic) prints ask, so bypassPermissions never runs it unprompted.
func runPlaneGuard(stdin io.Reader, stdout io.Writer) (code int) {
	isPlane := false
	readOnly := strings.EqualFold(strings.TrimSpace(os.Getenv("ZENIFY_PLANE_GUARD")), "read")
	failOut := planeAskOut
	if readOnly {
		failOut = planeDenyReadOut
	}
	defer func() {
		if r := recover(); r != nil {
			if isPlane {
				_, _ = io.WriteString(stdout, failOut)
			}
			code = 0
		}
	}()
	raw, err := io.ReadAll(stdin)
	var p planeGuardPayload
	if err != nil || json.Unmarshal(raw, &p) != nil {
		if readOnly {
			_, _ = io.WriteString(stdout, failOut)
		}
		return 0
	}
	if !strings.HasPrefix(p.ToolName, planeToolPrefix) {
		return 0
	}
	isPlane = true
	var input map[string]json.RawMessage
	if len(p.ToolInput) > 0 && json.Unmarshal(p.ToolInput, &input) != nil {
		_, _ = io.WriteString(stdout, failOut)
		return 0
	}
	var action string
	rawAction, hasAction := input["action"]
	if hasAction && json.Unmarshal(rawAction, &action) != nil {
		action, hasAction = "", true // non-string action: unknown, asks
	}
	decision := planeGuardDecision(p.ToolName, action, hasAction, input, readOnly)
	reason := "Plane: chỉ đọc" //znf:allow-lang
	switch {
	case decision == "ask":
		reason = strings.Join(strings.Fields("Plane: "+p.ToolName+" "+action), " ") + " ghi bằng tài khoản của bạn" //znf:allow-lang
	case decision == "deny" && readOnly:
		reason = planeReadOnlyReason
	case decision == "deny":
		reason = planeStateReason
	}
	out, err := json.Marshal(map[string]any{"hookSpecificOutput": map[string]string{
		"hookEventName":            "PreToolUse",
		"permissionDecision":       decision,
		"permissionDecisionReason": reason,
	}})
	if err != nil {
		_, _ = io.WriteString(stdout, failOut)
		return 0
	}
	_, _ = stdout.Write(append(out, '\n'))
	return 0
}
