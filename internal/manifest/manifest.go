// Package manifest reads the repo-manifest (FR-068): the desired set of repos
// zenify onboarding may wire, versioned inside the kit. Read-only.
package manifest

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// SecretKey describes one env key the kit manages: its name plus the metadata a
// teammate needs to fill it. Never carries a value (FR-041).
type SecretKey struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	HowTo       string `yaml:"howTo"`  // who or where hands the value out; never the value itself
	Scope       string `yaml:"scope"`  // "personal" (each dev owns it) or "shared" (team-wide)
	Secret      bool   `yaml:"secret"` // false → value is safe to echo (host, port, URL)
}

// UnmarshalYAML accepts a bare string (name only, Secret true) or the object form.
// In the object form an omitted `secret` also defaults to true.
func (k *SecretKey) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		*k = SecretKey{Name: n.Value, Secret: true}
		return nil
	}
	type plain SecretKey
	p := plain{Secret: true}
	if err := n.Decode(&p); err != nil {
		return err
	}
	*k = SecretKey(p)
	return nil
}

// KeyNames returns the Name of each key, order kept.
func KeyNames(keys []SecretKey) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k.Name)
	}
	return out
}

// AllKeys is the global keys followed by each repo's own, deduplicated by Name, order kept.
func (m *Manifest) AllKeys() []SecretKey {
	seen := map[string]bool{}
	var out []SecretKey
	add := func(list []SecretKey) {
		for _, k := range list {
			if !seen[k.Name] {
				seen[k.Name] = true
				out = append(out, k)
			}
		}
	}
	add(m.SecretKeys)
	for _, r := range m.Repos {
		add(r.SecretKeys)
	}
	return out
}

// Repo is one entry in the manifest desired-set.
type Repo struct {
	Name       string      `yaml:"name"`
	URL        string      `yaml:"url"`
	Path       string      `yaml:"path"` // relative to workspace root, overridable via overlay
	Base       string      `yaml:"base"`
	Tags       []string    `yaml:"tags"`
	SecretKeys []SecretKey `yaml:"secretKeys"` // repo-only env keys on top of Manifest.SecretKeys; names only, never values (FR-041)
}

// Manifest is the full desired-set plus the GitHub org that owns it.
type Manifest struct {
	Org        string      `yaml:"org"`
	Repos      []Repo      `yaml:"repos"`
	SecretKeys []SecretKey `yaml:"secretKeys"` // env keys kept in <workspace>/.zenify/.env; names only, values never distributed
}

// Load parses a manifest YAML file. A missing file, empty org, or empty repo
// list is an error — the manifest is required input, not optional.
func Load(path string) (*Manifest, error) {
	b, err := os.ReadFile(path) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	return Parse(b, path)
}

// Parse parses manifest YAML bytes. label names the source in error messages
// (a file path, or "embedded" for the compiled-in default).
func Parse(b []byte, label string) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if m.Org == "" {
		return nil, fmt.Errorf("manifest %s: org is required", label)
	}
	if len(m.Repos) == 0 {
		return nil, fmt.Errorf("manifest %s: repos is empty", label)
	}
	return &m, nil
}

// LoadWithOverlay loads base, then lets a personal overlay override each repo's
// Path by Name (FR-068 local overlay). A missing overlay file is not an error.
func LoadWithOverlay(basePath, overlayPath string) (*Manifest, error) {
	m, err := Load(basePath)
	if err != nil {
		return nil, err
	}
	return applyOverlay(m, overlayPath)
}

// ParseWithOverlay is LoadWithOverlay for in-memory manifest bytes (the
// embedded default). Same overlay semantics.
func ParseWithOverlay(b []byte, label, overlayPath string) (*Manifest, error) {
	m, err := Parse(b, label)
	if err != nil {
		return nil, err
	}
	return applyOverlay(m, overlayPath)
}

func applyOverlay(m *Manifest, overlayPath string) (*Manifest, error) {
	ob, err := os.ReadFile(overlayPath) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read overlay: %w", err)
	}
	var ov struct {
		Repos []struct {
			Name string `yaml:"name"`
			Path string `yaml:"path"`
		} `yaml:"repos"`
	}
	if err := yaml.Unmarshal(ob, &ov); err != nil {
		return nil, fmt.Errorf("parse overlay: %w", err)
	}
	for _, o := range ov.Repos {
		for i := range m.Repos {
			if m.Repos[i].Name == o.Name && o.Path != "" {
				m.Repos[i].Path = o.Path
			}
		}
	}
	return m, nil
}

// ByName returns the manifest entry for name.
func (m *Manifest) ByName(name string) (Repo, bool) {
	for _, r := range m.Repos {
		if r.Name == name {
			return r, true
		}
	}
	return Repo{}, false
}
