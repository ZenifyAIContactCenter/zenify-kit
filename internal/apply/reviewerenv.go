// internal/apply/reviewerenv.go
package apply

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ReviewerEnv is the env var `zenify reviewer` reads to pick the review
// engine. Values: "agent" (the Claude reviewer subagents) or "codex".
const ReviewerEnv = "ZNF_REVIEWER"

// ReviewerDefault is what the kit seeds: agent.
const ReviewerDefault = "agent"

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

// SetReviewerEnv overwrites env.ZNF_REVIEWER with value (agent or codex).
func SetReviewerEnv(settingsPath, value string, dryRun bool) error {
	if value != "agent" && value != "codex" {
		return fmt.Errorf("reviewer %q: want agent or codex", value)
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
	if err := json.Unmarshal(raw, &v); err != nil || (v != "agent" && v != "codex") {
		return "", fmt.Errorf("settings.json env.%s = %s: want agent or codex", ReviewerEnv, raw)
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
