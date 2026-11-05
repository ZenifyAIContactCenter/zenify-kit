package plugin

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
)

const (
	MarketplaceName = "zenify-kit"
	PluginID        = "znf@" + MarketplaceName
)

const marketplaceJSON = `{
  "name": "zenify-kit",
  "owner": {"name": "zenify"},
  "plugins": [{"name": "znf", "source": "./plugins/znf"}]
}
`

func MarketplaceDir(zenifyHome string) string {
	return filepath.Join(zenifyHome, "plugin-marketplace")
}

func PluginDir(zenifyHome string) string {
	return filepath.Join(MarketplaceDir(zenifyHome), "plugins", "znf")
}

func PluginManifest(zenifyHome string) string {
	return filepath.Join(PluginDir(zenifyHome), ".manifest.json")
}

// SyncMarketplace materializes the znf plugin as a directory marketplace under zenifyHome,
// stamping plugin.json with the running kit version.
func SyncMarketplace(zenifyHome string) (Result, error) {
	res, err := Sync(PluginDir(zenifyHome), PluginManifest(zenifyHome))
	if err != nil {
		return res, err
	}
	pj := filepath.Join(PluginDir(zenifyHome), ".claude-plugin", "plugin.json")
	raw, err := os.ReadFile(pj) //nolint:gosec // G304 -- path computed from zenifyHome internally
	if err != nil {
		return res, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return res, err
	}
	v, err := json.Marshal(version.Current())
	if err != nil {
		return res, err
	}
	if !bytes.Equal(m["version"], v) {
		m["version"] = v
		out, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			return res, err
		}
		if err := writeIfChanged(pj, append(out, '\n')); err != nil {
			return res, err
		}
	}
	mp := filepath.Join(MarketplaceDir(zenifyHome), ".claude-plugin", "marketplace.json")
	return res, writeIfChanged(mp, []byte(marketplaceJSON))
}

// writeIfChanged writes content atomically (temp file + rename), skipping an identical file.
func writeIfChanged(path string, content []byte) error {
	if cur, err := os.ReadFile(path); err == nil && bytes.Equal(cur, content) { //nolint:gosec // G304 -- internal path
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // best effort; gone after rename
	if _, err := tmp.Write(content); err != nil {
		tmp.Close() //nolint:errcheck,gosec
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
