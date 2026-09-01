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
