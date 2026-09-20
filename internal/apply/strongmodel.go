// internal/apply/strongmodel.go
package apply

import (
	"bytes"
	"encoding/json"
	"path/filepath"
)

// StrongModelEnv is the env var `select-route` reads to name the strong model
// for architect / round-3 investigator / blocked-twice reviewer / manual
// advisor dispatches. Values: "fable" or "opus"; anything else means opus.
const StrongModelEnv = "ZNF_STRONG_MODEL"

// StrongModelDefault is what the kit seeds: opus. A machine whose account has
// the stronger tier edits this one value by hand. The kit never probes access
// and never warns — a dispatch that names an unavailable model would print a
// harness warning on every teammate's screen, so the name must never be sent.
const StrongModelDefault = "opus"

// EnsureStrongModelEnv sets env.ZNF_STRONG_MODEL in <home>/.claude/settings.json
// only when the key is absent (same contract as EnsureSubagentModelEnv).
func EnsureStrongModelEnv(home string, dryRun bool) (bool, error) {
	path := filepath.Join(home, ".claude", "settings.json")
	root, mode, err := readSettingsRoot(path)
	if err != nil {
		return false, err
	}
	env, err := envOf(root)
	if err != nil {
		return false, err
	}
	if _, ok := env[StrongModelEnv]; ok {
		return false, nil
	}
	if dryRun {
		return true, nil
	}
	val, _ := json.Marshal(StrongModelDefault)
	env[StrongModelEnv] = json.RawMessage(val)
	return true, writeEnv(path, root, env, mode)
}

// RemoveStrongModelEnv removes the key only while it still holds the kit
// default, so a value the user changed stays.
func RemoveStrongModelEnv(home string, dryRun bool) (bool, error) {
	path := filepath.Join(home, ".claude", "settings.json")
	root, mode, err := readSettingsRoot(path)
	if err != nil {
		return false, err
	}
	if root == nil {
		return false, nil
	}
	env, err := envOf(root)
	if err != nil {
		return false, err
	}
	cur, ok := env[StrongModelEnv]
	if !ok {
		return false, nil
	}
	want, _ := json.Marshal(StrongModelDefault)
	if !bytes.Equal(bytes.TrimSpace(cur), want) {
		return false, nil
	}
	if dryRun {
		return true, nil
	}
	delete(env, StrongModelEnv)
	return true, writeEnv(path, root, env, mode)
}
