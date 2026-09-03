// Package review — phần smart-bundling (seam BUNDLE của znf:review, M4c).
// Chia một diff lớn thành các bundle cụm-file đủ nhỏ để review tốt. Cơ học,
// deterministic; KHÔNG dùng LLM (doctrine seam).
package review

import "sort"

// FileStat: một file trong diff với LOC = added + deleted.
type FileStat struct {
	Path string `json:"path"`
	LOC  int    `json:"loc"`
}

// Bundle: một cụm file để review cùng nhau.
type Bundle struct {
	ID    int      `json:"id"`
	LOC   int      `json:"loc"`
	Files []string `json:"files"`
}

// Plan: kết quả chia bundle. Verdict ∈ "passthrough" | "bundle" | "too-large".
type Plan struct {
	Verdict  string   `json:"verdict"`
	Bundles  []Bundle `json:"bundles"`
	TotalLOC int      `json:"total_loc"`
}

// PlanBundles chia files thành bundle theo path + size-cap greedy.
//   - tổng <= trigger  → "passthrough" (không cần bundle).
//   - capLOC           → LOC tối đa mỗi bundle; một file > cap tự thành bundle riêng.
//   - max              → quá số bundle này → "too-large".
//
// Sort theo Path trước khi gói → file cùng thư mục nằm liền nhau, gói vào cùng
// bundle (giảm mù cross-bundle), và làm kết quả deterministic bất kể thứ tự input.
func PlanBundles(files []FileStat, trigger, capLOC, max int) Plan {
	total := 0
	for _, f := range files {
		total += f.LOC
	}
	if total <= trigger {
		return Plan{Verdict: "passthrough", Bundles: []Bundle{}, TotalLOC: total}
	}

	sorted := make([]FileStat, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })

	bundles := []Bundle{}
	var cur Bundle
	flush := func() {
		if len(cur.Files) > 0 {
			bundles = append(bundles, cur)
			cur = Bundle{}
		}
	}
	for _, f := range sorted {
		if len(cur.Files) > 0 && cur.LOC+f.LOC > capLOC {
			flush()
		}
		cur.Files = append(cur.Files, f.Path)
		cur.LOC += f.LOC
	}
	flush()

	if len(bundles) > max {
		return Plan{Verdict: "too-large", Bundles: []Bundle{}, TotalLOC: total}
	}
	for i := range bundles {
		bundles[i].ID = i + 1
	}
	return Plan{Verdict: "bundle", Bundles: bundles, TotalLOC: total}
}
