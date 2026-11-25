package infra

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	p := ConfigPath(home)
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

// ConfigPath is the config file LoadConfig reads.
func ConfigPath(home string) string { return filepath.Join(infraDir(home), "config.json") }

// ValidateDomain requires a lower-case DNS name under .test.
func ValidateDomain(d string) error {
	if len(d) > 253 || !domainRe.MatchString(d) || !strings.HasSuffix(d, ".test") || d == ".test" {
		return fmt.Errorf("domain %q must be lower-case DNS labels (a-z, 0-9, -) ending in .test", d)
	}
	return nil
}

// Validate checks values a hand-edited config.json may break.
func (c Config) Validate() error {
	if c.MaxEnvs < 1 {
		return fmt.Errorf("maxEnvs must be at least 1, got %d", c.MaxEnvs)
	}
	if c.Domain != "" {
		if err := ValidateDomain(c.Domain); err != nil {
			return err
		}
	}
	if c.ChartRef == "" {
		return errors.New("chartRef must not be empty")
	}
	return nil
}
