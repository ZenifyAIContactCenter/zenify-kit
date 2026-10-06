package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

func TestRunE2eLint_NoSpecs_BadArgs(t *testing.T) {
	dir := t.TempDir()
	err := runE2eLint(dir, &bytes.Buffer{})
	if exitcode.Code(err) != exitcode.BadArgs {
		t.Fatalf("missing spec should be BadArgs, got %v", err)
	}
}

func TestRunE2eLint_Violations_Fail(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "x.spec.ts"),
		[]byte("test('x', async ({page}) => { await page.waitForTimeout(1); });"), 0o644)
	err := runE2eLint(dir, &bytes.Buffer{})
	if exitcode.Code(err) != exitcode.Fail {
		t.Fatalf("violation should be Fail, got %v", err)
	}
}

func TestRunE2eLint_Clean_OK(t *testing.T) {
	dir := t.TempDir()
	clean := `test('ok [FR-1]', async ({page, apiClient, cleanupTracker}) => {
  await page.getByRole('button').click();
  // @domain-assert:ticket
  const r = await apiClient.get('/v2/ticket/1'); expect((await r.json()).subject).toBe('x');
  cleanupTracker.add(async () => {});
});`
	os.WriteFile(filepath.Join(dir, "ok.spec.ts"), []byte(clean), 0o644)
	if err := runE2eLint(dir, &bytes.Buffer{}); err != nil {
		t.Fatalf("clean spec should be OK, got %v", err)
	}
}

func TestE2eLogin_NeedsExactlyOneOfUrlPort(t *testing.T) {
	for _, args := range [][]string{{"login"}, {"login", "--url", "x", "--port", "1"}} {
		c := newE2eCmd()
		c.SetArgs(args)
		c.SetOut(&bytes.Buffer{})
		c.SetErr(&bytes.Buffer{})
		if err := c.Execute(); exitcode.Code(err) != exitcode.BadArgs {
			t.Fatalf("%v: want BadArgs, got %v", args, err)
		}
	}
}

func TestE2eLogin_PortOutOfRange(t *testing.T) {
	for _, p := range []string{"-1", "70000"} {
		c := newE2eCmd()
		c.SetArgs([]string{"login", "--port", p})
		c.SetOut(&bytes.Buffer{})
		c.SetErr(&bytes.Buffer{})
		if err := c.Execute(); exitcode.Code(err) != exitcode.BadArgs {
			t.Fatalf("--port %s: want BadArgs, got %v", p, err)
		}
	}
}
