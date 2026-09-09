// Package speclife derives a spec's lifecycle state (planned/in-progress/built/built?/superseded/
// unknown) and a contract registry from data internal/release already produces. Pure: no I/O — the
// CLI layer gathers specs, git changes and note-commits and passes them in.
package speclife

import (
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/release"
)

// State is a spec's lifecycle state.
type State string

const (
	StatePlanned    State = "planned"     // a spec exists, no plan, nothing shipped
	StateInProgress State = "in-progress" // a plan exists but nothing links a shipped change
	StateBuilt      State = "built"       // a change links the spec via note or Spec: trailer
	StateBuiltFuzzy State = "built?"      // a change links only by slug (speculative)
	StateSuperseded State = "superseded"  // another spec's _Supersedes: names this one
	StateUnknown    State = "unknown"     // git was not evaluable for this repo — cannot tell
)

// Classify maps the four inputs to a state. Order: superseded is authoritative; then a real link
// (note/trailer = built, slug = built?); then, with no link, git-evaluability gates
// unknown-vs-plan-based. Mirrors the spec's Flow classify block.
func Classify(hasPlan bool, tier release.LinkTier, gitEvaluable, superseded bool) State {
	if superseded {
		return StateSuperseded
	}
	switch tier {
	case release.TierNote, release.TierTrailer:
		return StateBuilt
	case release.TierSlugExact, release.TierSlugFuzzy:
		return StateBuiltFuzzy
	}
	// tier == TierNone
	if !gitEvaluable {
		return StateUnknown
	}
	if hasPlan {
		return StateInProgress
	}
	return StatePlanned
}

// Status is one spec's computed lifecycle row.
type Status struct {
	Path         string           `json:"path"`
	Repo         string           `json:"repo"`
	Slug         string           `json:"slug"`
	State        State            `json:"state"`
	Tier         release.LinkTier `json:"tier"`
	HasPlan      bool             `json:"has_plan"`
	GitEvaluable bool             `json:"git_evaluable"`
}

// RepoOf returns the <repo> segment of a "specs/<repo>/<file>" path ("" if the shape doesn't match).
func RepoOf(specPath string) string {
	parts := strings.Split(specPath, "/")
	if len(parts) >= 3 && parts[0] == "specs" {
		return parts[1]
	}
	return ""
}

// Statuses computes a Status per spec.
//   - hasPlan[specPath]      : does a mirror plan file exist (CLI resolves this).
//   - changesByRepo[repo]    : git changes aggregated from that repo's base branch.
//   - notesByRepo[repo]      : release note-commit risk, keyed by normalized slug (NoteRiskBySlug).
//   - gitEvaluable[repo]     : did the repo's git log succeed (false → unknown).
func Statuses(
	specs []release.SpecMeta,
	hasPlan map[string]bool,
	changesByRepo map[string][]release.Change,
	notesByRepo map[string]map[string]release.RiskMeta,
	gitEvaluable map[string]bool,
) []Status {
	superseded := supersedeSet(specs)

	// Reverse-index the strongest tier that links each spec, reusing the canonical linker.
	// tierByPath: keyed by the spec path a trailer/slug match resolved to (precise).
	// tierBySlug: keyed by change slug, ONLY for note tier (whose RiskMeta carries no spec path).
	tierByPath := map[string]release.LinkTier{}
	tierBySlug := map[string]release.LinkTier{}
	for repo, changes := range changesByRepo {
		notes := notesByRepo[repo]
		for _, ch := range changes {
			rm, tier := release.LinkSpecTier(ch, specs, notes)
			if tier == release.TierNone {
				continue
			}
			if tier == release.TierNote && (rm.SpecPath == "" || rm.SpecPath == "note") {
				k := release.NormalizeKey(ch.Slug)
				if tier > tierBySlug[k] {
					tierBySlug[k] = tier
				}
				continue
			}
			if rm.SpecPath != "" && rm.SpecPath != "note" {
				if tier > tierByPath[rm.SpecPath] {
					tierByPath[rm.SpecPath] = tier
				}
			}
		}
	}

	out := make([]Status, 0, len(specs))
	for _, s := range specs {
		repo := RepoOf(s.Path)
		tier := tierByPath[s.Path]
		if t := tierBySlug[s.Slug]; t > tier {
			tier = t
		}
		st := Classify(hasPlan[s.Path], tier, gitEvaluable[repo], superseded[release.NormalizeKey(s.Slug)])
		out = append(out, Status{
			Path:         s.Path,
			Repo:         repo,
			Slug:         s.Slug,
			State:        st,
			Tier:         tier,
			HasPlan:      hasPlan[s.Path],
			GitEvaluable: gitEvaluable[repo],
		})
	}
	return out
}

// supersedeSet returns the normalized slugs named by any spec's _Supersedes: tag.
func supersedeSet(specs []release.SpecMeta) map[string]bool {
	m := map[string]bool{}
	for _, s := range specs {
		if s.Supersedes != "" {
			m[release.NormalizeKey(s.Supersedes)] = true
		}
	}
	return m
}
