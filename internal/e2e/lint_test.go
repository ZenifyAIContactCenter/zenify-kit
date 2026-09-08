// internal/e2e/lint_test.go
package e2e

import "testing"

const passing = `
import { test, expect } from '../../fixtures';
test('tạo ticket [FR-6]', async ({ page, apiClient, cleanupTracker }) => {
  await page.getByRole('button', { name: 'Tạo mới' }).click();
  // @domain-assert:ticket
  const r = await apiClient.get('/v2/ticket/abc');
  expect((await r.json()).subject).toBe('x');
  cleanupTracker.add(async () => { await apiClient.put('/v2/ticket/abc', { data: { is_deleted: true } }); });
});
`

func rules(findings []Finding) map[string]bool {
	m := map[string]bool{}
	for _, f := range findings {
		m[f.Rule] = true
	}
	return m
}

func TestLint_PassingCleans(t *testing.T) {
	if fs := LintSource("ok.spec.ts", passing); len(fs) != 0 {
		t.Fatalf("spec sạch phải 0 finding, got %+v", fs)
	}
}

func TestLint_MissingRefetch(t *testing.T) {
	src := `import { test, expect } from '../../fixtures';
test('x [FR-1]', async ({ page, cleanupTracker }) => {
  await page.click('text=go');
  // @domain-assert:ticket
  await expect(page).toHaveURL(/ok/);
  cleanupTracker.add(async () => {});
});`
	if !rules(LintSource("a.spec.ts", src))["refetch"] {
		t.Fatal("thiếu apiClient re-fetch phải bị bắt")
	}
}

// I-2: apiClient chỉ xuất hiện trong closure cleanup (không hề re-fetch/assert domain)
// KHÔNG được tính là "có re-fetch" — nếu tính, test shallow này vẫn qua rule.
func TestLint_CleanupOnlyApiClientFails(t *testing.T) {
	src := `import { test, expect } from '../../fixtures';
test('shallow [FR-1]', async ({ page, apiClient, cleanupTracker }) => {
  await page.getByTestId('submit').click();
  // @domain-assert:ticket
  expect(1).toBe(1);
  cleanupTracker.add(async () => { await apiClient.put('/v2/ticket/x', { data: { is_deleted: true } }); });
});`
	if !rules(LintSource("a.spec.ts", src))["refetch"] {
		t.Fatal("apiClient chỉ nằm trong cleanup closure — phải bị bắt refetch")
	}
}

// I-3: scenario test.only(...) phải được soi như test thường, không được bypass gate.
func TestLint_TestOnlyIsLinted(t *testing.T) {
	src := `import { test } from '../../fixtures';
test.only('x [FR-1]', async ({ page, apiClient, cleanupTracker }) => {
  const r = await apiClient.get('/v2/ticket/1'); expect(r.ok()).toBeTruthy();
  cleanupTracker.add(async () => {});
});`
	// thiếu // @domain-assert marker → phải bị bắt (chứng minh test.only được soi)
	if !rules(LintSource("a.spec.ts", src))["marker"] {
		t.Fatal("test.only thiếu @domain-assert phải bị bắt — không được bypass")
	}
}

// I-3b: một *.spec.ts không có scenario test(...) nào không được đọc là sạch.
func TestLint_ZeroTestBlocksFlagged(t *testing.T) {
	src := `import { test } from '../../fixtures';
// mọi scenario bị comment hết
const helper = 1;`
	if !rules(LintSource("empty.spec.ts", src))["no-test"] {
		t.Fatal("*.spec.ts không có test(...) phải bị bắt no-test")
	}
}

func TestLint_Networkidle(t *testing.T) {
	src := `import { test } from '../../fixtures';
test('x [SC-1]', async ({ page, apiClient, cleanupTracker }) => {
  await page.waitForLoadState('networkidle');
  // @domain-assert:ticket
  const r = await apiClient.get('/v2/ticket/1'); expect(r.ok()).toBeTruthy();
  cleanupTracker.add(async () => {});
});`
	if !rules(LintSource("a.spec.ts", src))["no-networkidle"] {
		t.Fatal("networkidle phải bị bắt")
	}
}

func TestLint_MissingCleanup(t *testing.T) {
	src := `import { test } from '../../fixtures';
test('x [FR-1]', async ({ page, apiClient }) => {
  await page.click('text=go');
  // @domain-assert:ticket
  const r = await apiClient.get('/v2/ticket/1'); expect(r.ok()).toBeTruthy();
});`
	if !rules(LintSource("a.spec.ts", src))["cleanup"] {
		t.Fatal("thiếu cleanup phải bị bắt")
	}
}

func TestLint_MissingMarker(t *testing.T) {
	src := `import { test } from '../../fixtures';
test('x [FR-1]', async ({ page, apiClient, cleanupTracker }) => {
  const r = await apiClient.get('/v2/ticket/1'); expect(r.ok()).toBeTruthy();
  cleanupTracker.add(async () => {});
});`
	if !rules(LintSource("a.spec.ts", src))["marker"] {
		t.Fatal("thiếu @domain-assert phải bị bắt")
	}
}

func TestLint_MissingTraceability(t *testing.T) {
	src := `import { test } from '../../fixtures';
test('không ref', async ({ page, apiClient, cleanupTracker }) => {
  // @domain-assert:ticket
  const r = await apiClient.get('/v2/ticket/1'); expect(r.ok()).toBeTruthy();
  cleanupTracker.add(async () => {});
});`
	if !rules(LintSource("a.spec.ts", src))["traceability"] {
		t.Fatal("thiếu FR/SC ref phải bị bắt")
	}
}

func TestLint_MultipleMarkers(t *testing.T) {
	src := `import { test } from '../../fixtures';
test('x [FR-1]', async ({ page, apiClient, cleanupTracker }) => {
  // @domain-assert:ticket
  const r = await apiClient.get('/v2/ticket/1'); expect(r.ok()).toBeTruthy();
  // @domain-assert:ticket
  const r2 = await apiClient.get('/v2/ticket/2'); expect(r2.ok()).toBeTruthy();
  cleanupTracker.add(async () => {});
});`
	if !rules(LintSource("a.spec.ts", src))["marker"] {
		t.Fatal("hai @domain-assert trong một scenario phải bị bắt")
	}
}

func TestLint_HardWait(t *testing.T) {
	src := `import { test } from '../../fixtures';
test('x [FR-1]', async ({ page, apiClient, cleanupTracker }) => {
  await page.waitForTimeout(2000);
  // @domain-assert:ticket
  const r = await apiClient.get('/v2/ticket/1'); expect(r.ok()).toBeTruthy();
  cleanupTracker.add(async () => {});
});`
	if !rules(LintSource("a.spec.ts", src))["no-hardwait"] {
		t.Fatal("waitForTimeout phải bị bắt")
	}
}

func TestLint_FragileSelector(t *testing.T) {
	xpath := `import { test } from '../../fixtures';
test('x [FR-1]', async ({ page, apiClient, cleanupTracker }) => {
  await page.locator('xpath=//button').click();
  // @domain-assert:ticket
  const r = await apiClient.get('/v2/ticket/1'); expect(r.ok()).toBeTruthy();
  cleanupTracker.add(async () => {});
});`
	if !rules(LintSource("a.spec.ts", xpath))["no-fragile-selector"] {
		t.Fatal("xpath= phải bị bắt")
	}

	nthChild := `import { test } from '../../fixtures';
test('x [FR-1]', async ({ page, apiClient, cleanupTracker }) => {
  await page.locator('li:nth-child(3)').click();
  // @domain-assert:ticket
  const r = await apiClient.get('/v2/ticket/1'); expect(r.ok()).toBeTruthy();
  cleanupTracker.add(async () => {});
});`
	if !rules(LintSource("a.spec.ts", nthChild))["no-fragile-selector"] {
		t.Fatal("nth-child( phải bị bắt")
	}
}
