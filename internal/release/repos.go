package release

import (
	"path/filepath"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	wspkg "github.com/ZenifyAIContactCenter/zenify-kit/internal/workspace"
)

// Resolve returns the list of release-tracked repos. If <workspace>/.znf/release-repos.txt can
// be read → use its non-empty lines. Otherwise auto-detect over `discovered` (the result of
// workspace.Discover) — any repo that has origin/release<n>. readFile is injected for testing
// (the CLI passes os.ReadFile).
func Resolve(r gitx.Runner, workspace string, n int,
	readFile func(string) ([]byte, error), discovered []wspkg.Repo) ([]string, error) {

	if pin, ok := pinList(workspace, readFile); ok {
		return pin, nil
	}
	var repos []string
	for _, rp := range discovered {
		nums, err := ReleaseNums(r, rp.Path)
		if err != nil {
			continue
		}
		for _, x := range nums {
			if x == n {
				repos = append(repos, rp.Name)
				break
			}
		}
	}
	return repos, nil
}

// ResolveUnreleased returns the repos for the UNRELEASED VIEW. It still honors the
// `.znf/release-repos.txt` pin (if present). Its auto-detect DIFFERS from Resolve: it takes EVERY
// repo with AT LEAST one release branch (not requiring release<n>) — because unreleased is the
// "daily pending deploy" of every deployed repo, regardless of whether that repo cut a release
// this week. A repo with staging commits not yet deployed shows up; a repo with nothing pending
// is dropped by buildReport (empty section).
func ResolveUnreleased(r gitx.Runner, workspace string,
	readFile func(string) ([]byte, error), discovered []wspkg.Repo) ([]string, error) {

	if pin, ok := pinList(workspace, readFile); ok {
		return pin, nil
	}
	var repos []string
	for _, rp := range discovered {
		if nums, err := ReleaseNums(r, rp.Path); err == nil && len(nums) > 0 {
			repos = append(repos, rp.Name)
		}
	}
	return repos, nil
}

// pinList reads `.znf/release-repos.txt` (one repo per line, `#`=comment). ok=false if the file
// cannot be read (→ caller auto-detects). Warning: the file EXISTS but is comments-only → ok=true
// with an EMPTY list (scans 0 repos) — this is intentional: an empty pin means "no repos"; to get
// auto-detect back, DELETE the file.
func pinList(workspace string, readFile func(string) ([]byte, error)) ([]string, bool) {
	b, err := readFile(filepath.Join(workspace, ".znf", "release-repos.txt"))
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
