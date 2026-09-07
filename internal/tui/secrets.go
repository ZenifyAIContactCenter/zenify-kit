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

// secretStep prompt masked các bootstrap secret key còn rỗng trong
// settings.local.json cấp workspace và ghi value đã nhập trở lại. KHÔNG bao
// giờ đọc-để-log, KHÔNG đè key đã có value (FR-069g). Headless/Accessible →
// no-op (prompt masked cần TTY thật); chỉ đạt tới từ wizard TTY interactive
// (runWizard set Accessible=false), path headless không gọi RunOnboard.
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
		_, _ = fmt.Fprintln(os.Stdout, "secrets: đã đủ, giữ nguyên")
		return nil
	}
	values, err := promptSecrets(cfg, missing)
	if err != nil {
		return err
	}
	changed := false
	for _, k := range missing {
		if v := values[k]; v != "" { // gõ trống → để nguyên placeholder
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

// promptSecrets hiện một form huh với một Input masked mỗi key. Seam
// SecretPromptFn thay thế trong test để không mở prompt thật.
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
			Description("để trống nếu điền sau").
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

// readEnvBlock đọc settings.local.json và trả env map + root map. File vắng →
// root/env rỗng. JSON hỏng hoặc "env" không phải object → lỗi (để nguyên file,
// không đụng secret live).
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
		return nil, nil, fmt.Errorf("settings %s không phải JSON hợp lệ, để nguyên: %w", path, err)
	}
	if root == nil {
		root = map[string]any{}
	}
	env, ok := root["env"].(map[string]any)
	if !ok {
		if _, present := root["env"]; present {
			return nil, nil, fmt.Errorf("settings %s có \"env\" không phải object, để nguyên", path)
		}
		env = map[string]any{}
		root["env"] = env
	}
	return env, root, nil
}

// writeSettingsAtomic mã hoá canonical (2-space, HTML-escape OFF để giữ nguyên
// '&' trong MONGO_URL) rồi ghi atomic temp+rename qua managed.WriteFileAtomic.
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
