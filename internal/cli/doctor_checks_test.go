package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
)

func noManifest() *manifest.Manifest { return nil }

func fixedManifest(m *manifest.Manifest) func() *manifest.Manifest {
	return func() *manifest.Manifest { return m }
}

func writeEnv(t *testing.T, body string, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(p, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
	return p
}

// FR-8.1: a global manifest key missing from env and file is named; a repo-only key is not.
func TestDoctor_MissingManifestKeys(t *testing.T) {
	m := &manifest.Manifest{
		SecretKeys: []manifest.SecretKey{{Name: "MONGO_URL", Secret: true}, {Name: "E2E_EMAIL", Secret: true}, {Name: "POSTHOG_API_KEY", Secret: true}},
		Repos:      []manifest.Repo{{Name: "lumi-agent", SecretKeys: []manifest.SecretKey{{Name: "OPENROUTER_API_KEY", Secret: true}}}},
	}
	p := writeEnv(t, "MONGO_URL='mongodb://fake'\nE2E_EMAIL=a@b.c\n", 0o600)
	ok, detail := secretPresenceCheck(func(string) string { return "" }, func() string { return p }, fixedManifest(m)).Run()
	if !ok {
		t.Fatalf("MONGO_URL present, want ok; detail=%q", detail)
	}
	for _, want := range []string{"MONGO_URL=present", "E2E_EMAIL=present", "POSTHOG_API_KEY=absent"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail %q lacks %q", detail, want)
		}
	}
	if strings.Contains(detail, "OPENROUTER_API_KEY") {
		t.Errorf("repo-scoped key must not be reported: %q", detail)
	}
}

// SC-10 / FR-8.2: a local key (not in the manifest) with no description line is named.
func TestDoctor_LocalKeyWithoutDesc(t *testing.T) {
	m := &manifest.Manifest{SecretKeys: []manifest.SecretKey{{Name: "MONGO_URL", Secret: true}}}
	p := writeEnv(t, "MONGO_URL=x\n# my own token\nDESCRIBED_KEY=1\nBARE_KEY=2\n", 0o600)
	ok, detail := envFileCheck(func() string { return p }, fixedManifest(m)).Run()
	if ok {
		t.Fatalf("a local key without description must flag; detail=%q", detail)
	}
	if !strings.Contains(detail, "BARE_KEY") {
		t.Errorf("detail should name BARE_KEY: %q", detail)
	}
	if strings.Contains(detail, "DESCRIBED_KEY") || strings.Contains(detail, "MONGO_URL") {
		t.Errorf("described / manifest keys must not be flagged: %q", detail)
	}
}

// FR-8.4: a file readable by group/other is flagged.
func TestDoctor_EnvFilePerm(t *testing.T) {
	m := &manifest.Manifest{SecretKeys: []manifest.SecretKey{{Name: "MONGO_URL", Secret: true}}}
	wide := writeEnv(t, "MONGO_URL=x\n", 0o644)
	ok, detail := envFileCheck(func() string { return wide }, fixedManifest(m)).Run()
	if ok || !strings.Contains(detail, "0644") {
		t.Fatalf("mode 0644 must flag; ok=%v detail=%q", ok, detail)
	}
	tight := writeEnv(t, "MONGO_URL=x\n", 0o600)
	if ok, detail := envFileCheck(func() string { return tight }, fixedManifest(m)).Run(); !ok {
		t.Fatalf("mode 0600 must pass; detail=%q", detail)
	}
}

func TestSecretPresenceNamesOnlyNeverLeaksValue(t *testing.T) {
	getenv := func(k string) string {
		if k == "MONGO_URL" {
			return "mongodb://secretuser:secretpass@10.0.0.1:27017"
		}
		return ""
	}
	c := secretPresenceCheck(getenv, func() string { return "/nonexistent/.env" }, noManifest)
	ok, detail := c.Run()
	if strings.Contains(detail, "secretpass") || strings.Contains(detail, "10.0.0.1") || strings.Contains(detail, "mongodb://") {
		t.Fatalf("FR-041 VIOLATION: secret value leaked into detail: %q", detail)
	}
	if !strings.Contains(detail, "MONGO_URL") {
		t.Fatalf("detail should name the key MONGO_URL: %q", detail)
	}
	_ = ok
}

func TestSecretPresenceReportsAbsent(t *testing.T) {
	c := secretPresenceCheck(func(string) string { return "" }, func() string { return "/nonexistent/.env" }, noManifest)
	ok, detail := c.Run()
	if ok {
		t.Fatal("no MONGO_URL anywhere → check should be not-ok")
	}
	if !strings.Contains(detail, "MONGO_URL") {
		t.Fatalf("should name the missing key: %q", detail)
	}
}

// F3: an exported MONGO_URL must count even when no workspace resolves
// (envPath returns "") — LoadCreds reads env first regardless of path.
func TestSecretPresenceCheck_EnvMongoUrlCountsWithNoWorkspace(t *testing.T) {
	getenv := func(k string) string {
		if k == "MONGO_URL" {
			return "mongodb://x"
		}
		return ""
	}
	c := secretPresenceCheck(getenv, func() string { return "" }, noManifest)
	ok, detail := c.Run()
	if !ok {
		t.Fatalf("MONGO_URL is set in env, ok should be true; detail=%q", detail)
	}
	if strings.Contains(detail, "no workspace") {
		t.Fatalf("MONGO_URL present should not append the no-workspace hint: %q", detail)
	}
}

// F3: with neither env nor a workspace, the check stays not-ok and now
// explains why via the no-workspace hint (previously an early return with a
// fixed message that ignored env entirely).
func TestSecretPresenceCheck_NoEnvNoWorkspace(t *testing.T) {
	c := secretPresenceCheck(func(string) string { return "" }, func() string { return "" }, noManifest)
	ok, detail := c.Run()
	if ok {
		t.Fatal("no MONGO_URL and no workspace → check should be not-ok")
	}
	if !strings.Contains(detail, "no workspace") {
		t.Fatalf("detail should explain no workspace: %q", detail)
	}
}

func TestToolPresenceReportsMissing(t *testing.T) {
	c := toolPresenceCheck([]string{"this-binary-does-not-exist-zzz"})
	ok, detail := c.Run()
	if ok {
		t.Fatal("a nonexistent tool should make the check not-ok")
	}
	if !strings.Contains(detail, "this-binary-does-not-exist-zzz") {
		t.Fatalf("should name the missing tool: %q", detail)
	}
}

func TestPlaywrightCheckIsReadOnlyRegistered(t *testing.T) {
	c := playwrightCheck()
	if c.Name != "playwright" {
		t.Fatalf("name: %q", c.Name)
	}
	// Run must not panic and must return a detail mentioning mcp state.
	_, detail := c.Run()
	if !strings.Contains(detail, "mcp=") {
		t.Fatalf("detail should report mcp state: %q", detail)
	}
}

func TestPluginCheckReportsMissing(t *testing.T) {
	c := pluginCheckAt(t.TempDir()) // empty dest → not synced
	ok, msg := c.Run()
	if ok {
		t.Fatal("plugin not synced but reports ok")
	}
	if !strings.Contains(msg, "chưa cài") { //znf:allow-lang
		t.Fatalf("msg should state the status: %q", msg)
	}
}

func TestDockerCheck_PresentAndRunning(t *testing.T) {
	c := dockerCheckWith(
		func(string) (string, error) { return "/usr/bin/docker", nil },
		func() error { return nil },
	)
	ok, detail := c.Run()
	if !ok {
		t.Errorf("docker present + info ok should be OK, detail=%q", detail)
	}
}

func TestDockerCheck_Missing(t *testing.T) {
	c := dockerCheckWith(
		func(string) (string, error) { return "", errors.New("not found") },
		func() error { return nil },
	)
	ok, detail := c.Run()
	if ok || !strings.Contains(detail, "docker=missing") {
		t.Errorf("docker missing should be !ok + detail containing docker=missing, got ok=%v detail=%q", ok, detail)
	}
}

// The git-guard check must fail on a fresh HOME, its Fix must wire the hook,
// and the check must then pass — the path `doctor --fix` takes.
func TestGitGuardCheck_MissingThenFixed(t *testing.T) {
	home := t.TempDir()
	c := gitGuardCheck(func() (string, error) { return home, nil })
	if ok, detail := c.Run(); ok || !strings.Contains(detail, "hook=missing") {
		t.Fatalf("fresh HOME should report hook=missing, got ok=%v detail=%q", ok, detail)
	}
	if ok, detail := c.Fix(); !ok {
		t.Fatalf("Fix failed: %s", detail)
	}
	if ok, detail := c.Run(); !ok {
		t.Fatalf("after Fix the check should pass, detail=%q", detail)
	}
}

func TestGitIdentityCheck_Unknown(t *testing.T) {
	c := gitIdentityCheckWith(func(v string) bool { return v == "GIT_AUTHOR_IDENT" })
	ok, detail := c.Run()
	if ok || !strings.Contains(detail, "GIT_COMMITTER_IDENT=unknown") || strings.Contains(detail, "GIT_AUTHOR_IDENT") {
		t.Errorf("want !ok naming only the committer, got ok=%v detail=%q", ok, detail)
	}
}

func TestGitIdentityCheck_Present(t *testing.T) {
	c := gitIdentityCheckWith(func(string) bool { return true })
	if ok, detail := c.Run(); !ok {
		t.Errorf("name+email set should be OK, detail=%q", detail)
	}
}

func TestKitPermissionsCheck_MissingThenFixed(t *testing.T) {
	home := t.TempDir()
	c := kitPermissionsCheck(func() (string, error) { return home, nil })
	if ok, detail := c.Run(); ok || !strings.Contains(detail, "missing") {
		t.Fatalf("empty HOME should report missing, got ok=%v detail=%q", ok, detail)
	}
	if ok, detail := c.Fix(); !ok {
		t.Fatalf("Fix failed: %s", detail)
	}
	if ok, detail := c.Run(); !ok {
		t.Fatalf("after Fix the check should pass, detail=%q", detail)
	}
}
