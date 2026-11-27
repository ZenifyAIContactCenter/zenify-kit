package infra

import (
	"fmt"
	"os"
	"path/filepath"
)

func infraDir(home string) string { return filepath.Join(home, ".zenify", "infra") }

// writeFileAtomic writes data to <home>/.zenify/infra/<name> (0600) via a temp
// file in the same directory + rename, so readers never see a partial file.
func writeFileAtomic(home, name string, data []byte) error {
	dir := infraDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("infra: mkdir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, name+".*.tmp")
	if err != nil {
		return fmt.Errorf("infra: temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("infra: write %s: %w", tmpName, err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("infra: chmod %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("infra: close %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, filepath.Join(dir, name)); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("infra: rename to %s: %w", name, err)
	}
	return nil
}
