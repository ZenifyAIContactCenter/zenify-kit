// Package review — the smart-bundling part (BUNDLE seam of znf:review, M4c).
// Splits a large diff into file-cluster bundles small enough to review well. Mechanical,
// deterministic; does NOT use an LLM (doctrine seam).
package review

import "sort"

// FileStat: one file in the diff, with LOC = added + deleted.
type FileStat struct {
	Path string `json:"path"`
	LOC  int    `json:"loc"`
}

// Bundle: a cluster of files to review together.
type Bundle struct {
	ID    int      `json:"id"`
	LOC   int      `json:"loc"`
	Files []string `json:"files"`
}

// Plan: the bundling result. Verdict ∈ "passthrough" | "bundle" | "too-large".
type Plan struct {
	Verdict  string   `json:"verdict"`
	Bundles  []Bundle `json:"bundles"`
	TotalLOC int      `json:"total_loc"`
}

// PlanBundles splits files into bundles by path + greedy size-cap.
//   - total <= trigger → "passthrough" (no bundling needed).
//   - capLOC           → max LOC per bundle; a file > cap becomes its own bundle.
//   - max              → more bundles than this → "too-large".
//
// Sort by Path before packing → files in the same directory sit next to each other and
// pack into the same bundle (reduces cross-bundle blindness), and makes the result
// deterministic regardless of input order.
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
