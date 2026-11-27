package infra

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// buildctlPath is where the pinned buildctl client lives.
func buildctlPath(home string) string { return filepath.Join(infraDir(home), "bin", "buildctl") }

// maxBuildctlSize caps the extracted binary (v0.34.2 buildctl is ~25MB).
const maxBuildctlSize = 200 << 20

// EnsureBuildctl installs buildctl BuildkitVersion into ~/.zenify/infra/bin once.
// The release ships no checksum file, so the tarball is checked against the
// sha256 digest GitHub records for the release asset.
func EnsureBuildctl(ctx context.Context, r Runner, home string) (string, error) {
	dst := buildctlPath(home)
	if _, err := os.Stat(dst); err == nil {
		return dst, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("buildctl: stat %s: %w", dst, err)
	}
	asset := fmt.Sprintf("buildkit-%s.linux-%s.tar.gz", BuildkitVersion, runtime.GOARCH)
	out, err := r.Run(ctx, nil, "gh", "api", "repos/moby/buildkit/releases/tags/"+BuildkitVersion)
	if err != nil {
		return "", fmt.Errorf("buildctl: read release %s: %w", BuildkitVersion, err)
	}
	var rel struct {
		Assets []struct {
			Name   string `json:"name"`
			Digest string `json:"digest"`
			URL    string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(out, &rel); err != nil {
		return "", fmt.Errorf("buildctl: parse release %s: %w", BuildkitVersion, err)
	}
	var digest, url string
	for _, a := range rel.Assets {
		if a.Name == asset {
			digest, url = a.Digest, a.URL
		}
	}
	if digest == "" || url == "" {
		return "", fmt.Errorf("buildctl: release %s has no asset %s with a digest", BuildkitVersion, asset)
	}

	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("buildctl: mkdir %s: %w", dir, err)
	}
	tgz, err := os.CreateTemp(dir, asset+".*")
	if err != nil {
		return "", fmt.Errorf("buildctl: temp file: %w", err)
	}
	tgzPath := tgz.Name()
	defer func() { _ = os.Remove(tgzPath) }() // download scratch; the binary is renamed out before this runs
	if err := tgz.Close(); err != nil {
		return "", fmt.Errorf("buildctl: close %s: %w", tgzPath, err)
	}
	if _, err := r.Run(ctx, nil, "curl", "-fsSL", "-o", tgzPath, url); err != nil {
		return "", fmt.Errorf("buildctl: download %s: %w", url, err)
	}
	if err := checkSHA256(tgzPath, digest); err != nil {
		return "", fmt.Errorf("buildctl: %s: %w", asset, err)
	}
	if err := extractBuildctl(tgzPath, dst); err != nil {
		return "", fmt.Errorf("buildctl: %w", err)
	}
	return dst, nil
}

func checkSHA256(p, digest string) error {
	f, err := os.Open(p) //nolint:gosec // G304 -- our own temp file
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }() // read-only
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash: %w", err)
	}
	if got := "sha256:" + hex.EncodeToString(h.Sum(nil)); got != digest {
		return fmt.Errorf("checksum mismatch: got %s, release says %s", got, digest)
	}
	return nil
}

// extractBuildctl writes bin/buildctl from the tarball to dst (0755) via temp + rename.
func extractBuildctl(tgzPath, dst string) error {
	f, err := os.Open(tgzPath) //nolint:gosec // G304 -- our own temp file
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }() // read-only
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gunzip: %w", err)
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return errors.New("tarball has no bin/buildctl")
		}
		if err != nil {
			return fmt.Errorf("read tarball: %w", err)
		}
		if h.Name != "bin/buildctl" {
			continue
		}
		tmp, err := os.CreateTemp(filepath.Dir(dst), "buildctl.*.tmp")
		if err != nil {
			return fmt.Errorf("temp file: %w", err)
		}
		_, cpErr := io.Copy(tmp, io.LimitReader(tr, maxBuildctlSize))
		closeErr := tmp.Close()
		if err := errors.Join(cpErr, closeErr); err != nil {
			_ = os.Remove(tmp.Name())
			return fmt.Errorf("write %s: %w", tmp.Name(), err)
		}
		if err := os.Chmod(tmp.Name(), 0o755); err != nil { //nolint:gosec // G302 -- an executable
			_ = os.Remove(tmp.Name())
			return fmt.Errorf("chmod %s: %w", tmp.Name(), err)
		}
		if err := os.Rename(tmp.Name(), dst); err != nil {
			_ = os.Remove(tmp.Name())
			return fmt.Errorf("rename to %s: %w", dst, err)
		}
		return nil
	}
}
