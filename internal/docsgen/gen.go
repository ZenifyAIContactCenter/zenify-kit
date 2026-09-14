// Package docsgen renders the kit's reference documentation (CLI, skills,
// agents, hooks) as VitePress-ready markdown from the code itself, so the
// site can never drift from the binary (spec FR-2).
package docsgen

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Files maps a path relative to the output dir to its rendered content.
type Files map[string][]byte

// Write materialises files under dir and removes stale *.md files inside the
// sub-directories it owns (cli/, skills/, agents/) so a deleted command
// cannot leave an orphan page behind.
func Write(dir string, files Files) error {
	owned := ownedDirs(files)
	for sub := range owned {
		entries, err := os.ReadDir(filepath.Join(dir, sub))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		for _, e := range entries {
			rel := filepath.ToSlash(filepath.Join(sub, e.Name()))
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				if _, keep := files[rel]; !keep {
					if err := os.Remove(filepath.Join(dir, rel)); err != nil {
						return err
					}
				}
			}
		}
	}
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(p, content, 0o644); err != nil { //nolint:gosec // G306 -- generated docs, world-readable by design
			return err
		}
	}
	return nil
}

// Check compares files against what is on disk under dir. It returns the
// relative paths that differ, are missing, or exist on disk inside an owned
// sub-directory without being generated. Empty result = in sync.
func Check(dir string, files Files) []string {
	var out []string
	for rel, want := range files {
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel))) //nolint:gosec // G304 -- rel comes from the generator's own file map, joined under --out
		if err != nil || !bytes.Equal(got, want) {
			out = append(out, rel)
		}
	}
	for sub := range ownedDirs(files) {
		entries, _ := os.ReadDir(filepath.Join(dir, sub))
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			rel := filepath.ToSlash(filepath.Join(sub, e.Name()))
			if _, ok := files[rel]; !ok {
				out = append(out, rel)
			}
		}
	}
	sort.Strings(out)
	return out
}

func ownedDirs(files Files) map[string]struct{} {
	out := map[string]struct{}{}
	for rel := range files {
		if d := filepath.ToSlash(filepath.Dir(rel)); d != "." {
			out[d] = struct{}{}
		}
	}
	return out
}
