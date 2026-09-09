package docsview

import (
	"os"
	"path/filepath"
	"strings"
)

// FS wraps every OS-specific operation BEHIND this seam so EnsureView stays OS-agnostic.
// OSFS (Task 3) implements it: unix uses os.Symlink; windows uses a directory junction
// (mklink /J).
type FS interface {
	ReadDir(string) ([]os.DirEntry, error)
	MkdirAll(string, os.FileMode) error
	Remove(string) error
	// Link creates a link from link→target (symlink on unix / junction on windows).
	Link(target, link string) error
	// IsManagedLink: is path a link we manage (symlink/junction), NOT a real dir/file?
	IsManagedLink(path string) (bool, error)
	// SameTarget: does the link resolve to the expected target? (EvalSymlinks compare;
	// errors if the link is dead).
	SameTarget(link, target string) (bool, error)
}

// EnsureView guarantees viewDir contains only a link to each non-dot top-level DIR of
// store. Idempotent: creates what's missing, fixes a wrong target, prunes dead links.
// Does NOT touch a REAL dir/file inside viewDir (only operates on links it manages).
// Fail-open: an error becomes a note, never a panic.
func EnsureView(fs FS, store, viewDir string) []string {
	var notes []string
	entries, err := fs.ReadDir(store)
	if err != nil {
		return []string{"docs view: could not read store " + store + ": " + err.Error()}
	}
	if err := fs.MkdirAll(viewDir, 0o755); err != nil {
		return []string{"docs view: mkdir viewDir failed: " + err.Error()}
	}
	want := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		want[e.Name()] = true
		link := filepath.Join(viewDir, e.Name())
		target := filepath.Join(store, e.Name())
		if managed, _ := fs.IsManagedLink(link); managed {
			if same, _ := fs.SameTarget(link, target); same {
				continue // already correct
			}
			_ = fs.Remove(link) // wrong target → remove and recreate (removes only the link, safe with junctions)
		} else if _, err := fs.ReadDir(link); err == nil {
			// a REAL dir/file occupies the spot → do NOT clobber it, just note
			notes = append(notes, "docs view: "+link+" is a real dir/file (not a link) — skipping")
			continue
		}
		if err := fs.Link(target, link); err != nil {
			notes = append(notes, "docs view: creating link "+link+" failed: "+err.Error())
		}
	}
	if vents, err := fs.ReadDir(viewDir); err == nil {
		for _, v := range vents {
			if want[v.Name()] {
				continue
			}
			link := filepath.Join(viewDir, v.Name())
			if managed, _ := fs.IsManagedLink(link); managed {
				_ = fs.Remove(link) // NOT RemoveAll: only removes the link, never the store contents
				notes = append(notes, "docs view: pruned dead link "+v.Name())
			}
		}
	}
	return notes
}
