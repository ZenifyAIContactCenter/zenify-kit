// Package manifest reads the repo-manifest (FR-068): the desired set of repos
// zenify onboarding may wire, versioned inside the kit. Read-only.
package manifest

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Repo is one entry in the manifest desired-set.
type Repo struct {
	Name string   `yaml:"name"`
	URL  string   `yaml:"url"`
	Path string   `yaml:"path"` // relative to workspace root, overridable via overlay
	Base string   `yaml:"base"`
	Tags []string `yaml:"tags"`
}

// Manifest is the full desired-set plus the GitHub org that owns it.
type Manifest struct {
	Org        string   `yaml:"org"`
	Repos      []Repo   `yaml:"repos"`
	SecretKeys []string `yaml:"secretKeys"` // env keys the kit scaffolds as empty placeholders (FR-065); values never distributed
}

// Load parses a manifest YAML file. A missing file, empty org, or empty repo
// list is an error — the manifest is required input, not optional.
func Load(path string) (*Manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m Manifest
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if m.Org == "" {
		return nil, fmt.Errorf("manifest %s: org is required", path)
	}
	if len(m.Repos) == 0 {
		return nil, fmt.Errorf("manifest %s: repos is empty", path)
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
	ob, err := os.ReadFile(overlayPath)
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
