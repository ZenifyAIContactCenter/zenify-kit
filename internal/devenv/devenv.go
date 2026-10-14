// Package devenv reads and writes the workspace credential file
// <workspace>/.zenify/.env and resolves values env-first.
package devenv

import (
	"os"
	"path/filepath"
	"strings"
)

// Path returns the credential file location for a workspace.
func Path(workspace string) string {
	return filepath.Join(workspace, ".zenify", ".env")
}

// File keeps the raw lines so Save preserves order and comments.
type File struct {
	lines []string
}

// Load parses path. A missing file yields an empty File.
func Load(path string) (*File, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path is the workspace credential file
	if os.IsNotExist(err) {
		return &File{}, nil
	}
	if err != nil {
		return nil, err
	}
	s := strings.TrimSuffix(string(b), "\n")
	if s == "" {
		return &File{}, nil
	}
	return &File{lines: strings.Split(s, "\n")}, nil
}

// parseLine returns the key and value of a KEY=value line.
func parseLine(line string) (key, value string, ok bool) {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, "#") {
		return "", "", false
	}
	i := strings.IndexByte(t, '=')
	if i <= 0 {
		return "", "", false
	}
	key = strings.TrimSpace(t[:i])
	return key, parseValue(strings.TrimSpace(t[i+1:])), true
}

func parseValue(v string) string {
	if v == "" {
		return ""
	}
	switch v[0] {
	case '\'':
		if end := strings.IndexByte(v[1:], '\''); end >= 0 {
			return v[1 : 1+end]
		}
		return v[1:]
	case '"':
		var sb strings.Builder
		for i := 1; i < len(v); i++ {
			c := v[i]
			if c == '"' {
				break
			}
			if c == '\\' && i+1 < len(v) && strings.IndexByte("\\\"$`", v[i+1]) >= 0 {
				i++
				c = v[i]
			}
			sb.WriteByte(c)
		}
		return sb.String()
	}
	// bare value: an inline comment starts at whitespace followed by '#'
	for i := 1; i < len(v); i++ {
		if v[i] == '#' && (v[i-1] == ' ' || v[i-1] == '\t') {
			return strings.TrimSpace(v[:i])
		}
	}
	return v
}

func formatValue(v string) string {
	if !strings.Contains(v, "'") {
		return "'" + v + "'"
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`, "`", "\\`")
	return `"` + r.Replace(v) + `"`
}

// index returns the line index of the last assignment to key, or -1.
func (f *File) index(key string) int {
	idx := -1
	for i, l := range f.lines {
		if k, _, ok := parseLine(l); ok && k == key {
			idx = i
		}
	}
	return idx
}

// Get returns the value of key.
func (f *File) Get(key string) (string, bool) {
	i := f.index(key)
	if i < 0 {
		return "", false
	}
	_, v, _ := parseLine(f.lines[i])
	return v, true
}

// Set replaces key's line in place, or appends a new one.
func (f *File) Set(key, value string) {
	line := key + "=" + formatValue(value)
	if i := f.index(key); i >= 0 {
		f.lines[i] = line
		return
	}
	f.lines = append(f.lines, line)
}

func descLine(line string) (string, bool) {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "#") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(t, "#")), true
}

// Desc returns the comment on the line directly above key, if any.
func (f *File) Desc(key string) string {
	i := f.index(key)
	if i <= 0 {
		return ""
	}
	d, _ := descLine(f.lines[i-1])
	return d
}

// SetDesc writes desc as the comment directly above key; unknown key is a no-op.
func (f *File) SetDesc(key, desc string) {
	i := f.index(key)
	if i < 0 {
		return
	}
	line := "# " + desc
	if i > 0 {
		if _, ok := descLine(f.lines[i-1]); ok {
			f.lines[i-1] = line
			return
		}
	}
	f.lines = append(f.lines[:i], append([]string{line}, f.lines[i:]...)...)
}

// Keys returns the distinct keys in file order.
func (f *File) Keys() []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range f.lines {
		if k, _, ok := parseLine(l); ok && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

// Values returns every key with its value.
func (f *File) Values() map[string]string {
	m := map[string]string{}
	for _, l := range f.lines {
		if k, v, ok := parseLine(l); ok {
			m[k] = v
		}
	}
	return m
}

// Save writes the file atomically (temp file + rename) with mode 0600.
func (f *File) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".env-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	content := ""
	if len(f.lines) > 0 {
		content = strings.Join(f.lines, "\n") + "\n"
	}
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

// Lookup resolves a key from the environment first (non-empty wins), then f.
func Lookup(getenv func(string) string, f *File) func(string) string {
	return func(key string) string {
		if v := getenv(key); v != "" {
			return v
		}
		if f != nil {
			if v, ok := f.Get(key); ok {
				return v
			}
		}
		return ""
	}
}

// ChildEnv returns base plus KEY=value for every key that has a value.
func ChildEnv(base []string, keys []string, lookup func(string) string) []string {
	out := append([]string{}, base...)
	for _, k := range keys {
		if v := lookup(k); v != "" {
			out = append(out, k+"="+v)
		}
	}
	return out
}
