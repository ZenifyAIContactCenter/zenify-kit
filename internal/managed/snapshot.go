package managed

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// manifestName holds the origin map inside a snapshot dir.
const snapIndexName = ".origin.json"

// Snapshot copies each existing file into snapRoot/<id>/ and records its origin
// path, so Restore can put it back. Missing files are skipped, not errors.
func Snapshot(id string, files []string, snapRoot string) (string, error) {
	snapDir := filepath.Join(snapRoot, id)
	if err := os.MkdirAll(snapDir, 0o750); err != nil {
		return "", err
	}
	origin := map[string]string{} // snapshotFile -> originalPath
	for i, f := range files {
		b, err := os.ReadFile(f) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		name := filepath.Base(f) + "." + itoa(i)
		if err := os.WriteFile(filepath.Join(snapDir, name), b, 0o600); err != nil { //nolint:gosec // G703 -- snapDir is this tool's own snapshot directory; name is derived from filepath.Base of an internally-configured target, not externally-tainted input
			return "", err
		}
		origin[name] = f
	}
	idx, err := json.MarshalIndent(origin, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(snapDir, snapIndexName), idx, 0o600); err != nil {
		return "", err
	}
	return snapDir, nil
}

// Restore copies every file in the snapshot back to its origin path,
// all-or-nothing: it loads every snapshot file into memory first (so a bad
// read aborts before any write), writes each destination via a same-directory
// temp file + rename, and rolls back everything already written if a later
// write fails. On success the tree matches the snapshot; on failure it is left
// exactly as Restore found it.
func Restore(snapDir string) error {
	b, err := os.ReadFile(filepath.Join(snapDir, snapIndexName)) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
	if err != nil {
		return err
	}
	var origin map[string]string
	if err := json.Unmarshal(b, &origin); err != nil {
		return err
	}

	// Phase 1: load every snapshot payload. Any read failure aborts before any write.
	payload := make(map[string][]byte, len(origin)) // originPath -> bytes to restore
	for name, orig := range origin {
		data, err := os.ReadFile(filepath.Join(snapDir, name)) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
		if err != nil {
			return err
		}
		payload[orig] = data
	}

	// Phase 2: write each destination, capturing its prior state for rollback.
	type undo struct {
		path    string
		data    []byte
		existed bool
	}
	var done []undo
	rollback := func() {
		for i := len(done) - 1; i >= 0; i-- {
			u := done[i]
			if u.existed {
				_ = WriteFileAtomic(u.path, u.data)
			} else {
				_ = os.Remove(u.path)
			}
		}
	}
	for orig, data := range payload {
		prior, rerr := os.ReadFile(orig) //nolint:gosec // G304 -- path is computed internally by this tool from its own config/workspace state, not externally-tainted input
		existed := rerr == nil
		if rerr != nil && !os.IsNotExist(rerr) {
			rollback()
			return rerr
		}
		if err := WriteFileAtomic(orig, data); err != nil {
			rollback()
			return err
		}
		done = append(done, undo{path: orig, data: prior, existed: existed})
	}
	return nil
}

// WriteFileAtomic writes data to path by creating a temp file in the same
// directory and renaming it over path, so a reader never sees a partial file.
// It preserves the destination's existing file mode (os.CreateTemp defaults to
// 0600, which would otherwise silently narrow permissions on every write);
// if the destination does not yet exist, it falls back to 0o644.
func WriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)

	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".zenify-restore-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

// itoa avoids importing strconv for a single small use.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
