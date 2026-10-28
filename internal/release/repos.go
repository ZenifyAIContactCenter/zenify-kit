package release

import (
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	wspkg "github.com/ZenifyAIContactCenter/zenify-kit/internal/workspace"
)

// Resolve returns the list of release-tracked repos. If <workspace>/.zenify/release-repos.txt (legacy
// fallback: .znf/release-repos.txt) can be read → use its non-empty lines. Otherwise auto-detect over `discovered` (the result of
// workspace.Discover) — any repo that carries a release tag vYYMMDD.n. readFile is injected for testing
// (the CLI passes os.ReadFile).
func Resolve(r gitx.Runner, workspace string,
	readFile func(string) ([]byte, error), discovered []wspkg.Repo) ([]string, error) {

	if pin, ok := pinList(workspace, readFile); ok {
		return pin, nil
	}
	var repos []string
	for _, rp := range discovered {
		if tags, err := releaseTags(r, rp.Path); err == nil && len(tags) > 0 {
			repos = append(repos, rp.Name)
		}
	}
	return repos, nil
}

// pinList reads `.zenify/release-repos.txt`, falling back to the legacy `.znf/release-repos.txt`
// (one repo per line, `#`=comment). ok=false if neither file can be read (→ caller auto-detects). Warning: the file EXISTS but is comments-only → ok=true
// with an EMPTY list (scans 0 repos) — this is intentional: an empty pin means "no repos"; to get
// auto-detect back, DELETE the file.
func pinList(workspace string, readFile func(string) ([]byte, error)) ([]string, bool) {
	b, err := readFile(filepath.Join(workspace, ".zenify", "release-repos.txt"))
	if err != nil {
		b, err = readFile(filepath.Join(workspace, ".znf", "release-repos.txt"))
	}
	if err != nil {
		return nil, false
	}
	var repos []string
	for _, l := range strings.Split(string(b), "\n") {
		s := strings.TrimSpace(l)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		repos = append(repos, s)
	}
	return repos, true
}
