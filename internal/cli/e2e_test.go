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
		t.Fatalf("thiếu spec phải BadArgs, got %v", err)
	}
}

func TestRunE2eLint_Violations_Fail(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "x.spec.ts"),
		[]byte("test('x', async ({page}) => { await page.waitForTimeout(1); });"), 0o644)
	err := runE2eLint(dir, &bytes.Buffer{})
	if exitcode.Code(err) != exitcode.Fail {
		t.Fatalf("vi phạm phải Fail, got %v", err)
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
		t.Fatalf("spec sạch phải OK, got %v", err)
	}
}
