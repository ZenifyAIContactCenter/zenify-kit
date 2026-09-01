package managed

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
)

// Entry records one file zenify wrote and the fingerprint it wrote.
type Entry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// Manifest is the set of files zenify owns on this machine.
type Manifest struct {
	Entries map[string]Entry `json:"entries"`
}

// Fingerprint returns the hex sha256 of b.
func Fingerprint(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Load reads a manifest from disk; a missing file yields an empty manifest.
func Load(path string) (*Manifest, error) {
	b, err := os.ReadFile(path) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if os.IsNotExist(err) {
		return &Manifest{Entries: map[string]Entry{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if m.Entries == nil {
		m.Entries = map[string]Entry{}
	}
	return &m, nil
}

// Save writes the manifest as pretty JSON.
func (m *Manifest) Save(path string) error {
	if m.Entries == nil {
		m.Entries = map[string]Entry{}
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// Record reads filePath, fingerprints it, and stores/updates its entry.
func (m *Manifest) Record(filePath string) error {
	if m.Entries == nil {
		m.Entries = map[string]Entry{}
	}
	b, err := os.ReadFile(filePath) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if err != nil {
		return err
	}
	m.Entries[filePath] = Entry{Path: filePath, SHA256: Fingerprint(b)}
	return nil
}

// Get returns the recorded entry for filePath.
func (m *Manifest) Get(filePath string) (Entry, bool) {
	e, ok := m.Entries[filePath]
	return e, ok
}
