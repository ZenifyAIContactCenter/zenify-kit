package release

import (
	"fmt"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
)

// Build ráp report cho release n từ danh sách repo. Fail-open mỗi repo: lỗi một repo
// không làm hỏng cả report. loadPatterns inject để test không phụ thuộc worktree.json thật.
// resolve định vị dir thật của mỗi repo (qua workspace.Resolve) — không giả định repo là
// con trực tiếp của workspace, nesting-safe.
func Build(r gitx.Runner, resolve func(name string) (string, bool), repos []string, n int, loadPatterns func(dir string) []string, loadSpecs func(repo string) []SpecMeta) Report {
	return buildReport(r, resolve, repos, n, false, loadPatterns, loadSpecs)
}

// BuildUnreleased ráp report "đang hình thành" cho release latestN (release đang gom, chưa
// deploy — convention A). Range = release<prev>..origin/staging với prev = release đã-deploy
// gần nhất, tức toàn bộ delta của release đang hình thành so với production. Cùng lõi buildReport,
// không tính regression (to==staging nên NotInStaging luôn rỗng) và không có CutDate (chưa cắt).
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
			rep.Repos = append(rep.Repos, RepoReport{Name: name, Err: "không định vị được repo trong workspace"})
			continue
		}
		nums, err := ReleaseNums(r, dir)
		if err != nil {
			rep.Repos = append(rep.Repos, RepoReport{Name: name, Err: "không đọc được release branches: " + err.Error()})
			continue
		}
		var relPrev, relN string
		var prevForReport int
		if unreleased {
			// unreleased = view "pending deploy" cập nhật theo staging HẰNG NGÀY cho MỌI repo
			// deploy — KHÔNG đòi repo phải cắt release<n>. Mốc dưới = release ĐÃ-DEPLOY gần nhất
			// của CHÍNH repo = release tồn-tại lớn nhất < n (forming): với repo đang gom R<n> đó là
			// release cắt trước; với repo tuần này chưa gom (chưa có release<n>) đó chính là release
			// max của nó. PrevRelease(nums,n) trả đúng cả hai. (n = forming = release cao nhất toàn
			// workspace; numbering dùng chung một dãy.) Repo chỉ có release>=n (mới tinh, không mốc
			// so) → bỏ qua.
			prev, ok := PrevRelease(nums, n)
			if !ok {
				continue
			}
			relPrev = fmt.Sprintf("origin/release%d", prev)
			relN = "origin/staging"
			prevForReport = prev
		} else {
			// finalize R<n>: một repo tham gia release n iff có nhánh release<n>.
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
				rep.Repos = append(rep.Repos, RepoReport{Name: name, Err: "không tìm được release trước"})
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
		// regression: khi unreleased, to==staging → NotInStaging luôn rỗng, khỏi gọi.
		if !unreleased {
			if cs, err := NotInStaging(r, dir, relPrev, relN, "origin/staging"); err == nil {
				rr.Regression = cs
			} else {
				rr.RegressionUncomputed = true
			}
		}
		// tập SHA chưa-trên-staging để đánh dấu Change.
		notStaging := map[string]bool{}
		for _, c := range rr.Regression {
			notStaging[c.SHA] = true
		}
		aggIn := rr.Commits // fallback phẳng (degrade-safe)
		if gcs, err := RangeCommitsGrouped(r, dir, relPrev, relN); err == nil {
			var gfeats []Commit
			for _, c := range gcs {
				if !IsReleaseNote(c) { // note-commit KHÔNG vào bucket (giữ option B)
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
		// unreleased: repo không có commit nào pending (staging == release đã-deploy của nó) →
		// bỏ khỏi view, không liệt kê section rỗng (giữ doc gọn, đúng "không đổi thì bỏ qua").
		// Repo lỗi (rr.Err) vẫn giữ để lộ sự cố.
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
	// FR-1.3: dòng thứ tự deploy chỉ nêu khi có ≥2 repo cùng chạm một shared-collection
	// (tính SAU prune), không phải bất cứ khi nào >1 repo ship.
	rep.DeployOrderNote = len(rep.SharedCrossRepo) > 0
	// headline aggregates: chỉ tính trên repo tham gia không lỗi.
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
				// không đếm vào headline
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
