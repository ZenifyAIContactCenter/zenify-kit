package gitx

import (
	"os"
	"path/filepath"
	"strings"
)

// EnsureExclude appends each of lines to <repoDir>/.git/info/exclude when it is
// not already present. Returns the exclude path if the file was modified, ""
// if every line was already there. Creates .git/info when missing so a fresh
// clone is covered. Shared by apply (wire/adopt) and wt (first .wt/ write).
func EnsureExclude(repoDir string, lines ...string) (string, error) {
	infoDir := filepath.Join(repoDir, ".git", "info")
	if err := os.MkdirAll(infoDir, 0o750); err != nil {
		return "", err
	}
	excludePath := filepath.Join(infoDir, "exclude")
	b, err := os.ReadFile(excludePath) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	present := map[string]bool{}
	for _, ln := range strings.Split(string(b), "\n") {
		present[strings.TrimSpace(ln)] = true
	}
	content := string(b)
	added := false
	for _, want := range lines {
		if present[want] {
			continue
		}
		if content != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += want + "\n"
		present[want] = true
		added = true
	}
	if !added {
		return "", nil
	}
	if err := os.WriteFile(excludePath, []byte(content), 0o600); err != nil { //nolint:gosec // G703 -- excludePath is the repo's own .git/info/exclude, computed internally, not externally-tainted input
		return "", err
	}
	return excludePath, nil
}
