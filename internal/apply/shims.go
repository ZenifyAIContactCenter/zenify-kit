// internal/apply/shims.go
package apply

import (
	"os"
	"path/filepath"
	"strings"
)

// shimMarker flags a file as written by `zenify up`; only marked files are
// ever overwritten or removed.
const shimMarker = "# managed by zenify"

var shimNames = []string{"wt", "pm"}

func shimBody(name string) string {
	return "#!/bin/sh\n" + shimMarker + "\nexec zenify " + name + " \"$@\"\n"
}

func shimDir(home string) string { return filepath.Join(home, ".local", "bin") }

func isManagedShim(path string) (managed, exists bool) {
	b, err := os.ReadFile(path) //nolint:gosec // G304 -- fixed path under the user's home
	if err != nil {
		return false, false
	}
	return strings.Contains(string(b), shimMarker), true
}

// InstallShims writes ~/.local/bin/wt and ~/.local/bin/pm (mode 0755), each
// delegating to `zenify <name>`. A file without the marker is the user's own:
// it is left untouched and reported in skipped. Files already up to date are
// not reported. dryRun reports what would be written without touching disk.
func InstallShims(home string, dryRun bool) (written, skipped []string, err error) {
	dir := shimDir(home)
	for _, n := range shimNames {
		p := filepath.Join(dir, n)
		want := shimBody(n)
		managed, exists := isManagedShim(p)
		if exists && !managed {
			skipped = append(skipped, p)
			continue
		}
		if exists {
			if b, rerr := os.ReadFile(p); rerr == nil && string(b) == want { //nolint:gosec // G304 -- fixed path under the user's home
				if fi, serr := os.Stat(p); serr == nil && fi.Mode().Perm() == 0o755 {
					continue
				}
			}
		}
		if !dryRun {
			if err = os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G301 -- ~/.local/bin is conventionally 0755
				return written, skipped, err
			}
			if err = os.WriteFile(p, []byte(want), 0o755); err != nil { //nolint:gosec // G306 -- shims must be executable
				return written, skipped, err
			}
			if err = os.Chmod(p, 0o755); err != nil { //nolint:gosec // G302 -- shims must be executable
				return written, skipped, err
			}
		}
		written = append(written, p)
	}
	return written, skipped, nil
}

// RemoveShims deletes the shims InstallShims wrote; files without the marker
// stay. dryRun reports without removing.
func RemoveShims(home string, dryRun bool) (removed []string, err error) {
	for _, n := range shimNames {
		p := filepath.Join(shimDir(home), n)
		if managed, _ := isManagedShim(p); !managed {
			continue
		}
		if !dryRun {
			if err = os.Remove(p); err != nil {
				return removed, err
			}
		}
		removed = append(removed, p)
	}
	return removed, nil
}
