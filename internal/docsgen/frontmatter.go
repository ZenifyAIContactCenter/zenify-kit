package docsgen

import (
	"bytes"
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Frontmatter is the subset of SKILL.md / agent frontmatter the site shows.
// Bodies are agent-read (English) and are never copied (FR-2.2).
type Frontmatter struct {
	Name                   string `yaml:"name"`
	Description            string `yaml:"description"`
	ArgumentHint           string `yaml:"argument-hint"`
	AllowedTools           string `yaml:"allowed-tools"`
	DisableModelInvocation bool   `yaml:"disable-model-invocation"`
	Model                  string `yaml:"model"`
}

var fmDelim = []byte("---\n")

// ParseFrontmatter reads the leading YAML block. name is required.
func ParseFrontmatter(b []byte) (Frontmatter, error) {
	var fm Frontmatter
	if !bytes.HasPrefix(b, fmDelim) {
		return fm, errors.New("no frontmatter")
	}
	rest := b[len(fmDelim):]
	end := bytes.Index(rest, []byte("\n---"))
	if end < 0 {
		return fm, errors.New("unterminated frontmatter")
	}
	if err := yaml.Unmarshal(rest[:end], &fm); err != nil {
		return fm, fmt.Errorf("frontmatter yaml: %w", err)
	}
	if fm.Name == "" {
		return fm, errors.New("frontmatter: name is required")
	}
	return fm, nil
}
