package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/devenv"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
	"github.com/charmbracelet/huh"
)

// secretStep prompts (masked) for bootstrap secret keys still empty in the
// workspace credential file <workspace>/.zenify/.env and writes the entered
// value back. NEVER reads-to-log, NEVER overwrites a key that already has a
// value (FR-069g). Headless/Accessible → no-op (masked prompt needs a real
// TTY); only reachable from the wizard's interactive TTY path (runWizard sets
// Accessible=false); the headless path never calls RunOnboard.
func secretStep(cfg OnboardConfig) error {
	if cfg.Accessible || len(cfg.SecretKeys) == 0 {
		return nil
	}
	path := devenv.Path(cfg.Workspace)
	f, err := devenv.Load(path)
	if err != nil {
		return err
	}
	var missing []manifest.SecretKey
	for _, k := range cfg.SecretKeys {
		if v, _ := f.Get(k.Name); v == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "secrets: đã đủ, giữ nguyên") //znf:allow-lang
		return nil
	}
	values, err := PromptSecrets(missing, cfg.SecretPromptFn)
	if err != nil {
		return err
	}
	for _, k := range missing {
		if strings.ContainsAny(values[k.Name], "\r\n") { // a newline would inject a second line
			return fmt.Errorf("giá trị của %s có ký tự xuống dòng, không ghi", k.Name) //znf:allow-lang
		}
	}
	changed := false
	for _, k := range missing {
		if v := values[k.Name]; v != "" { // blank input → leave the key unset
			f.Set(k.Name, v)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return f.Save(path)
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
