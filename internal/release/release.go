package release

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

// Build ráp report cho release n từ danh sách repo. Fail-open mỗi repo: lỗi một repo
// không làm hỏng cả report. loadPatterns inject để test không phụ thuộc worktree.json thật.
func Build(r gitx.Runner, workspace string, repos []string, n int, loadPatterns func(dir string) []string) Report {
	rep := Report{
		N:               n,
		GeneratedAt:     time.Now().Format("2006-01-02 15:04"),
		SharedCrossRepo: map[string][]string{},
	}
	relN := fmt.Sprintf("origin/release%d", n)
	for _, name := range repos {
		dir := filepath.Join(workspace, name)
		nums, err := ReleaseNums(r, dir)
		if err != nil {
			rep.Repos = append(rep.Repos, RepoReport{Name: name, Err: "không đọc được release branches: " + err.Error()})
			continue
		}
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
			rep.Repos = append(rep.Repos, RepoReport{Name: name, Participated: true, Err: "không tìm được release trước"})
			continue
		}
		relPrev := fmt.Sprintf("origin/release%d", prev)
		rr := RepoReport{Name: name, Participated: true, PrevRelease: prev, TypeCounts: map[string]int{}}
		rr.CutDate, _ = CutDate(r, dir, n)
		if cs, err := RangeCommits(r, dir, relPrev, relN); err == nil {
			rr.Commits = cs
			for _, c := range cs {
				rr.TypeCounts[c.Type]++
				if c.Merge && IsHotfixBranch(c.Branch) {
					rr.Hotfixes = append(rr.Hotfixes, c)
				}
			}
		} else {
			rr.Err = "log lỗi: " + err.Error()
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
		if cs, err := NotInStaging(r, dir, relPrev, relN, "origin/staging"); err == nil {
			rr.Regression = cs
		}
		rep.Repos = append(rep.Repos, rr)
	}
	rep.DeployOrderNote = len(rep.Repos) > 1
	for p, rs := range rep.SharedCrossRepo {
		if len(rs) < 2 {
			delete(rep.SharedCrossRepo, p)
		}
	}
	return rep
}
