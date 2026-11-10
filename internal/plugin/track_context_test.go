package plugin

import "testing"

// The znf hook (hooks/register.tsx trackContextFor) injects the `## Rules` block and the block named
// after the loaded skill from this file; a renamed heading would silently drop the track steps.
func TestTrackMdHasSectionsTheHookInjects(t *testing.T) {
	assertContainsAll(t, "_shared/track.md", syncedAsset(t, "skills/_shared/track.md"), []string{
		"\n## Rules\n",
		"\n## cook\n",
		"\n## fix\n",
		"\n## hotfix\n",
		"\n## ship\n",
		"\n## ci-watch\n",
		"\n## Day close\n",
	})
}
