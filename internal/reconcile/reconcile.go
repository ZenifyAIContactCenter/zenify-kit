// Package reconcile turns the desired-set (manifest ∩ access) and the scanned
// actual-state into a plan-diff. B1 only classifies and reports — it never acts.
package reconcile

import (
	"fmt"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ghx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
)

// State is the plan-diff verdict for one repo (Terraform-style vocabulary).
type State string

const (
	Clone       State = "CLONE"
	OK          State = "OK"
	Adopt       State = "ADOPT"
	Wire        State = "WIRE-config"
	Migrate     State = "MIGRATE-layout"
	SkipDirty   State = "DRIFT:skip-dirty"
	WrongRemote State = "wrong-remote"
	NoAccess    State = "SKIP:no-access"
	Skip        State = "SKIP"
)

// RepoPlan is the classified action for one manifest repo.
type RepoPlan struct {
	Name   string `json:"name"`
	State  State  `json:"state"`
	Reason string `json:"reason"`
	Path   string `json:"path"`
}

// Build classifies each manifest repo. It is pure: same inputs, same output,
// no I/O. access is keyed by repo name (present ⇒ viewer can access); scans is
// keyed by repo name (RepoState from a gitx.Scan at the manifest path).
func Build(m *manifest.Manifest, access map[string]ghx.RemoteRepo, scans map[string]gitx.RepoState) []RepoPlan {
	plans := make([]RepoPlan, 0, len(m.Repos))
	for _, r := range m.Repos {
		p := RepoPlan{Name: r.Name, Path: r.Path}
		st := scans[r.Name]
		switch {
		case !hasAccess(access, r.Name):
			p.State, p.Reason = NoAccess, "no access — ask lead"
		case !st.Cloned:
			p.State, p.Reason = Clone, "absent — would clone"
		case st.Dirty:
			p.State, p.Reason = SkipDirty, "working tree dirty — will not touch"
		case remoteMismatch(r.URL, st.NormalizedRemote):
			p.State, p.Reason = WrongRemote, fmt.Sprintf("remote %s ≠ manifest %s — report only", st.NormalizedRemote, canonical(r.URL))
		case st.Layout == "old":
			p.State, p.Reason = Migrate, "gitignore hides .claude — layout flip pending (B2)"
		case !st.HasClaude:
			p.State, p.Reason = Wire, "no .claude/ — config wiring pending (B2)"
		case st.HasClaude && st.Layout == "new" && onBase(r.Base, st.Branch):
			p.State, p.Reason = OK, "adopted, clean, on base"
		default:
			p.State, p.Reason = Adopt, "adopt in place pending (B2)"
		}
		plans = append(plans, p)
	}
	return plans
}

func hasAccess(access map[string]ghx.RemoteRepo, name string) bool {
	_, ok := access[name]
	return ok
}

// canonical reduces a manifest URL to owner/repo for comparison.
func canonical(url string) string {
	return gitx.NormalizeRemote(url, nil)
}

func remoteMismatch(manifestURL, normalizedRemote string) bool {
	if normalizedRemote == "" {
		return false // unknown remote is not a mismatch we can assert
	}
	return normalizedRemote != canonical(manifestURL)
}

// onBase reports whether the current branch matches the manifest base ref
// (base is like "origin/staging"; the local branch is "staging").
func onBase(base, branch string) bool {
	b := base
	if i := strings.LastIndex(base, "/"); i >= 0 {
		b = base[i+1:]
	}
	return branch == b
}
