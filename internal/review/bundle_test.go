package review

import (
	"reflect"
	"testing"
)

func TestPlanBundles_Passthrough(t *testing.T) {
	// tổng 1500 <= trigger 2000 → passthrough, bundles rỗng (không nil).
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
	// 13 file 200 LOC = 2600 > 2000; cap 600 → mỗi bundle gói 3 file (600).
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
	// ID liên tục từ 1.
	for i, b := range p.Bundles {
		if b.ID != i+1 {
			t.Errorf("bundle thứ %d có ID=%d, want %d", i, b.ID, i+1)
		}
	}
}

func TestPlanBundles_SingleFileOverCap(t *testing.T) {
	// file 700 > cap 600 → tự thành 1 bundle riêng (LOC vượt cap là hợp lệ).
	files := []FileStat{{Path: "a/big.go", LOC: 700}, {Path: "b/s1.go", LOC: 700}, {Path: "b/s2.go", LOC: 700}}
	p := PlanBundles(files, 2000, 600, 8) // tổng 2100 > 2000
	if p.Verdict != "bundle" {
		t.Fatalf("verdict=%q, want bundle", p.Verdict)
	}
	// mỗi file 700 phải nằm một mình (700+700 > 600).
	for _, b := range p.Bundles {
		if len(b.Files) != 1 {
			t.Errorf("bundle %d có %d file, want 1 (mỗi file 700 > cap)", b.ID, len(b.Files))
		}
	}
}

func TestPlanBundles_TooLarge(t *testing.T) {
	// 10 file 600 LOC = 6000; cap 600 → 10 bundle > max 8 → too-large.
	var files []FileStat
	for i := 0; i < 10; i++ {
		files = append(files, FileStat{Path: "z/f" + string(rune('a'+i)) + ".go", LOC: 600})
	}
	p := PlanBundles(files, 2000, 600, 8)
	if p.Verdict != "too-large" {
		t.Fatalf("verdict=%q, want too-large", p.Verdict)
	}
	if len(p.Bundles) != 0 {
		t.Errorf("too-large phải trả bundles rỗng, got %d", len(p.Bundles))
	}
}

func TestPlanBundles_Deterministic(t *testing.T) {
	a := []FileStat{{Path: "a/x.go", LOC: 400}, {Path: "b/y.go", LOC: 900}, {Path: "a/z.go", LOC: 900}}
	b := []FileStat{{Path: "b/y.go", LOC: 900}, {Path: "a/z.go", LOC: 900}, {Path: "a/x.go", LOC: 400}}
	if !reflect.DeepEqual(PlanBundles(a, 2000, 600, 8), PlanBundles(b, 2000, 600, 8)) {
		t.Errorf("PlanBundles không deterministic theo thứ tự input")
	}
}
