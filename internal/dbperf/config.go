package dbperf

import (
	"encoding/json"
	"os"
)

// Config holds the tenant-classification lists and the numeric thresholds.
// It is read at runtime from the knowledge store; absent keys keep Defaults().
type Config struct {
	TenantScoped      []string `json:"tenant_scoped"`
	Global            []string `json:"global"`
	LargeCount        int      `json:"large_count"`
	ScanRatioBlock    int      `json:"scan_ratio_block"`
	ScanRatioAdvisory int      `json:"scan_ratio_advisory"`
	SkipLarge         int      `json:"skip_large"`
}

// Defaults are the fallback thresholds when the config file or a key is absent.
func Defaults() Config {
	return Config{
		LargeCount:        100000,
		ScanRatioBlock:    100,
		ScanRatioAdvisory: 10,
		SkipLarge:         1000,
	}
}

// Load reads the JSON config at path over Defaults(). A missing file is NOT an
// error (fail-open): the gate must run with defaults even before the store exists.
func Load(path string) (Config, error) {
	c := Defaults()
	b, err := os.ReadFile(path) //nolint:gosec // G304 -- path is the knowledge-store config location computed by resolveDocsStore, not externally-tainted
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return c, err
	}
	// Unmarshal over c so absent numeric keys keep their default (JSON leaves
	// missing fields untouched; explicit 0 is not expected in this config).
	if err := json.Unmarshal(b, &c); err != nil {
		return Defaults(), err
	}
	return c, nil
}
