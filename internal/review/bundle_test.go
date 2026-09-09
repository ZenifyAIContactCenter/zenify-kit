package review

import (
	"reflect"
	"testing"
)

func TestPlanBundles_Passthrough(t *testing.T) {
	// total 1500 <= trigger 2000 → passthrough, empty (non-nil) bundles.
	files := []FileStat{{Path: "a/x.go", LOC: 500}, {Path: "a/y.go", LOC: 1000}}
	p := PlanBundles(files, 2000, 600, 8)
	if p.Verdict != "passthrough" {
		t.Fatalf("verdict=%q, want passthrough", p.Verdict)
	}
	if p.TotalLOC != 1500 {
		t.Errorf("total=%d, want 1500", p.TotalLOC)
	}
	if p.Bundles == nil || len(p.Bundles) != 0 {
		t.Errorf("bundles=%v, want empty non-nil", p.Bundles)
	}
}

func TestPlanBundles_PacksSmallFiles(t *testing.T) {
	// 13 files at 200 LOC = 2600 > 2000; cap 600 → each bundle packs 3 files (600).
	var files []FileStat
	for i := 0; i < 13; i++ {
		files = append(files, FileStat{Path: "pkg/f" + string(rune('a'+i)) + ".go", LOC: 200})
	}
	p := PlanBundles(files, 2000, 600, 8)
	if p.Verdict != "bundle" {
		t.Fatalf("verdict=%q, want bundle", p.Verdict)
	}
	for _, b := range p.Bundles {
		if b.LOC > 600 {
			t.Errorf("bundle %d LOC=%d > cap 600", b.ID, b.LOC)
		}
	}
	// IDs run consecutively from 1.
	for i, b := range p.Bundles {
		if b.ID != i+1 {
			t.Errorf("bundle #%d has ID=%d, want %d", i, b.ID, i+1)
		}
	}
}

func TestPlanBundles_SingleFileOverCap(t *testing.T) {
	// file at 700 > cap 600 → becomes its own bundle (LOC over cap is valid).
	files := []FileStat{{Path: "a/big.go", LOC: 700}, {Path: "b/s1.go", LOC: 700}, {Path: "b/s2.go", LOC: 700}}
	p := PlanBundles(files, 2000, 600, 8) // total 2100 > 2000
	if p.Verdict != "bundle" {
		t.Fatalf("verdict=%q, want bundle", p.Verdict)
	}
	// each 700-LOC file must sit alone (700+700 > 600).
	for _, b := range p.Bundles {
		if len(b.Files) != 1 {
			t.Errorf("bundle %d has %d files, want 1 (each file at 700 > cap)", b.ID, len(b.Files))
		}
	}
}

func TestPlanBundles_TooLarge(t *testing.T) {
	// 10 files at 600 LOC = 6000; cap 600 → 10 bundles > max 8 → too-large.
	var files []FileStat
	for i := 0; i < 10; i++ {
		files = append(files, FileStat{Path: "z/f" + string(rune('a'+i)) + ".go", LOC: 600})
	}
	p := PlanBundles(files, 2000, 600, 8)
	if p.Verdict != "too-large" {
		t.Fatalf("verdict=%q, want too-large", p.Verdict)
	}
	if len(p.Bundles) != 0 {
		t.Errorf("too-large must return empty bundles, got %d", len(p.Bundles))
	}
}

func TestPlanBundles_Deterministic(t *testing.T) {
	a := []FileStat{{Path: "a/x.go", LOC: 400}, {Path: "b/y.go", LOC: 900}, {Path: "a/z.go", LOC: 900}}
	b := []FileStat{{Path: "b/y.go", LOC: 900}, {Path: "a/z.go", LOC: 900}, {Path: "a/x.go", LOC: 400}}
	if !reflect.DeepEqual(PlanBundles(a, 2000, 600, 8), PlanBundles(b, 2000, 600, 8)) {
		t.Errorf("PlanBundles is not deterministic across input order")
	}
}
