package release

import (
	"slices"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

// noTagNote is the per-repo line for a repo that carries no release tag at all.
const noTagNote = "chưa có tag vYYMMDD.n" //znf:allow-lang

// stagingRef is the integration branch: regression is "not on staging", and a PR merged from it
// into main is the weekly release whose inner PRs are the report's changes.
const stagingRef = "origin/staging"

// Build assembles the report for release tag `tag` (vYYMMDD.n) from a list of repos. tag "" means
// the newest tag name across all repos; in that default mode a repo that does not carry it is
// omitted, while an explicit tag keeps a "chưa có tag" row for it. Each repo's range runs from //znf:allow-lang
// the tag just before to the tag, hotfix tags included. Fail-open per repo: one repo's error does
// not break the whole report. loadPatterns is injected so tests don't depend on a real
// worktree.json. resolve locates each repo's real dir (via workspace.Resolve) — it does not assume
// a repo is a direct child of the workspace, so it is nesting-safe.
func Build(r gitx.Runner, resolve func(name string) (string, bool), repos []string, tag string, loadPatterns func(dir string) []string, loadSpecs func(repo string) []SpecMeta) Report {
	return buildReport(r, resolve, repos, tag, loadPatterns, loadSpecs)
}

func buildReport(r gitx.Runner, resolve func(name string) (string, bool), repos []string, tag string, loadPatterns func(dir string) []string, loadSpecs func(repo string) []SpecMeta) Report {
	defaultMode := tag == ""
	if defaultMode {
		tag = newestTag(r, resolve, repos)
	}
	rep := Report{
		Tag:             tag,
		GeneratedAt:     time.Now().Format("2006-01-02 15:04"),
		SharedCrossRepo: map[string][]string{},
	}
	for _, name := range repos {
		dir, ok := resolve(name)
		if !ok {
			rep.Repos = append(rep.Repos, RepoReport{Name: name, Err: "could not locate repo in workspace"})
			continue
		}
		tags, err := releaseTags(r, dir)
		if err != nil {
			rep.Repos = append(rep.Repos, RepoReport{Name: name, Err: "không đọc được tag release: " + err.Error()}) //znf:allow-lang
			continue
		}
		if len(tags) == 0 {
			rep.Repos = append(rep.Repos, RepoReport{Name: name, Err: noTagNote})
			continue
		}
		if !slices.Contains(tags, tag) {
			if !defaultMode {
				rep.Repos = append(rep.Repos, RepoReport{Name: name, Err: "chưa có tag " + tag}) //znf:allow-lang
			}
			continue
		}
		prev, ok := PrevTag(tags, tag)
		if !ok {
			rep.Repos = append(rep.Repos, RepoReport{Name: name, Err: "không tìm thấy tag liền trước " + tag}) //znf:allow-lang
			continue
		}
		rr := RepoReport{Name: name, Tag: tag, PrevTag: prev, TypeCounts: map[string]int{}}
		var notes []Commit
		if cs, err := RangeCommits(r, dir, prev, tag); err == nil {
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
		if fs, err := ChangedFiles(r, dir, prev, tag); err == nil {
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
		if cs, err := NotInStaging(r, dir, prev, tag, stagingRef); err == nil {
			rr.Regression = cs
		} else {
			rr.RegressionUncomputed = true
		}
		// the set of SHAs not-yet-on-staging, used to mark a Change.
		notStaging := map[string]bool{}
		for _, c := range rr.Regression {
			notStaging[c.SHA] = true
		}
		aggIn := rr.Commits // flat fallback (degrade-safe)
		if gcs, err := RangeCommitsGrouped(r, dir, prev, tag, stagingRef); err == nil {
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

// newestTag returns the newest release tag name across repos (by tagLess); "" when none carries one.
// Unresolvable or unreadable repos are skipped here — buildReport reports them in its own pass.
func newestTag(r gitx.Runner, resolve func(name string) (string, bool), repos []string) string {
	newest := ""
	for _, name := range repos {
		dir, ok := resolve(name)
		if !ok {
			continue
		}
		tags, err := releaseTags(r, dir)
		if err != nil || len(tags) == 0 {
			continue
		}
		if t := tags[len(tags)-1]; newest == "" || tagLess(newest, t) {
			newest = t
		}
	}
	return newest
}
