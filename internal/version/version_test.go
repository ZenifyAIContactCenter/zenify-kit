package version

import "testing"

func TestCurrent_DefaultsToDev(t *testing.T) {
	if got := Current(); got == "" {
		t.Error("Current() returned empty; want non-empty default")
	}
}

func TestCurrent_ReflectsInjected(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })
	Version = "v1.2.3"
	if got := Current(); got != "v1.2.3" {
		t.Errorf("Current() = %q, want v1.2.3", got)
	}
}
