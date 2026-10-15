package main

import "testing"

// The spec allows each login at most 2 concurrent /query children.
func TestMaxConcurrentPerLoginIsTwo(t *testing.T) {
	if maxConcurrentPerLogin != 2 {
		t.Fatalf("maxConcurrentPerLogin = %d, want 2", maxConcurrentPerLogin)
	}
}
