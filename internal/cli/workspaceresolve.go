package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
)

// wsSource says which rule resolved the workspace (for messages and tests).
type wsSource string

const (
	wsFromFlag    wsSource = "flag"
	wsFromMarker  wsSource = "marker"
	wsFromPointer wsSource = "pointer"
)

// workspacePointerName is the one-line file under zenifyHome holding the
// absolute path of the workspace `zenify up` last applied (FR-2.2).
const workspacePointerName = "workspace"

// zenifyHome is $ZENIFY_HOME when set, else <home>/.zenify — the same
// convention resolveDocsStore uses for the knowledge store.
func zenifyHome(getenv func(string) string, userHome func() (string, error)) string {
	if zh := getenv("ZENIFY_HOME"); zh != "" {
		return zh
	}
	home, _ := userHome()
	return filepath.Join(home, ".zenify")
}

// resolveWorkspace finds the workspace for any command run from any cwd (FR-2.1):
//  1. an explicit --workspace flag (anything but "" or ".") is used as given;
//  2. the .zenify/manifest.json marker, walking up from cwd (findWorkspaceRoot);
//  3. the pointer file <zenifyHome>/workspace, if it names a dir carrying the marker.
//
// ok=false means "no workspace" — the caller decides (wizard asks, headless errors).
func resolveWorkspace(cwd, flag string, getenv func(string) string, userHome func() (string, error)) (string, wsSource, bool) {
	if flag != "" && flag != "." {
		if abs, err := filepath.Abs(flag); err == nil {
			return abs, wsFromFlag, true
		}
		return flag, wsFromFlag, true
	}
	if root, ok := findWorkspaceRoot(cwd); ok {
		return root, wsFromMarker, true
	}
	if p, ok := readWorkspacePointer(getenv, userHome); ok {
		return p, wsFromPointer, true
	}
	return "", "", false
}

// readWorkspacePointer returns the pointer's path when the file exists, holds
// an absolute path, and that path still carries the workspace marker.
func readWorkspacePointer(getenv func(string) string, userHome func() (string, error)) (string, bool) {
	b, err := os.ReadFile(filepath.Join(zenifyHome(getenv, userHome), workspacePointerName)) //nolint:gosec // G304 -- path is computed internally from the kit's own home, not externally-tainted input
	if err != nil {
		return "", false
	}
	p := strings.TrimSpace(string(b))
	if p == "" || !filepath.IsAbs(p) {
		return "", false
	}
	if fi, err := os.Stat(filepath.Join(p, ".zenify", "manifest.json")); err != nil || fi.IsDir() {
		return "", false
	}
	return p, true
}

// writeWorkspacePointer records ws as the current workspace (FR-2.2): mkdir
// zenifyHome, then an atomic one-line write. Mode 0600 comes from WriteFileAtomic.
func writeWorkspacePointer(getenv func(string) string, userHome func() (string, error), ws string) error {
	abs, err := filepath.Abs(ws)
	if err != nil {
		return err
	}
	dir := zenifyHome(getenv, userHome)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	p := filepath.Join(dir, workspacePointerName)
	if err := managed.WriteFileAtomic(p, []byte(abs+"\n")); err != nil {
		return err
	}
	// WriteFileAtomic keeps an existing file's mode and defaults a NEW file to
	// 0o644 (snapshot.go:111) — the pointer is per-user state, so pin 0o600.
	return os.Chmod(p, 0o600)
}
