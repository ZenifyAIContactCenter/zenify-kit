package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/managed"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/reconcile"
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
	if fi, err := os.Stat(filepath.Join(p, ".zenify", "manifest.json")); err != nil || fi.IsDir() { //nolint:gosec // G703 -- p comes from the kit-owned pointer file under ~/.zenify, validated absolute above, and is only probed for the workspace marker
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

// workspaceSettingsPath is <workspace>/.claude/settings.local.json for the
// workspace resolved from the current cwd, or "" when there is none (FR-2.3).
func workspaceSettingsPath() string {
	cwd, _ := os.Getwd()
	ws, _, ok := resolveWorkspace(cwd, "", os.Getenv, os.UserHomeDir)
	if !ok {
		return ""
	}
	return filepath.Join(ws, ".claude", "settings.local.json")
}

// workspaceOrCwd resolves the workspace for commands that used to default to
// cwd (docs sync, config). Falls back to cwd with one warning so an
// un-migrated machine keeps working (FR-2.4).
func workspaceOrCwd(flag string, errW io.Writer) string {
	cwd, _ := os.Getwd()
	if ws, _, ok := resolveWorkspace(cwd, flag, os.Getenv, os.UserHomeDir); ok {
		return ws
	}
	fmt.Fprintln(errW, "zenify: chưa có workspace (không thấy .zenify/manifest.json hay ~/.zenify/workspace) — dùng thư mục hiện tại") //znf:allow-lang
	return cwd
}

// osDefaultWorkspace is where the wizard proposes to put a new workspace
// (FR-3.3): ~/Developer/zenify on macOS (Finder gives ~/Developer its own
// icon), ~/zenify on Linux, %USERPROFILE%\zenify on Windows.
func osDefaultWorkspace(goos, home string) string {
	if goos == "darwin" {
		return filepath.Join(home, "Developer", "zenify")
	}
	return filepath.Join(home, "zenify")
}

// kitOwnedEntries are the only names a directory may contain and still count
// as an empty workspace (a half-finished earlier `up`, or a docs view).
var kitOwnedEntries = map[string]bool{".zenify": true, ".zenify-overlay.yaml": true, ".claude": true, "docs": true, "repos": true}

// validateWorkspaceDir applies FR-3.4: not the home dir, not inside a git
// repo, and — when it exists — empty apart from kit-owned entries. gitTop is
// `git -C dir rev-parse --show-toplevel` (error ⇒ not in a repo), injected.
func validateWorkspaceDir(dir, home string, gitTop func(dir string) (string, error)) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if filepath.Clean(abs) == filepath.Clean(home) {
		return errors.New("không dùng home dir làm workspace — chọn một thư mục con") //znf:allow-lang
	}
	probe := abs
	for {
		if _, err := os.Stat(probe); err == nil {
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			break
		}
		probe = parent
	}
	if top, err := gitTop(probe); err == nil && top != "" {
		return fmt.Errorf("nằm trong git repo %s — workspace phải ở ngoài mọi repo", top) //znf:allow-lang
	}
	entries, err := os.ReadDir(abs)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !kitOwnedEntries[e.Name()] {
			return fmt.Errorf("thư mục không rỗng (có %s) — chọn thư mục rỗng hoặc mới", e.Name()) //znf:allow-lang
		}
	}
	return nil
}

// gitToplevel is the real gitTop for validateWorkspaceDir.
func gitToplevel(dir string) (string, error) {
	b, err := gitx.ExecRunner().Run(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// whereNeeded reports whether the Where step (tui.RunWhere) must run (FR-3.1):
// in the wizard, whenever cwd resolved to no workspace at all, OR resolved
// only via the pointer — the pointer is a soft "last workspace used" default,
// not a confirmed choice, so it still needs the "use existing / create here"
// prompt (FR-3.2/SC-2). A marker or an explicit --workspace flag is already a
// confirmed answer and skips the step; headless (wantWizard=false) never asks.
func whereNeeded(wantWizard bool, src wsSource, ok bool) bool {
	return wantWizard && (!ok || src == wsFromPointer)
}

// expandHome expands a leading "~" in p to home (SC-5: a literal "~/projects"
// typed into the Where prompts must resolve, since the shell never expands it
// for a TUI input). "~" alone becomes home; "~/rest" (or the OS separator
// variant) becomes home joined with rest; anything else — including "~user"
// and an empty string — passes through unchanged.
func expandHome(p, home string) string {
	if p == "" {
		return p
	}
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	if strings.HasPrefix(p, "~"+string(os.PathSeparator)) {
		return filepath.Join(home, p[2:])
	}
	return p
}

// dropInPlaceSources removes any source whose path is already at the
// manifest's own destination for that repo — a source dir that happens to
// contain the workspace itself (e.g. sources dir ~/Developer, workspace
// ~/Developer/zenify) would otherwise resolve to <ws>/repos/<name>, which
// reconcile.Build then classifies SKIP instead of the real WIRE/ADOPT.
func dropInPlaceSources(m *manifest.Manifest, workspace string, sources map[string]reconcile.Source) map[string]reconcile.Source {
	out := make(map[string]reconcile.Source, len(sources))
	for name, s := range sources {
		r, ok := m.ByName(name)
		if !ok {
			out[name] = s
			continue
		}
		dest, err1 := filepath.Abs(filepath.Join(workspace, r.Path))
		src, err2 := filepath.Abs(s.Path)
		if err1 == nil && err2 == nil && filepath.Clean(dest) == filepath.Clean(src) {
			continue
		}
		out[name] = s
	}
	return out
}
