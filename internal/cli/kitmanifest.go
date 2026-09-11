package cli

import (
	"os"
	"path/filepath"

	kit "github.com/ZenifyAIContactCenter/zenify-kit"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
)

// defaultManifestRel is where the kit checkout keeps its manifest; used only
// when the caller passes no --manifest and the file exists under cwd.
var defaultManifestRel = filepath.Join("manifest", "repos.yaml")

// loadKitManifest resolves the repos manifest for `up` and `down` (FR-05):
//  1. explicit != "" → that file, errors surface (no fallback);
//  2. a regular file manifest/repos.yaml under cwd → that file (kit dev loop);
//  3. otherwise the embedded default compiled into the binary.
//
// The second return value names the source ("embedded" or the path) so the
// caller can print where the plan came from.
func loadKitManifest(explicit, overlayPath string) (*manifest.Manifest, string, error) {
	if explicit != "" {
		m, err := manifest.LoadWithOverlay(explicit, overlayPath)
		return m, explicit, err
	}
	if fi, err := os.Stat(defaultManifestRel); err == nil && fi.Mode().IsRegular() {
		m, err := manifest.LoadWithOverlay(defaultManifestRel, overlayPath)
		return m, defaultManifestRel, err
	}
	m, err := manifest.ParseWithOverlay(kit.DefaultRepos, "embedded", overlayPath)
	return m, "embedded", err
}
