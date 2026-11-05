// internal/apply/settingspath.go
package apply

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

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
// under <home>/.zenify/kit-local/<hash of the absolute ws path>, so each
// workspace on a machine keeps its own record of what the kit added.
func WorkspaceKitTarget(home, ws string) KitTarget {
	if abs, err := filepath.Abs(ws); err == nil {
		ws = abs
	}
	sum := sha256.Sum256([]byte(filepath.Clean(ws)))
	dir := filepath.Join(home, ".zenify", "kit-local", hex.EncodeToString(sum[:8]))
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

// SeedWorkspaceRecords starts the workspace records of a workspace that has
// none with the kit rules the user removed at user level (recorded there but
// absent from ~/.claude/settings.json), so the first workspace ensure treats
// them as user-removed and does not add them back. A list with no user record,
// or a workspace record already present, is left alone.
func SeedWorkspaceRecords(home, ws string) error {
	user, local := UserKitTarget(home), WorkspaceKitTarget(home, ws)
	for _, p := range [][2]kitList{
		{user.allow(), local.allow()},
		{user.sandbox(), local.sandbox()},
		{user.ask(), local.ask()},
	} {
		from, to := p[0], p[1]
		if _, err := os.Stat(to.record(home)); !os.IsNotExist(err) {
			continue
		}
		rec, err := from.readRecord(home)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		root, _, err := readSettingsRoot(from.settings(home))
		if err != nil {
			return err
		}
		_, list, err := from.listOf(root)
		if err != nil {
			return err
		}
		have := map[string]bool{}
		for _, a := range list {
			have[a] = true
		}
		var removed []string
		for _, r := range rec {
			if !have[r] {
				removed = append(removed, r)
			}
		}
		if len(removed) == 0 {
			continue
		}
		b, err := json.Marshal(removed)
		if err != nil {
			return err
		}
		if err := writeAtomic(to.record(home), b, 0o644); err != nil {
			return err
		}
	}
	return nil
}
