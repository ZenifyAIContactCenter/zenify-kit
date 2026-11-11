package plugin

import (
	"os/exec"
	"path/filepath"
	"regexp"
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
		{"900", "0", "0", "T2"},
		{"700", "0", "0", "T2"},
		{"150", "0", "0", "T1"},
		{"150", "1", "0", "T2"},
		{"50", "1", "0", "T2"},  // W3: shared floors at T2 (was forced T3)
		{"300", "1", "0", "T2"}, // shared + medium -> T2
		{"700", "1", "0", "T2"}, // shared + large -> T2: size never forces T3
		{"5000", "1", "1", "T3"},
		{"50", "0", "1", "T3"}, // critical forces T3 regardless of size
		{"700", "0", "1", "T3"},
	}
	for _, c := range cases {
		if got := runTier(t, c.loc, c.shared, c.critical); got != c.want {
			t.Errorf("select-tier(%s,%s,%s)=%s, want %s", c.loc, c.shared, c.critical, got, c.want)
		}
	}
}

// A size threshold no longer rescues a sensitive diff, so the caller's CRITICAL must survive step 1.
func TestReviewKeepsCallerCritical(t *testing.T) {
	skill := syncedAsset(t, "skills/review/SKILL.md")
	if !strings.Contains(skill, `[ "${CRITICAL:-}" = 1 ] || CRITICAL=$(`) {
		t.Error("review SKILL.md step 1 must keep a caller's CRITICAL=1 and derive it otherwise")
	}
	bundling := syncedAsset(t, "skills/review/references/bundling.md")
	if !strings.Contains(bundling, `"$CRITICAL_b"`) {
		t.Error("bundling.md must select each bundle's tier from its own CRITICAL_b")
	}
	if strings.Contains(skill, "\nCRITICAL=0 ") {
		t.Error("review SKILL.md must not reset CRITICAL to 0")
	}
}

// No caller sets CRITICAL, so step 1 derives it from the changed paths: exact directory or file-stem names only.
func TestReviewDerivesCriticalFromPaths(t *testing.T) {
	skill := syncedAsset(t, "skills/review/SKILL.md")
	m := regexp.MustCompile(`rg -qi '([^']+)' && echo 1 \|\| echo 0\)`).FindStringSubmatch(skill)
	if m == nil || !strings.Contains(skill, "--no-renames") || !strings.Contains(skill, `grep -vE '\.(md|json|lock|snap)$'`) {
		t.Fatal("review SKILL.md step 1 must derive CRITICAL from code paths (no renames, docs excluded) with an rg -qi pattern")
	}
	re := regexp.MustCompile("(?i)" + m[1])
	for _, p := range []string{
		"migration/20261101-add-index.js", "apps/auth/src/auth.service.ts", "app/services/iam/role.js",
		"src/modules/permissions/permissions.service.ts", "app/migrations/001.js", "src/Tenant/x.go",
		"app/middleware/permission.js", "src/modules/mcp/oauth/token.ts", "apps/auth/src/security/jwt.ts",
	} {
		if !re.MatchString(p) {
			t.Errorf("%s must be a sensitive area", p)
		}
	}
	for _, p := range []string{
		"src/routes/_auth/settings/index.tsx", "app/services/campaign/authorizeCampaignDelete.js",
		"src/models/oauth-client.model.ts", "scripts/clone-tenant/lib/run.js", "src/author/list.tsx",
		"src/auth.mock/widget.ts", "src/tenant.assets/icon.svg",
	} {
		if re.MatchString(p) {
			t.Errorf("%s must not be a sensitive area", p)
		}
	}
}
