package cli

import (
	"encoding/json"
	"io"
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

// planeGuardDecision returns "allow" for a read-only Plane MCP call and "ask"
// for everything else. get_pql_reference takes no action and only reads.
func planeGuardDecision(tool, action string, hasAction bool) string {
	if !hasAction {
		if tool == planeToolPrefix+"get_pql_reference" {
			return "allow"
		}
		return "ask"
	}
	if planeReadActions[action] {
		return "allow"
	}
	return "ask"
}

type planeGuardPayload struct {
	ToolName  string                     `json:"tool_name"`
	ToolInput map[string]json.RawMessage `json:"tool_input"`
}

// runPlaneGuard is the plane-guard PreToolUse hook. It always returns 0: a
// decision is printed as hookSpecificOutput JSON; a non-Plane tool, bad JSON
// or a panic prints nothing, so the harness falls back to its normal prompt
// (never an allow on input it could not read).
func runPlaneGuard(stdin io.Reader, stdout io.Writer) (code int) {
	defer func() {
		if r := recover(); r != nil {
			code = 0
		}
	}()
	raw, err := io.ReadAll(stdin)
	if err != nil {
		return 0
	}
	var p planeGuardPayload
	if json.Unmarshal(raw, &p) != nil || !strings.HasPrefix(p.ToolName, planeToolPrefix) {
		return 0
	}
	var action string
	rawAction, hasAction := p.ToolInput["action"]
	if hasAction && json.Unmarshal(rawAction, &action) != nil {
		action, hasAction = "", true // non-string action: unknown, asks
	}
	decision := planeGuardDecision(p.ToolName, action, hasAction)
	reason := "Plane: chỉ đọc" //znf:allow-lang
	if decision == "ask" {
		reason = strings.Join(strings.Fields("Plane: "+p.ToolName+" "+action), " ") + " ghi bằng tài khoản của bạn" //znf:allow-lang
	}
	out, err := json.Marshal(map[string]any{"hookSpecificOutput": map[string]string{
		"hookEventName":            "PreToolUse",
		"permissionDecision":       decision,
		"permissionDecisionReason": reason,
	}})
	if err != nil {
		return 0
	}
	_, _ = stdout.Write(append(out, '\n'))
	return 0
}
