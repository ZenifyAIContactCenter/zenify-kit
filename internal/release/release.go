package release

import (
	"fmt"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

// Build assembles the report for release n from a list of repos. Fail-open per repo: one repo's
// error does not break the whole report. loadPatterns is injected so tests don't depend on a real
// worktree.json. resolve locates each repo's real dir (via workspace.Resolve) — it does not assume
// a repo is a direct child of the workspace, so it is nesting-safe.
func Build(r gitx.Runner, resolve func(name string) (string, bool), repos []string, n int, loadPatterns func(dir string) []string, loadSpecs func(repo string) []SpecMeta) Report {
	return buildReport(r, resolve, repos, n, false, loadPatterns, loadSpecs)
}

// BuildUnreleased assembles the "still forming" report for release latestN (the release currently
// being gathered, not yet deployed — convention A). Range = release<prev>..origin/staging with
// prev = the most recently deployed release, i.e. the whole delta of the forming release versus
// production. Shares buildReport's core; skips regression (to==staging so NotInStaging is always
// empty) and has no CutDate (not cut yet).
func BuildUnreleased(r gitx.Runner, resolve func(name string) (string, bool), repos []string, latestN int, loadPatterns func(dir string) []string, loadSpecs func(repo string) []SpecMeta) Report {
	return buildReport(r, resolve, repos, latestN, true, loadPatterns, loadSpecs)
}

func buildReport(r gitx.Runner, resolve func(name string) (string, bool), repos []string, n int, unreleased bool, loadPatterns func(dir string) []string, loadSpecs func(repo string) []SpecMeta) Report {
	rep := Report{
		N:               n,
		GeneratedAt:     time.Now().Format("2006-01-02 15:04"),
		SharedCrossRepo: map[string][]string{},
		Unreleased:      unreleased,
	}
	for _, name := range repos {
		dir, ok := resolve(name)
		if !ok {
			rep.Repos = append(rep.Repos, RepoReport{Name: name, Err: "could not locate repo in workspace"})
			continue
		}
		nums, err := ReleaseNums(r, dir)
		if err != nil {
			rep.Repos = append(rep.Repos, RepoReport{Name: name, Err: "could not read release branches: " + err.Error()})
			continue
		}
		var relPrev, relN string
		var prevForReport int
		if unreleased {
			// unreleased = the "pending deploy" view, updated against staging DAILY for EVERY
			// deployed repo — it does NOT require the repo to have cut release<n>. The lower bound
			// = that repo's own most recently DEPLOYED release = its highest existing release < n
			// (forming): for a repo currently gathering R<n> that's the previously cut release; for
			// a repo that hasn't gathered anything this week (no release<n> yet) that's simply its
			// max release. PrevRelease(nums,n) returns the right answer for both cases. (n =
			// forming = the highest release across the whole workspace; numbering shares one
			// sequence.) A repo with only release>=n (brand new, no baseline to compare) → skipped.
			prev, ok := PrevRelease(nums, n)
			if !ok {
				continue
			}
			relPrev = fmt.Sprintf("origin/release%d", prev)
			relN = "origin/staging"
			prevForReport = prev
		} else {
			// finalize R<n>: a repo participates in release n iff it has a release<n> branch.
			has := false
			for _, x := range nums {
				if x == n {
					has = true
				}
			}
			if !has {
				rep.NotShipped = append(rep.NotShipped, name)
				continue
			}
			prev, ok := PrevRelease(nums, n)
			if !ok {
				rep.Repos = append(rep.Repos, RepoReport{Name: name, Err: "could not find the previous release"})
				continue
			}
			relPrev = fmt.Sprintf("origin/release%d", prev)
			relN = fmt.Sprintf("origin/release%d", n)
			prevForReport = prev
		}
		rr := RepoReport{Name: name, PrevRelease: prevForReport, TypeCounts: map[string]int{}}
		if !unreleased {
			rr.CutDate, _ = CutDate(r, dir, n)
		}
		var notes []Commit
		if cs, err := RangeCommits(r, dir, relPrev, relN); err == nil {
			var feats []Commit
			for _, c := range cs {
				if IsReleaseNote(c) {
					notes = append(notes, c)
				} else {
					feats = append(feats, c)
				}
			}
			rr.Commits = feats
			for _, c := range feats {
				rr.TypeCounts[c.Type]++
				if c.Merge && IsHotfixBranch(c.Branch) {
					rr.Hotfixes = append(rr.Hotfixes, c)
				}
			}
		} else {
			rr.Err = "log error: " + err.Error()
		}
		if fs, err := ChangedFiles(r, dir, relPrev, relN); err == nil {
			pats := loadPatterns(dir)
			hits := map[string]bool{}
			for _, f := range fs {
				if IsMigrationPath(f) {
					rr.HasMigration = true
				}
				if IsTestPath(f) {
					rr.HasTestTouch = true
				}
				if p, ok := MatchesAny(f, pats); ok {
					hits[p] = true
				}
			}
			for p := range hits {
				rr.SharedHits = append(rr.SharedHits, p)
				rep.SharedCrossRepo[p] = append(rep.SharedCrossRepo[p], name)
			}
		}
		// regression: when unreleased, to==staging → NotInStaging is always empty, so skip calling it.
		if !unreleased {
			if cs, err := NotInStaging(r, dir, relPrev, relN, "origin/staging"); err == nil {
				rr.Regression = cs
			} else {
				rr.RegressionUncomputed = true
			}
		}
		// the set of SHAs not-yet-on-staging, used to mark a Change.
		notStaging := map[string]bool{}
		for _, c := range rr.Regression {
			notStaging[c.SHA] = true
		}
		aggIn := rr.Commits // flat fallback (degrade-safe)
		if gcs, err := RangeCommitsGrouped(r, dir, relPrev, relN); err == nil {
			var gfeats []Commit
			for _, c := range gcs {
				if !IsReleaseNote(c) { // a note-commit does NOT go into a bucket (keeping option B)
					gfeats = append(gfeats, c)
				}
			}
			aggIn = gfeats
		}
		rr.Changes = Aggregate(aggIn, notStaging)
		specs := loadSpecs(name)
		noteMap := NoteRiskBySlug(notes)
		for i := range rr.Changes {
			rr.Changes[i].Risk = LinkSpec(rr.Changes[i], specs, noteMap)
		}
		// unreleased: a repo with no pending commits (staging == its own deployed release) →
		// dropped from the view, no empty section listed (keeps the doc terse, per "no change
		// means skip it"). A repo with an error (rr.Err) is still kept so the failure surfaces.
		if unreleased && rr.Err == "" && len(rr.Commits) == 0 {
			continue
		}
		rep.Repos = append(rep.Repos, rr)
	}
	for p, rs := range rep.SharedCrossRepo {
		if len(rs) < 2 {
			delete(rep.SharedCrossRepo, p)
		}
	}
	// FR-1.3: the deploy-order line is shown only when ≥2 repos touch the same shared collection
	// (computed AFTER pruning), not whenever >1 repo ships.
	rep.DeployOrderNote = len(rep.SharedCrossRepo) > 0
	// headline aggregates: computed only over participating repos without errors.
	for _, rr := range rep.Repos {
		if rr.Err != "" {
			continue
		}
		rep.ShippingRepos = append(rep.ShippingRepos, rr.Name)
		if rr.HasMigration {
			rep.Migrations = append(rep.Migrations, rr.Name)
		}
		for _, ch := range rr.Changes {
			switch ch.Type {
			case "feat":
				rep.TotalFeat++
			case "hotfix":
				rep.TotalHotfix++
				if ch.NotOnStaging {
					rep.HotfixesNotSynced++
				}
			case "chore", "other":
				// not counted toward the headline
			default:
				rep.TotalFix++
			}
			if ch.Type != "chore" && ch.Type != "other" {
				rep.SpecTotal++
				if ch.Risk.SpecPath != "" {
					rep.SpecLinked++
				}
			}
		}
	}
	return rep
}
