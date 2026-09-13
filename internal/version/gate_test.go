package version

import (
	"errors"
	"testing"
)

func TestMeetsMin(t *testing.T) {
	cases := []struct {
		current, min string
		want         bool
	}{
		{"v1.2.3", "v1.2.0", true},
		{"v1.2.0", "v1.2.0", true},
		{"v1.1.9", "v1.2.0", false},
		{"dev", "v9.9.9", true},    // local build never blocked
		{"0.17.2", "v0.3.0", true}, // goreleaser {{.Version}} has no "v" prefix
		{"0.2.9", "v0.3.0", false},
		{"v0.17.2", "0.3.0", true}, // min without prefix is accepted too
	}
	for _, c := range cases {
		got, err := MeetsMin(c.current, c.min)
		if err != nil {
			t.Fatalf("MeetsMin(%q,%q) err: %v", c.current, c.min, err)
		}
		if got != c.want {
			t.Errorf("MeetsMin(%q,%q)=%v want %v", c.current, c.min, got, c.want)
		}
	}
}

func TestGuardMutation_BlocksOld(t *testing.T) {
	err := GuardMutation("v1.0.0", "v2.0.0")
	if !errors.Is(err, ErrTooOld) {
		t.Errorf("want ErrTooOld, got %v", err)
	}
}

func TestGuardMutation_AllowsCurrent(t *testing.T) {
	if err := GuardMutation("v2.0.0", "v2.0.0"); err != nil {
		t.Errorf("want nil, got %v", err)
	}
}
