package plugin

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runRoute(t *testing.T, strong string, args ...string) (model, gates string) {
	t.Helper()
	script := filepath.Join("assets", "znf", "skills", "_shared", "scripts", "select-route")
	cmd := exec.Command("bash", append([]string{script}, args...)...)
	cmd.Env = append(os.Environ(), "ZNF_STRONG_MODEL="+strong)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("select-route %v: %v", args, err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 lines, got %q", out)
	}
	return strings.TrimPrefix(lines[0], "model="), strings.TrimPrefix(lines[2], "gates: ")
}

func TestSelectRoute_Table(t *testing.T) {
	cases := []struct {
		strong    string
		args      []string
		wantModel string
		wantGates string
	}{
		{"fable", []string{"architect", "TIER=bounded", "REPOS=3", "SHARED=1"}, "none", "-"},
		{"fable", []string{"architect", "TIER=architectural", "REPOS=1", "SHARED=0", "NEW_CONTRACT=0", "CRITICAL=0"}, "none", "-"},
		{"fable", []string{"architect", "TIER=architectural", "REPOS=2"}, "fable", "REPOS>=2"},
		{"fable", []string{"architect", "TIER=architectural", "REPOS=1", "SHARED=1", "CRITICAL=1"}, "fable", "SHARED,CRITICAL"},
		{"", []string{"architect", "TIER=architectural", "NEW_CONTRACT=1"}, "opus", "NEW_CONTRACT"},
		{"xyz", []string{"architect", "TIER=architectural", "CRITICAL=1"}, "opus", "CRITICAL"},
		{"fable", []string{"investigator", "ROUND=1"}, "sonnet", "-"},
		{"fable", []string{"investigator", "ROUND=2"}, "opus", "ROUND=2"},
		{"fable", []string{"investigator", "ROUND=3"}, "fable", "ROUND>=3"},
		{"", []string{"investigator", "ROUND=4"}, "opus", "ROUND>=3"},
		{"fable", []string{"implementer", "SPEC=code", "FAIL=0"}, "haiku", "-"},
		{"fable", []string{"implementer", "SPEC=prose", "FAIL=1"}, "sonnet", "-"},
		{"fable", []string{"implementer", "SPEC=prose", "FAIL=2"}, "opus", "FAIL=2"},
		{"fable", []string{"implementer", "SPEC=code", "FAIL=3"}, "none", "FAIL>=3"},
		{"fable", []string{"reviewer", "TIER=T3", "BLOCKED_STREAK=1"}, "inherit", "-"},
		{"fable", []string{"reviewer", "TIER=T2", "BLOCKED_STREAK=5"}, "inherit", "-"},
		{"fable", []string{"reviewer", "TIER=T3", "BLOCKED_STREAK=2"}, "fable", "BLOCKED_STREAK>=2"},
		{"opus", []string{"manual"}, "opus", "TAG"},
		{"fable", []string{"manual"}, "fable", "TAG"},
	}
	for _, c := range cases {
		m, g := runRoute(t, c.strong, c.args...)
		if m != c.wantModel || g != c.wantGates {
			t.Errorf("select-route(%s, %v) = %s/%s, want %s/%s", c.strong, c.args, m, g, c.wantModel, c.wantGates)
		}
	}
}

func TestSelectRoute_BadInputExit2(t *testing.T) {
	script := filepath.Join("assets", "znf", "skills", "_shared", "scripts", "select-route")
	for _, args := range [][]string{{}, {"nowhere"}, {"investigator", "ROUND=x"}, {"architect", "BOGUS=1"}} {
		err := exec.Command("bash", append([]string{script}, args...)...).Run()
		ee, ok := err.(*exec.ExitError)
		if !ok || ee.ExitCode() != 2 {
			t.Errorf("args %v: want exit 2, got %v", args, err)
		}
	}
}
