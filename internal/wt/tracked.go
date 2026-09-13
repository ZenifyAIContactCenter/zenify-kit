package wt

import (
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

// dropTrackedInBase splits the copy list into entries to seed (keep) and
// entries that are already tracked in base (skipped). `git worktree add` has
// checked those out at the base version; copying them from the main checkout
// would overwrite that with whatever the main checkout happens to hold — a
// CLAUDE.md hundreds of commits stale, in one observed case. A git error
// (unknown ref, path not in tree) means "not tracked" and the entry is seeded
// as before, so an untracked or ignored file never loses its seed.
func dropTrackedInBase(r gitx.Runner, repoRoot, base string, copyList []string) (keep, skipped []string) {
	for _, f := range copyList {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if _, err := r.Run(repoRoot, "cat-file", "-e", base+":"+strings.TrimSuffix(f, "/")); err == nil {
			skipped = append(skipped, f)
			continue
		}
		keep = append(keep, f)
	}
	return keep, skipped
}
