package apply

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const codexTrustLine = `trust_level = "trusted"`

// codexTableHeader is the `[projects."<ws>"]` header line, the form Codex
// itself writes, with the path as a TOML basic string.
func codexTableHeader(workspace string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `[projects."` + r.Replace(workspace) + `"]`
}

// isTableHeader reports whether l is a TOML table header (`[a]`, `[[a]]`,
// optionally followed by a comment) starting in column 0, as Codex writes
// them. The closing bracket is found outside quoted keys, so a `]` inside a
// key or a trailing comment does not count. An array element (`[1, 2]`,
// `  [1, 2],`) is not a header: a bare `,` or `[` cannot appear in a key.
func isTableHeader(l string) bool {
	t := strings.TrimRight(l, " \t\r")
	if !strings.HasPrefix(t, "[") {
		return false
	}
	double := strings.HasPrefix(t, "[[")
	i := 1
	if double {
		i = 2
	}
	var quote byte
	for ; i < len(t); i++ {
		c := t[i]
		switch {
		case quote == '"' && c == '\\':
			i++ // skip the escaped character
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == ',' || c == '[' || c == '#':
			return false
		case c == ']':
			end := i + 1
			if double {
				if end >= len(t) || t[end] != ']' {
					return false
				}
				end++
			}
			rest := strings.TrimSpace(t[end:])
			return rest == "" || strings.HasPrefix(rest, "#")
		}
	}
	return false
}

// codexTable locates the `[projects."<ws>"]` table in lines: header index and
// the index of the next table header (or len(lines)). header is -1 if absent.
// The dotted (`[projects]` + `"<ws>".trust_level`) and inline forms are not
// recognised; Codex writes the header form.
func codexTable(lines []string, workspace string) (header, end int) {
	want := codexTableHeader(workspace)
	header = -1
	for i, l := range lines {
		if strings.TrimSpace(l) == want {
			header = i
			break
		}
	}
	if header < 0 {
		return -1, 0
	}
	end = len(lines)
	for i := header + 1; i < len(lines); i++ {
		if isTableHeader(lines[i]) {
			end = i
			break
		}
	}
	return header, end
}

func isTrustKey(l string) bool {
	k, _, ok := strings.Cut(strings.TrimSpace(l), "=")
	return ok && strings.TrimSpace(k) == "trust_level"
}

func readCodexConfig(codexHome string) (string, string, error) {
	path := filepath.Join(codexHome, "config.toml")
	b, err := os.ReadFile(path) //nolint:gosec // G304 -- the user's own Codex config under CODEX_HOME
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return path, "", err
	}
	return path, string(b), nil
}

// EnsureCodexTrust marks workspace as trusted in <codexHome>/config.toml by
// editing the text in place (no decode/encode, so comments and other tables
// survive byte for byte). Reports whether the file changed (or would, dryRun).
func EnsureCodexTrust(codexHome, workspace string, dryRun bool) (bool, error) {
	path, text, err := readCodexConfig(codexHome)
	if err != nil {
		return false, err
	}
	lines := strings.Split(text, "\n")
	header, end := codexTable(lines, workspace)
	var out string
	if header < 0 {
		out = text
		if out != "" {
			if !strings.HasSuffix(out, "\n") {
				out += "\n"
			}
			out += "\n"
		}
		out += codexTableHeader(workspace) + "\n" + codexTrustLine + "\n"
	} else {
		idx := -1
		for i := header + 1; i < end; i++ {
			if isTrustKey(lines[i]) {
				idx = i
				break
			}
		}
		switch {
		case idx >= 0 && strings.TrimSpace(lines[idx]) == codexTrustLine:
			return false, nil
		case idx >= 0:
			lines[idx] = codexTrustLine
		default:
			lines = append(lines[:header+1], append([]string{codexTrustLine}, lines[header+1:]...)...)
		}
		out = strings.Join(lines, "\n")
	}
	if dryRun {
		return true, nil
	}
	return true, writeAtomic(path, []byte(out), 0o600)
}

// RemoveCodexTrust removes the workspace table only when it holds nothing but
// `trust_level = "trusted"` (what EnsureCodexTrust creates), plus the blank
// line Ensure put before it. A table with other keys, or one the user set to
// another trust level, is left alone.
func RemoveCodexTrust(codexHome, workspace string, dryRun bool) (bool, error) {
	path, text, err := readCodexConfig(codexHome)
	if err != nil {
		return false, err
	}
	lines := strings.Split(text, "\n")
	header, end := codexTable(lines, workspace)
	if header < 0 {
		return false, nil
	}
	for i := header + 1; i < end; i++ {
		t := strings.TrimSpace(lines[i])
		if t != "" && (!isTrustKey(lines[i]) || t != codexTrustLine) {
			return false, nil
		}
	}
	start := header
	if start > 0 && strings.TrimSpace(lines[start-1]) == "" {
		start--
	}
	switch {
	case end == len(lines) && lines[end-1] == "":
		end-- // keep the file's final newline
	case end < len(lines) && start < header && end-1 > header && strings.TrimSpace(lines[end-1]) == "":
		end-- // mid-file: drop one blank separator, keep the one before the next table
	}
	lines = append(lines[:start], lines[end:]...)
	if dryRun {
		return true, nil
	}
	return true, writeAtomic(path, []byte(strings.Join(lines, "\n")), 0o600)
}
