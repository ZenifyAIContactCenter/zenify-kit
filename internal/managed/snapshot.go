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
	if err := os.MkdirAll(snapDir, 0o755); err != nil {
		return "", err
	}
	origin := map[string]string{} // snapshotFile -> originalPath
	for i, f := range files {
		b, err := os.ReadFile(f)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		name := filepath.Base(f) + "." + itoa(i)
		if err := os.WriteFile(filepath.Join(snapDir, name), b, 0o644); err != nil {
			return "", err
		}
		origin[name] = f
	}
	idx, err := json.MarshalIndent(origin, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(snapDir, snapIndexName), idx, 0o644); err != nil {
		return "", err
	}
	return snapDir, nil
}

// Restore copies every file in the snapshot back to its origin path.
func Restore(snapDir string) error {
	b, err := os.ReadFile(filepath.Join(snapDir, snapIndexName))
	if err != nil {
		return err
	}
	var origin map[string]string
	if err := json.Unmarshal(b, &origin); err != nil {
		return err
	}
	for name, orig := range origin {
		data, err := os.ReadFile(filepath.Join(snapDir, name))
		if err != nil {
			return err
		}
		if err := os.WriteFile(orig, data, 0o644); err != nil {
			return err
		}
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
