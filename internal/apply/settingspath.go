// internal/apply/settingspath.go
package apply

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

// removedRecord is where KeepUserKitRemovals keeps the rules the user removed
// at user level once the migration has deleted that list's record. It is a
// separate file because the record itself means "the kit owns these rules":
// written back there, a rule the user later adds by hand would be removed.
func removedRecord(record string) string { return record + ".removed" }

// userRemovals returns, keyed by the user-level record path, the kit rules
// the user removed at user level: recorded there but absent from
// ~/.claude/settings.json, plus those kept by an earlier migration.
func userRemovals(home string) (map[string][]string, error) {
	out := map[string][]string{}
	user := UserKitTarget(home)
	for _, l := range []kitList{user.allow(), user.sandbox(), user.ask()} {
		record := l.record(home)
		var removed []string
		if b, err := os.ReadFile(removedRecord(record)); err == nil {
			if err := json.Unmarshal(b, &removed); err != nil {
				return nil, fmt.Errorf("%s: %w", removedRecord(record), err)
			}
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		rec, err := l.readRecord(home)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if len(rec) > 0 {
			root, _, err := readSettingsRoot(l.settings(home))
			if err != nil {
				return nil, err
			}
			_, list, err := l.listOf(root)
			if err != nil {
				return nil, err
			}
			have := map[string]bool{}
			for _, a := range list {
				have[a] = true
			}
			seen := map[string]bool{}
			for _, r := range removed {
				seen[r] = true
			}
			for _, r := range rec {
				if !have[r] && !seen[r] {
					removed = append(removed, r)
				}
			}
		}
		if len(removed) > 0 {
			out[record] = removed
		}
	}
	return out, nil
}

// KeepUserKitRemovals saves the rules the user removed at user level beside
// their records, then runs remove (the migration of the user-level kit lists,
// which deletes the records), so SeedWorkspaceRecords still finds them for a
// workspace set up later, even when remove fails part-way.
func KeepUserKitRemovals(home string, remove func() error) error {
	rm, err := userRemovals(home)
	if err != nil {
		return err
	}
	for record, rules := range rm {
		b, err := json.Marshal(rules)
		if err != nil {
			return err
		}
		if err := writeAtomic(removedRecord(record), b, 0o644); err != nil {
			return err
		}
	}
	return remove()
}

// SeedWorkspaceRecords starts the workspace records of a workspace that has
// none with the kit rules the user removed at user level (recorded there but
// absent from ~/.claude/settings.json), so the first workspace ensure treats
// them as user-removed and does not add them back. A list with no user
// removals, or a workspace record already present, is left alone.
func SeedWorkspaceRecords(home, ws string) error {
	rm, err := userRemovals(home)
	if err != nil {
		return err
	}
	user, local := UserKitTarget(home), WorkspaceKitTarget(home, ws)
	for _, p := range [][2]kitList{
		{user.allow(), local.allow()},
		{user.sandbox(), local.sandbox()},
		{user.ask(), local.ask()},
	} {
		rules, ok := rm[p[0].record(home)]
		to := p[1]
		if !ok {
			continue
		}
		if _, err := os.Stat(to.record(home)); !os.IsNotExist(err) {
			continue
		}
		// A rule already in the workspace list is the user's own: seeding it
		// would mark it kit-owned and down would remove it.
		root, _, err := readSettingsRoot(to.settings(home))
		if err != nil {
			return err
		}
		_, list, err := to.listOf(root)
		if err != nil {
			return err
		}
		have := map[string]bool{}
		for _, a := range list {
			have[a] = true
		}
		var seed []string
		for _, r := range rules {
			if !have[r] {
				seed = append(seed, r)
			}
		}
		if len(seed) == 0 {
			continue
		}
		b, err := json.Marshal(seed)
		if err != nil {
			return err
		}
		if err := writeAtomic(to.record(home), b, 0o644); err != nil {
			return err
		}
	}
	return nil
}
