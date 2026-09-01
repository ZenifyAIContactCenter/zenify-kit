package wt

import (
	"fmt"
	"net"
	"testing"
)

func TestAllocate_Deterministic(t *testing.T) {
	// Same key + range → same port when nothing is taken and the port is free.
	p1, ok1 := Allocate("repo:slug:web", 3200, 3249, nil)
	p2, ok2 := Allocate("repo:slug:web", 3200, 3249, nil)
	if !ok1 || !ok2 || p1 != p2 {
		t.Fatalf("not deterministic: %d/%v vs %d/%v", p1, ok1, p2, ok2)
	}
	if p1 < 3200 || p1 > 3249 {
		t.Fatalf("port %d out of range", p1)
	}
}

func TestAllocate_SkipsTaken(t *testing.T) {
	// Force a collision: pre-take the deterministic start, expect a different port.
	start, _ := Allocate("k", 3200, 3249, nil)
	got, ok := Allocate("k", 3200, 3249, map[int]bool{start: true})
	if !ok || got == start {
		t.Fatalf("did not skip taken start %d (got %d)", start, got)
	}
}

func TestAllocate_ProbesPastListeningPort(t *testing.T) {
	// Occupy the deterministic start with a real listener; Allocate must bind-test
	// past it to a free port.
	start, _ := Allocate("busy", 3400, 3449, nil)
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", start))
	if err != nil {
		t.Skipf("cannot occupy port %d: %v", start, err)
	}
	defer ln.Close()
	got, ok := Allocate("busy", 3400, 3449, nil)
	if !ok || got == start {
		t.Fatalf("did not probe past listening port %d (got %d)", start, got)
	}
}

func TestAllocate_FullRangeExhausted(t *testing.T) {
	taken := map[int]bool{3200: true, 3201: true}
	if _, ok := Allocate("x", 3200, 3201, taken); ok {
		t.Fatal("want (_, false) when every port is taken")
	}
}

func TestAllocateRange_ContiguousBlock(t *testing.T) {
	ports, ok := AllocateRange("repo:slug:hub", 3200, 3299, 3, nil)
	if !ok {
		t.Fatal("want ok")
	}
	if len(ports) != 3 {
		t.Fatalf("want 3 ports, got %d", len(ports))
	}
	for i, p := range ports {
		if p < 3200 || p > 3299 {
			t.Fatalf("port %d out of range", p)
		}
		if i > 0 && ports[i] != ports[i-1]+1 {
			t.Fatalf("not contiguous: %v", ports)
		}
	}
}

func TestAllocateRange_CountOne(t *testing.T) {
	// count=1 must degenerate to a single-element slice matching a plain probe.
	single, singleOK := Allocate("repo:slug:single", 3200, 3249, nil)
	ports, ok := AllocateRange("repo:slug:single", 3200, 3249, 1, nil)
	if !ok || !singleOK {
		t.Fatal("want ok")
	}
	if len(ports) != 1 || ports[0] != single {
		t.Fatalf("want [%d], got %v", single, ports)
	}
}

func TestAllocateRange_SkipsTakenBlock(t *testing.T) {
	base, _ := AllocateRange("k-range", 3200, 3249, 3, nil)
	taken := map[int]bool{base[0]: true}
	got, ok := AllocateRange("k-range", 3200, 3249, 3, taken)
	if !ok {
		t.Fatal("want ok")
	}
	for _, p := range got {
		if taken[p] {
			t.Fatalf("returned block %v overlaps taken port %d", got, base[0])
		}
	}
}

func TestAllocateRange_SpanTooSmall(t *testing.T) {
	if _, ok := AllocateRange("x", 3200, 3201, 5, nil); ok {
		t.Fatal("want (_, false) when span < count")
	}
}

func TestAllocateRange_Deterministic(t *testing.T) {
	p1, ok1 := AllocateRange("repo:slug:det", 3200, 3299, 4, nil)
	p2, ok2 := AllocateRange("repo:slug:det", 3200, 3299, 4, nil)
	if !ok1 || !ok2 {
		t.Fatal("want ok")
	}
	if len(p1) != len(p2) {
		t.Fatalf("length mismatch: %v vs %v", p1, p2)
	}
	for i := range p1 {
		if p1[i] != p2[i] {
			t.Fatalf("not deterministic: %v vs %v", p1, p2)
		}
	}
}
