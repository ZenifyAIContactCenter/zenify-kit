package plugin

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runTier(t *testing.T, loc, shared, critical string) string {
	t.Helper()
	script := filepath.Join("assets", "znf", "skills", "review", "scripts", "select-tier")
	out, err := exec.Command("bash", script, loc, shared, critical).Output()
	if err != nil {
		t.Fatalf("select-tier %s %s %s: %v", loc, shared, critical, err)
	}
	return strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
}

func TestSelectTier(t *testing.T) {
	cases := []struct {
		loc, shared, critical, want string
	}{
		{"50", "0", "0", "T1"},
		{"200", "0", "0", "T1"},
		{"350", "0", "0", "T2"},
		{"600", "0", "0", "T2"},
		{"900", "0", "0", "T3"},
		{"50", "1", "0", "T2"},  // W3: shared floors at T2 (was forced T3)
		{"300", "1", "0", "T2"}, // shared + medium -> T2
		{"700", "1", "0", "T3"}, // shared + large -> T3 by size
		{"50", "0", "1", "T3"},  // critical forces T3 regardless of size
		{"700", "0", "1", "T3"},
	}
	for _, c := range cases {
		if got := runTier(t, c.loc, c.shared, c.critical); got != c.want {
			t.Errorf("select-tier(%s,%s,%s)=%s, want %s", c.loc, c.shared, c.critical, got, c.want)
		}
	}
}
