package wt

import (
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// derivedDataRoot is Xcode's default DerivedData directory; "" when the home
// directory cannot be resolved. A var so tests can point it at a temp dir.
var derivedDataRoot = func() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "Developer", "Xcode", "DerivedData")
}

var workspacePathRe = regexp.MustCompile(`<key>WorkspacePath</key>\s*<string>([^<]*)</string>`)

// clearXcodeDerivedData deletes the DerivedData folders Xcode built for a
// workspace inside a removed worktree. Xcode keys each folder on a hash of the
// workspace path, so every worktree gets its own multi-GB copy that outlives
// the checkout. The folder's info.plist records that path; only folders whose
// WorkspacePath sits under one of paths (the worktree path, and its symlink-
// resolved form) are removed. Best-effort: no DerivedData dir (non-macOS, never
// built) is a no-op, and failures only warn.
func clearXcodeDerivedData(stderr io.Writer, paths ...string) {
	root := derivedDataRoot()
	if root == "" {
		return
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		b, err := os.ReadFile(filepath.Join(dir, "info.plist")) //nolint:gosec // G304 -- path is under Xcode's own DerivedData dir
		if err != nil {
			continue
		}
		m := workspacePathRe.FindSubmatch(b)
		if m == nil || !underAny(html.UnescapeString(string(m[1])), paths) {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			_, _ = fmt.Fprintf(stderr, "wt: warning — could not delete Xcode DerivedData %s: %v\n", dir, err)
			continue
		}
		_, _ = fmt.Fprintf(stderr, "wt: deleted Xcode DerivedData %s\n", e.Name())
	}
}

func underAny(p string, roots []string) bool {
	for _, r := range roots {
		if r != "" && (p == r || strings.HasPrefix(p, r+string(filepath.Separator))) {
			return true
		}
	}
	return false
}
