package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
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

// promptSecrets adapts the wizard's name list to PromptSecrets, keeping the
// SecretPromptFn seam so no real prompt opens in tests.
func promptSecrets(cfg OnboardConfig, keys []string) (map[string]string, error) {
	ks := make([]manifest.SecretKey, len(keys))
	for i, k := range keys {
		ks[i] = manifest.SecretKey{Name: k, Secret: true}
	}
	return PromptSecrets(ks, cfg.SecretPromptFn)
}

// PromptSecrets shows a huh form with one masked Input per key (title = key
// name, description = Description plus HowTo when set). promptFn, when non-nil,
// replaces the form (test seam) and receives the key names.
func PromptSecrets(keys []manifest.SecretKey, promptFn func([]string) (map[string]string, error)) (map[string]string, error) {
	if promptFn != nil {
		return promptFn(manifest.KeyNames(keys))
	}
	vals := make([]*string, len(keys))
	fields := make([]huh.Field, 0, len(keys))
	for i, k := range keys {
		var s string
		vals[i] = &s
		fields = append(fields, huh.NewInput().
			Title(k.Name).
			Description(secretDescription(k)).
			EchoMode(huh.EchoModePassword).
			Value(&s))
	}
	if err := huh.NewForm(huh.NewGroup(fields...)).Run(); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(keys))
	for i, k := range keys {
		out[k.Name] = *vals[i]
	}
	return out, nil
}

func secretDescription(k manifest.SecretKey) string {
	var parts []string
	if k.Description != "" {
		parts = append(parts, k.Description)
	}
	if k.HowTo != "" {
		parts = append(parts, k.HowTo)
	}
	if len(parts) == 0 {
		return "để trống nếu điền sau" //znf:allow-lang
	}
	return strings.Join(parts, " — ")
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

// PickKey asks which key to fill or edit next. It returns "" when the user is done.
func PickKey(names []string) (string, error) {
	opts := []huh.Option[string]{huh.NewOption("xong", "")} //znf:allow-lang
	for _, n := range names {
		opts = append(opts, huh.NewOption(n, n))
	}
	var pick string
	sel := huh.NewSelect[string]().
		Title("Chọn key để điền hoặc sửa"). //znf:allow-lang
		Options(opts...).
		Value(&pick)
	if err := huh.NewForm(huh.NewGroup(sel)).Run(); err != nil {
		return "", err
	}
	return pick, nil
}
