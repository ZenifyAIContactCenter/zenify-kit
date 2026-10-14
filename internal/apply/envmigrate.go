package apply

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/devenv"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
)

// MigrateResult names the keys a migration touched. Names only, never values.
type MigrateResult struct {
	Moved     []string // had a value in a settings file; now in .zenify/.env and gone from settings
	Removed   []string // only empty placeholders existed; deleted from settings
	Conflicts []string // two settings files hold different values; left everywhere for a manual fix
}

// settingsDoc is one parsed settings file held in memory between the read and
// write phases of MigrateSettingsEnv.
type settingsDoc struct {
	path      string
	root      map[string]json.RawMessage
	env       map[string]json.RawMessage
	mode      os.FileMode
	unchanged bool // kit-owned and not edited since the kit last recorded it
	changed   bool
}

// loadSettingsDoc parses path. A missing file yields (nil, nil). A file that is
// not a JSON object, or whose "env" is not an object, is an error.
func loadSettingsDoc(path string, owned *managed.Manifest) (*settingsDoc, error) {
	b, err := os.ReadFile(path) //nolint:gosec // G304 -- settings path computed from the workspace and plan
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(b, &root); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	if root == nil {
		root = map[string]json.RawMessage{}
	}
	env, err := envOf(root)
	if err != nil {
		return nil, fmt.Errorf("%s: \"env\" is not an object", path)
	}
	d := &settingsDoc{path: path, root: root, env: env, mode: 0o600}
	if fi, err := os.Stat(path); err == nil {
		d.mode = fi.Mode().Perm()
	}
	if owned != nil {
		d.unchanged = owned.DecideRefresh(path, b) == managed.DecisionUpdate
	}
	return d, nil
}

// save writes the document back atomically, keeping an "env" object even when
// it is now empty.
func (d *settingsDoc) save() error {
	envOut, err := marshalNoEscape(d.env)
	if err != nil {
		return err
	}
	d.root["env"] = json.RawMessage(bytes.TrimSpace(envOut))
	out, err := marshalNoEscape(d.root)
	if err != nil {
		return err
	}
	return writeAtomic(d.path, out, d.mode)
}

// MigrateSettingsEnv moves keys out of the "env" block of each settings file
// into the credential file at envPath (FR-9.1–9.3, 9.5, 9.6).
//
// Every file is read first; a malformed one aborts the run before any write.
// Per key: two different non-empty values across files are a conflict and
// that key is left alone everywhere; one value is copied into envPath when
// envPath has none for the key, then deleted from every file; only
// empty placeholders are just deleted. A non-string value, or one with a line
// break (it cannot be one dotenv line), is left in place. .zenify/.env counts as
// one more place: a non-empty value there that differs is a conflict too, and
// an equal one just drops the key from settings.
//
// envPath is written BEFORE any settings file, so a failed write never leaves
// a value in neither place. A settings file the kit owns and that was unchanged
// before this run is re-recorded in owned, so its next refresh still sees it as
// kit-managed.
func MigrateSettingsEnv(settingsPaths []string, keys []string, envPath string, owned *managed.Manifest) (MigrateResult, error) {
	var docs []*settingsDoc
	for _, p := range settingsPaths {
		d, err := loadSettingsDoc(p, owned)
		if err != nil {
			return MigrateResult{}, err
		}
		if d != nil {
			docs = append(docs, d)
		}
	}
	envFile, err := devenv.Load(envPath)
	if err != nil {
		return MigrateResult{}, err
	}

	var res MigrateResult
	envDirty := false
	for _, k := range keys {
		var holders []*settingsDoc
		values := map[string]bool{}
		value, skip := "", false
		for _, d := range docs {
			raw, ok := d.env[k]
			if !ok {
				continue
			}
			var v string
			if json.Unmarshal(raw, &v) != nil || strings.ContainsAny(v, "\r\n") {
				skip = true
				break
			}
			holders = append(holders, d)
			if v != "" {
				values[v] = true
				value = v
			}
		}
		if skip || len(holders) == 0 {
			continue
		}
		cur, _ := envFile.Get(k)
		switch {
		case len(values) > 1 || (len(values) == 1 && cur != "" && cur != value):
			// .zenify/.env counts as one more place (FR-9.2).
			res.Conflicts = append(res.Conflicts, k)
			continue
		case len(values) == 0:
			res.Removed = append(res.Removed, k)
		default:
			if cur == "" {
				envFile.Set(k, value)
				envDirty = true
			}
			res.Moved = append(res.Moved, k)
		}
		for _, d := range holders {
			delete(d.env, k)
			d.changed = true
		}
	}

	if envDirty {
		if err := envFile.Save(envPath); err != nil {
			return MigrateResult{}, fmt.Errorf("write %s: %w", envPath, err)
		}
	}
	for _, d := range docs {
		if !d.changed {
			continue
		}
		if err := d.save(); err != nil {
			return res, fmt.Errorf("write %s: %w", d.path, err)
		}
		if owned != nil && d.unchanged {
			if err := owned.Record(d.path); err != nil {
				return res, err
			}
		}
	}
	return res, nil
}

// LeftoverEnvKeys returns, per settings file, the sorted names of "env" keys
// whose value is anything but "" or null (FR-8.3). Missing files are skipped;
// files with no such key are absent from the map.
func LeftoverEnvKeys(settingsPaths []string) (map[string][]string, error) {
	out := map[string][]string{}
	for _, p := range settingsPaths {
		d, err := loadSettingsDoc(p, nil)
		if err != nil {
			return nil, err
		}
		if d == nil {
			continue
		}
		var names []string
		for k, raw := range d.env {
			if t := string(bytes.TrimSpace(raw)); t != `""` && t != "null" {
				names = append(names, k)
			}
		}
		if len(names) > 0 {
			sort.Strings(names)
			out[p] = names
		}
	}
	return out, nil
}
