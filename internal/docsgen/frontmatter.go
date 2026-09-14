package docsgen

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"strings"
)

// Frontmatter is the subset of SKILL.md / agent frontmatter the site shows.
// Bodies are agent-read (English) and are never copied (FR-2.2).
type Frontmatter struct {
	Name                   string
	Description            string
	ArgumentHint           string
	AllowedTools           string
	DisableModelInvocation bool
	Model                  string
}

var fmDelim = []byte("---\n")

// ParseFrontmatter reads the leading `---`-delimited frontmatter block with a
// line-based parser (not YAML): every real SKILL.md/agent frontmatter block
// is `key: value` on one line, and several real descriptions contain an
// unquoted "word: word" inside the value, which a strict YAML mapping
// rejects. Each line is split on the FIRST ':', so that is safe here. name
// is required; unknown keys are ignored.
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
	scanner := bufio.NewScanner(bytes.NewReader(rest[:end]))
	line := 1
	for scanner.Scan() {
		line++
		raw := scanner.Text()
		if strings.TrimSpace(raw) == "" {
			continue
		}
		idx := strings.Index(raw, ":")
		if idx < 0 {
			return fm, fmt.Errorf("frontmatter line %d: expected \"key: value\": %q", line, raw)
		}
		key := raw[:idx]
		if !isKey(key) {
			return fm, fmt.Errorf("frontmatter line %d: invalid key %q", line, key)
		}
		val := unquote(strings.TrimSpace(raw[idx+1:]))
		switch key {
		case "name":
			fm.Name = val
		case "description":
			fm.Description = val
		case "argument-hint":
			fm.ArgumentHint = val
		case "allowed-tools":
			fm.AllowedTools = val
		case "disable-model-invocation":
			fm.DisableModelInvocation = val == "true"
		case "model":
			fm.Model = val
		}
	}
	if err := scanner.Err(); err != nil {
		return fm, err
	}
	if fm.Name == "" {
		return fm, errors.New("frontmatter: name is required")
	}
	return fm, nil
}

// isKey reports whether s matches ^[A-Za-z][A-Za-z0-9-]*$.
func isKey(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9' && i > 0:
		case r == '-' && i > 0:
		default:
			return false
		}
	}
	return true
}

// unquote strips one matching pair of surrounding ' or " quotes, if present.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
