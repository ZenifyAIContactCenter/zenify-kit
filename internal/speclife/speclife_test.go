package speclife

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/release"
)

// TestJSONKeysAreLowercase guards FR-01.6 / FR-02.4: the machine-readable output must use the
// documented lowercase keys, not Go's capitalized field names.
func TestJSONKeysAreLowercase(t *testing.T) {
	sb, _ := json.Marshal(Status{})
	for _, k := range []string{`"slug"`, `"path"`, `"state"`} {
		if !strings.Contains(string(sb), k) {
			t.Errorf("Status JSON missing key %s: %s", k, sb)
		}
	}
	if strings.Contains(string(sb), `"State"`) {
		t.Errorf("Status JSON leaks capitalized key: %s", sb)
	}
	cb, _ := json.Marshal(Contract{})
	for _, k := range []string{`"path"`, `"repo"`, `"blast_radius"`, `"db"`} {
		if !strings.Contains(string(cb), k) {
			t.Errorf("Contract JSON missing key %s: %s", k, cb)
		}
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name         string
		hasPlan      bool
		tier         release.LinkTier
		gitEvaluable bool
		superseded   bool
		want         State
	}{
		{"superseded wins over built", true, release.TierTrailer, true, true, StateSuperseded},
		{"trailer -> built", false, release.TierTrailer, true, false, StateBuilt},
		{"note -> built", false, release.TierNote, true, false, StateBuilt},
		{"exact slug -> built?", false, release.TierSlugExact, true, false, StateBuiltFuzzy},
		{"fuzzy slug -> built?", false, release.TierSlugFuzzy, true, false, StateBuiltFuzzy},
		{"git not evaluable -> unknown", true, release.TierNone, false, false, StateUnknown},
		{"has plan, no link -> in-progress", true, release.TierNone, true, false, StateInProgress},
		{"no plan, no link -> planned", false, release.TierNone, true, false, StatePlanned},
	}
	for _, c := range cases {
		if got := Classify(c.hasPlan, c.tier, c.gitEvaluable, c.superseded); got != c.want {
			t.Errorf("%s: Classify = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestRepoOf(t *testing.T) {
	if got := RepoOf("specs/zenify-kit/2026-09-09-x-design.md"); got != "zenify-kit" {
		t.Fatalf("RepoOf = %q, want zenify-kit", got)
	}
	if got := RepoOf("weird"); got != "" {
		t.Fatalf("RepoOf(weird) = %q, want empty", got)
	}
}

func TestStatuses_BuiltViaTrailer(t *testing.T) {
	spec := release.SpecMeta{Path: "specs/zenify-kit/2026-09-09-fields-design.md", Slug: "fields"}
	changes := map[string][]release.Change{
		"zenify-kit": {{Slug: "fields", Commits: []release.Commit{{
			Body: "Spec: specs/zenify-kit/2026-09-09-fields-design.md",
		}}}},
	}
	got := Statuses([]release.SpecMeta{spec}, map[string]bool{spec.Path: true},
		changes, nil, map[string]bool{"zenify-kit": true})
	if len(got) != 1 || got[0].State != StateBuilt {
		t.Fatalf("got %+v, want one StateBuilt", got)
	}
}

func TestStatuses_FuzzyIsBuiltQuestion(t *testing.T) {
	spec := release.SpecMeta{Path: "specs/zenify-kit/2026-09-09-fields-design.md", Slug: "fields"}
	changes := map[string][]release.Change{
		"zenify-kit": {{Slug: "fields-extra", Commits: []release.Commit{{Body: ""}}}},
	}
	got := Statuses([]release.SpecMeta{spec}, nil, changes, nil, map[string]bool{"zenify-kit": true})
	if got[0].State != StateBuiltFuzzy {
		t.Fatalf("State = %q, want built?", got[0].State)
	}
}

func TestStatuses_Superseded(t *testing.T) {
	old := release.SpecMeta{Path: "specs/zenify-kit/2026-09-01-old-design.md", Slug: "old"}
	nu := release.SpecMeta{Path: "specs/zenify-kit/2026-09-09-new-design.md", Slug: "new", Supersedes: "old"}
	got := Statuses([]release.SpecMeta{old, nu}, nil, nil, nil, map[string]bool{"zenify-kit": true})
	byPath := map[string]State{}
	for _, s := range got {
		byPath[s.Path] = s.State
	}
	if byPath[old.Path] != StateSuperseded {
		t.Fatalf("old state = %q, want superseded", byPath[old.Path])
	}
}

func TestStatuses_UnknownWhenGitNotEvaluable(t *testing.T) {
	spec := release.SpecMeta{Path: "specs/zenify-kit/2026-09-09-x-design.md", Slug: "x"}
	got := Statuses([]release.SpecMeta{spec}, map[string]bool{spec.Path: true}, nil, nil,
		map[string]bool{"zenify-kit": false})
	if got[0].State != StateUnknown {
		t.Fatalf("State = %q, want unknown", got[0].State)
	}
}

func TestStatuses_NoteLinkedBySlug(t *testing.T) {
	spec := release.SpecMeta{Path: "specs/zenify-kit/2026-09-09-fields-design.md", Slug: "fields"}
	changes := map[string][]release.Change{
		"zenify-kit": {{Slug: "fields", Commits: []release.Commit{{Body: ""}}}},
	}
	notes := map[string]map[string]release.RiskMeta{
		"zenify-kit": {"fields": {SpecPath: "note", BlastRadius: "x"}},
	}
	got := Statuses([]release.SpecMeta{spec}, nil, changes, notes, map[string]bool{"zenify-kit": true})
	if got[0].State != StateBuilt {
		t.Fatalf("State = %q, want built (note tier)", got[0].State)
	}
}
