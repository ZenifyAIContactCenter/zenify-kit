package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
	"github.com/charmbracelet/huh"
)

// secretStep prompts (masked) for bootstrap secret keys still empty in the
// workspace-level settings.local.json and writes the entered value back. NEVER
// reads-to-log, NEVER overwrites a key that already has a value (FR-069g). Headless/Accessible →
// no-op (masked prompt needs a real TTY); only reachable from the wizard's interactive TTY path
// (runWizard sets Accessible=false); the headless path never calls RunOnboard.
func secretStep(cfg OnboardConfig) error {
	if cfg.Accessible || len(cfg.SecretKeys) == 0 {
		return nil
	}
	settingsPath := filepath.Join(cfg.Workspace, ".claude", "settings.local.json")
	env, root, err := readEnvBlock(settingsPath)
	if err != nil {
		return err
	}
	var missing []string
	for _, k := range cfg.SecretKeys {
		if v, _ := env[k].(string); v == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "secrets: đã đủ, giữ nguyên") //znf:allow-lang
		return nil
	}
	values, err := promptSecrets(cfg, missing)
	if err != nil {
		return err
	}
	changed := false
	for _, k := range missing {
		if v := values[k]; v != "" { // blank input → leave the placeholder as-is
			env[k] = v
			changed = true
		}
	}
	if !changed {
		return nil
	}
	root["env"] = env
	return writeSettingsAtomic(settingsPath, root)
}

// promptSecrets shows a huh form with one masked Input per key. The
// SecretPromptFn seam replaces this in tests so no real prompt opens.
func promptSecrets(cfg OnboardConfig, keys []string) (map[string]string, error) {
	if cfg.SecretPromptFn != nil {
		return cfg.SecretPromptFn(keys)
	}
	vals := make([]*string, len(keys))
	fields := make([]huh.Field, 0, len(keys))
	for i, k := range keys {
		var s string
		vals[i] = &s
		fields = append(fields, huh.NewInput().
			Title(k).
			Description("để trống nếu điền sau"). //znf:allow-lang
			EchoMode(huh.EchoModePassword).
			Value(&s))
	}
	if err := huh.NewForm(huh.NewGroup(fields...)).Run(); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(keys))
	for i, k := range keys {
		out[k] = *vals[i]
	}
	return out, nil
}

// readEnvBlock reads settings.local.json and returns the env map + root map. Missing file →
// empty root/env. Malformed JSON or "env" not an object → error (leaves the file as-is,
// doesn't touch the live secret).
func readEnvBlock(path string) (map[string]any, map[string]any, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // G304 -- path computed internally from cfg.Workspace
	if err != nil {
		if os.IsNotExist(err) {
			env := map[string]any{}
			return env, map[string]any{"env": env}, nil
		}
		return nil, nil, err
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, nil, fmt.Errorf("settings %s is not valid JSON, left as-is: %w", path, err)
	}
	if root == nil {
		root = map[string]any{}
	}
	env, ok := root["env"].(map[string]any)
	if !ok {
		if _, present := root["env"]; present {
			return nil, nil, fmt.Errorf("settings %s has \"env\" that isn't an object, left as-is", path)
		}
		env = map[string]any{}
		root["env"] = env
	}
	return env, root, nil
}

// writeSettingsAtomic encodes canonically (2-space indent, HTML-escape OFF to preserve
// '&' in MONGO_URL) then writes atomically via temp+rename through managed.WriteFileAtomic.
func writeSettingsAtomic(path string, root map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(root); err != nil {
		return err
	}
	return managed.WriteFileAtomic(path, buf.Bytes())
}
