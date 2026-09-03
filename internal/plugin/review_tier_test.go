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
		{"50", "1", "0", "T3"}, // shared force T3
		{"50", "0", "1", "T3"}, // critical force T3
	}
	for _, c := range cases {
		if got := runTier(t, c.loc, c.shared, c.critical); got != c.want {
			t.Errorf("select-tier(%s,%s,%s)=%s, want %s", c.loc, c.shared, c.critical, got, c.want)
		}
	}
}
