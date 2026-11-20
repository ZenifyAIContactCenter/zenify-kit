// internal/apply/reviewerenv.go
package apply

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ReviewerEnv is the env var `zenify reviewer` reads to pick the review
// engine. Values: "agent" (the Claude reviewer subagents), "codex", "project" (the review the project's CLAUDE.md declares), or "agent+project" (the Claude review, then the project's).
const ReviewerEnv = "ZNF_REVIEWER"

// ReviewerDefault is what the kit seeds: agent+project.
const ReviewerDefault = "agent+project"

// validReviewer reports whether v is a ZNF_REVIEWER value.
func validReviewer(v string) bool {
	return v == "agent" || v == "codex" || v == "project" || v == "agent+project"
}

// EnsureReviewerEnv sets env.ZNF_REVIEWER in the settings file at settingsPath
// only when the key is absent (same contract as EnsureSubagentModelEnv).
func EnsureReviewerEnv(settingsPath string, dryRun bool) (bool, error) {
	root, mode, err := readSettingsRoot(settingsPath)
	if err != nil {
		return false, err
	}
	env, err := envOf(root)
	if err != nil {
		return false, err
	}
	if _, ok := env[ReviewerEnv]; ok {
		return false, nil
	}
	if dryRun {
		return true, nil
	}
	val, _ := json.Marshal(ReviewerDefault)
	env[ReviewerEnv] = json.RawMessage(val)
	return true, writeEnv(settingsPath, root, env, mode)
}

// SetReviewerEnv overwrites env.ZNF_REVIEWER with value (agent, codex, project or agent+project).
func SetReviewerEnv(settingsPath, value string, dryRun bool) error {
	if !validReviewer(value) {
		return fmt.Errorf("reviewer %q: want agent, codex, project or agent+project", value)
	}
	root, mode, err := readSettingsRoot(settingsPath)
	if err != nil {
		return err
	}
	env, err := envOf(root)
	if err != nil {
		return err
	}
	if dryRun {
		return nil
	}
	val, _ := json.Marshal(value)
	env[ReviewerEnv] = json.RawMessage(val)
	return writeEnv(settingsPath, root, env, mode)
}

// ReviewerEnvValue returns the stored env.ZNF_REVIEWER of the settings file at settingsPath:
// ReviewerDefault when the file or the key is absent, an error for any other value.
func ReviewerEnvValue(settingsPath string) (string, error) {
	root, _, err := readSettingsRoot(settingsPath)
	if err != nil {
		return "", err
	}
	env, err := envOf(root)
	if err != nil {
		return "", err
	}
	raw, ok := env[ReviewerEnv]
	if !ok {
		return ReviewerDefault, nil
	}
	var v string
	if err := json.Unmarshal(raw, &v); err != nil || !validReviewer(v) {
		return "", fmt.Errorf("settings.json env.%s = %s: want agent, codex, project or agent+project", ReviewerEnv, raw)
	}
	return v, nil
}

// RemoveReviewerEnv removes the key only while it still holds the kit
// default, so a value the user changed stays.
func RemoveReviewerEnv(settingsPath string, dryRun bool) (bool, error) {
	root, mode, err := readSettingsRoot(settingsPath)
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
	cur, ok := env[ReviewerEnv]
	if !ok {
		return false, nil
	}
	want, _ := json.Marshal(ReviewerDefault)
	if !bytes.Equal(bytes.TrimSpace(cur), want) {
		return false, nil
	}
	if dryRun {
		return true, nil
	}
	delete(env, ReviewerEnv)
	return true, writeEnv(settingsPath, root, env, mode)
}

// KitReviewerDefaultRecord is the JSON list of workspaces already migrated off the old "agent" default.
func KitReviewerDefaultRecord(home string) string {
	return filepath.Join(home, ".zenify", "kit-reviewer-default.json")
}

// MigrateReviewerDefault moves a workspace the old kit seeded with "agent" to
// ReviewerDefault, once: the workspace (the directory two levels above
// settingsPath, symlinks resolved) is then added to the record at recordPath, and
// a later "agent" stays. Any other value is left alone, but the workspace is still
// recorded. A malformed record is an error and changes nothing: it cannot prove the
// workspace was never set explicitly. Returns whether the value changed; dryRun
// writes nothing.
func MigrateReviewerDefault(settingsPath, recordPath string, dryRun bool) (bool, error) {
	ws := migrationKey(settingsPath)
	if !dryRun {
		defer lockMigrationRecord(recordPath)()
	}
	seen, err := readMigrationRecord(recordPath)
	if err != nil {
		return false, err
	}
	for _, p := range seen {
		if p == ws {
			return false, nil
		}
	}
	root, mode, err := readSettingsRoot(settingsPath)
	if err != nil {
		return false, err
	}
	env, err := envOf(root)
	if err != nil {
		return false, err
	}
	agent, _ := json.Marshal("agent")
	changed := bytes.Equal(bytes.TrimSpace(env[ReviewerEnv]), agent)
	if dryRun {
		return changed, nil
	}
	if changed {
		val, _ := json.Marshal(ReviewerDefault)
		env[ReviewerEnv] = json.RawMessage(val)
		if err := writeEnv(settingsPath, root, env, mode); err != nil {
			return false, err
		}
	}
	return changed, writeMigrationRecord(recordPath, append(seen, ws))
}

// SetReviewerExplicit writes value as the workspace's ZNF_REVIEWER and records the
// workspace as migrated, both under the migration record's lock, so a concurrent
// MigrateReviewerDefault can neither overwrite the choice between the write and the
// record nor read the old value before it. written reports whether the value reached
// the settings, so a record-only failure can be told from a failed write.
func SetReviewerExplicit(settingsPath, recordPath, value string) (written bool, err error) {
	defer lockMigrationRecord(recordPath)()
	if err := SetReviewerEnv(settingsPath, value, false); err != nil {
		return false, err
	}
	return true, recordMigratedLocked(settingsPath, recordPath)
}

// recordMigratedLocked adds the workspace to the record; the caller holds the lock.
func recordMigratedLocked(settingsPath, recordPath string) error {
	ws := migrationKey(settingsPath)
	seen, err := readMigrationRecord(recordPath)
	if err != nil {
		return err
	}
	for _, p := range seen {
		if p == ws {
			return nil
		}
	}
	return writeMigrationRecord(recordPath, append(seen, ws))
}

// migrationKey is the record key of the workspace two levels above settingsPath:
// the cleaned path with symlinks resolved, or the cleaned path when that fails.
func migrationKey(settingsPath string) string {
	ws := filepath.Clean(filepath.Dir(filepath.Dir(settingsPath)))
	if r, err := filepath.EvalSymlinks(ws); err == nil {
		return r
	}
	return ws
}

// lockMigrationRecord takes a blocking exclusive advisory lock on <recordPath>.lock
// so concurrent ensures do not lose each other's read-modify-write of the record,
// and returns its release. Any failure proceeds unlocked (fail-open): the record
// only keeps a one-time migration from repeating, and a session start must never
// block or fail on it.
func lockMigrationRecord(recordPath string) func() {
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o750); err != nil {
		return func() {}
	}
	f, err := os.OpenFile(recordPath+".lock", os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // G304 -- fixed path under the caller's home dir
	if err != nil {
		return func() {}
	}
	if err := lockFileBlocking(f); err != nil {
		_ = f.Close()
		return func() {}
	}
	return func() {
		_ = unlockFileBlocking(f)
		_ = f.Close()
	}
}

func readMigrationRecord(recordPath string) ([]string, error) {
	var seen []string
	if raw, err := os.ReadFile(recordPath); err == nil { //nolint:gosec // G304 -- fixed path under the caller's home dir
		if err := json.Unmarshal(raw, &seen); err != nil {
			return nil, fmt.Errorf("%s record malformed: %w", filepath.Base(recordPath), err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return seen, nil
}

func writeMigrationRecord(recordPath string, list []string) error {
	out, err := json.Marshal(list)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o750); err != nil {
		return err
	}
	return writeAtomic(recordPath, out, 0o600)
}
