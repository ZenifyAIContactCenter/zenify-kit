package apply

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const codexTrustLine = `trust_level = "trusted"`

// codexTable locates the `[projects."<ws>"]` table in lines: header index and
// the index of the next table header (or len(lines)). header is -1 if absent.
func codexTable(lines []string, workspace string) (header, end int) {
	want := `[projects."` + workspace + `"]`
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
		if strings.HasPrefix(lines[i], "[") {
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
	b, err := os.ReadFile(path)
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
		out += `[projects."` + workspace + `"]` + "\n" + codexTrustLine + "\n"
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
// trust_level (what EnsureCodexTrust creates), plus the blank line Ensure put
// before it. A table with other keys is left alone.
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
		if t := strings.TrimSpace(lines[i]); t != "" && !isTrustKey(lines[i]) {
			return false, nil
		}
	}
	start := header
	if start > 0 && strings.TrimSpace(lines[start-1]) == "" {
		start--
	}
	if end == len(lines) && lines[end-1] == "" {
		end-- // keep the file's final newline
	}
	lines = append(lines[:start], lines[end:]...)
	if dryRun {
		return true, nil
	}
	return true, writeAtomic(path, []byte(strings.Join(lines, "\n")), 0o600)
}
