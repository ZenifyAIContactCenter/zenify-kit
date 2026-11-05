package apply

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const claudeTrustKey = "hasTrustDialogAccepted"

// ClaudeJSON is Claude Code's per-user state file, <home>/.claude.json. The kit
// does not honour CLAUDE_CONFIG_DIR anywhere, so neither does this path.
func ClaudeJSON(home string) string { return filepath.Join(home, ".claude.json") }

// EnsureClaudeTrust sets projects["<workspace>"].hasTrustDialogAccepted = true
// in claudeJSON (FR-2.4): local-scope MCP servers only run their headersHelper
// in a trusted workspace. Only that key is touched; every other value is kept
// as its raw JSON (the top level, other projects, other keys of this project).
// A missing file is an error (Claude was never run there) and is never created.
// Errors never carry the file's content. Reports whether the file changed (or
// would, dryRun).
func EnsureClaudeTrust(claudeJSON, workspace string, dryRun bool) (bool, error) {
	fi, err := os.Stat(claudeJSON)
	if err != nil {
		return false, err
	}
	b, err := os.ReadFile(claudeJSON) //nolint:gosec // G304 -- the user's own Claude state file
	if err != nil {
		return false, err
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(b, &root); err != nil || root == nil {
		return false, fmt.Errorf("%s: not a JSON object", claudeJSON)
	}
	projects := map[string]json.RawMessage{}
	if raw, ok := root["projects"]; ok {
		if err := json.Unmarshal(raw, &projects); err != nil || projects == nil {
			return false, fmt.Errorf("%s: projects is not a JSON object", claudeJSON)
		}
	}
	entry := map[string]json.RawMessage{}
	if raw, ok := projects[workspace]; ok {
		if err := json.Unmarshal(raw, &entry); err != nil || entry == nil {
			return false, fmt.Errorf("%s: projects entry for %s is not a JSON object", claudeJSON, workspace)
		}
	}
	var accepted bool
	if raw, ok := entry[claudeTrustKey]; ok && json.Unmarshal(raw, &accepted) == nil && accepted {
		return false, nil
	}
	if dryRun {
		return true, nil
	}
	entry[claudeTrustKey] = json.RawMessage("true")
	if projects[workspace], err = marshalNoEscape(entry); err != nil {
		return false, err
	}
	if root["projects"], err = marshalNoEscape(projects); err != nil {
		return false, err
	}
	out, err := marshalNoEscape(root)
	if err != nil {
		return false, err
	}
	return true, writeAtomic(claudeJSON, out, fi.Mode().Perm())
}
