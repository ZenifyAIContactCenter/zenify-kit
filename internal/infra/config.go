package infra

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Config is the user-editable local infra configuration.
type Config struct {
	Domain   string `json:"domain"`
	MaxEnvs  int    `json:"maxEnvs"`
	ChartRef string `json:"chartRef"`
}

func defaultConfig() Config { return Config{MaxEnvs: 3, ChartRef: "main"} }

// LoadConfig reads config.json over the defaults; a missing file yields the
// defaults, a corrupt file is an error.
func LoadConfig(home string) (Config, error) {
	c := defaultConfig()
	p := filepath.Join(infraDir(home), "config.json")
	data, err := os.ReadFile(p) //nolint:gosec // G304 -- path derived from home
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("infra: read config: %w", err)
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("infra: parse %s: %w", p, err)
	}
	return c, nil
}

// SaveConfig writes config.json atomically with mode 0600.
func SaveConfig(home string, c Config) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("infra: encode config: %w", err)
	}
	return writeFileAtomic(home, "config.json", append(data, '\n'))
}
