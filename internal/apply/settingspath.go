// internal/apply/settingspath.go
package apply

import "path/filepath"

// UserSettings is the user-scope Claude Code settings file.
func UserSettings(home string) string {
	return filepath.Join(home, ".claude", "settings.json")
}

// WorkspaceLocalSettings is the workspace-local (uncommitted) Claude Code
// settings file.
func WorkspaceLocalSettings(ws string) string {
	return filepath.Join(ws, ".claude", "settings.local.json")
}

// KitTarget names where the kit permission lists live: the settings file they
// are unioned into and the records of what the kit added to each list.
type KitTarget struct {
	Settings string

	permissionsRecord string
	askRecord         string
	sandboxRecord     string
}

// UserKitTarget targets ~/.claude/settings.json. The record paths are the
// ones earlier kits wrote, so records on installed machines stay valid.
func UserKitTarget(home string) KitTarget {
	return KitTarget{
		Settings:          UserSettings(home),
		permissionsRecord: kitPermissionsRecord(home),
		askRecord:         kitAskRecord(home),
		sandboxRecord:     kitSandboxRecord(home),
	}
}

// WorkspaceKitTarget targets <ws>/.claude/settings.local.json, with records
// under <home>/.zenify/kit-local.
func WorkspaceKitTarget(home, ws string) KitTarget {
	dir := filepath.Join(home, ".zenify", "kit-local")
	return KitTarget{
		Settings:          WorkspaceLocalSettings(ws),
		permissionsRecord: filepath.Join(dir, "permissions.json"),
		askRecord:         filepath.Join(dir, "ask.json"),
		sandboxRecord:     filepath.Join(dir, "sandbox.json"),
	}
}

// list binds a kit list to this target's settings file and record.
func (t KitTarget) list(l kitList, record string) kitList {
	l.settings = func(string) string { return t.Settings }
	l.record = func(string) string { return record }
	return l
}

func (t KitTarget) allow() kitList   { return t.list(allowList, t.permissionsRecord) }
func (t KitTarget) sandbox() kitList { return t.list(sandboxList, t.sandboxRecord) }
func (t KitTarget) ask() kitList     { return t.list(askList, t.askRecord) }
