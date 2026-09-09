// internal/e2e/lint_test.go
package e2e

import "testing"

const passing = `
import { test, expect } from '../../fixtures';
test('create ticket [FR-6]', async ({ page, apiClient, cleanupTracker }) => {
  await page.getByRole('button', { name: 'Create new' }).click();
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
		t.Fatalf("clean spec must have 0 findings, got %+v", fs)
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
		t.Fatal("missing apiClient re-fetch must be caught")
	}
}

// I-2: apiClient appearing only in the cleanup closure (no actual re-fetch/domain assertion)
// must NOT count as "has re-fetch" — if it did, this shallow test would still pass the rule.
func TestLint_CleanupOnlyApiClientFails(t *testing.T) {
	src := `import { test, expect } from '../../fixtures';
test('shallow [FR-1]', async ({ page, apiClient, cleanupTracker }) => {
  await page.getByTestId('submit').click();
  // @domain-assert:ticket
  expect(1).toBe(1);
  cleanupTracker.add(async () => { await apiClient.put('/v2/ticket/x', { data: { is_deleted: true } }); });
});`
	if !rules(LintSource("a.spec.ts", src))["refetch"] {
		t.Fatal("apiClient only inside cleanup closure — must be caught as missing refetch")
	}
}

// I-3: a test.only(...) scenario must be scanned like a normal test — must not bypass the gate.
func TestLint_TestOnlyIsLinted(t *testing.T) {
	src := `import { test } from '../../fixtures';
test.only('x [FR-1]', async ({ page, apiClient, cleanupTracker }) => {
  const r = await apiClient.get('/v2/ticket/1'); expect(r.ok()).toBeTruthy();
  cleanupTracker.add(async () => {});
});`
	// missing // @domain-assert marker → must be caught (proves test.only is scanned)
	if !rules(LintSource("a.spec.ts", src))["marker"] {
		t.Fatal("test.only missing @domain-assert must be caught — must not bypass")
	}
}

// I-3b: a *.spec.ts with no test(...) scenario at all must not be read as clean.
func TestLint_ZeroTestBlocksFlagged(t *testing.T) {
	src := `import { test } from '../../fixtures';
// every scenario is commented out
const helper = 1;`
	if !rules(LintSource("empty.spec.ts", src))["no-test"] {
		t.Fatal("*.spec.ts with no test(...) must be caught as no-test")
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
		t.Fatal("networkidle must be caught")
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
		t.Fatal("missing cleanup must be caught")
	}
}

func TestLint_MissingMarker(t *testing.T) {
	src := `import { test } from '../../fixtures';
test('x [FR-1]', async ({ page, apiClient, cleanupTracker }) => {
  const r = await apiClient.get('/v2/ticket/1'); expect(r.ok()).toBeTruthy();
  cleanupTracker.add(async () => {});
});`
	if !rules(LintSource("a.spec.ts", src))["marker"] {
		t.Fatal("missing @domain-assert must be caught")
	}
}

func TestLint_MissingTraceability(t *testing.T) {
	src := `import { test } from '../../fixtures';
test('no ref', async ({ page, apiClient, cleanupTracker }) => {
  // @domain-assert:ticket
  const r = await apiClient.get('/v2/ticket/1'); expect(r.ok()).toBeTruthy();
  cleanupTracker.add(async () => {});
});`
	if !rules(LintSource("a.spec.ts", src))["traceability"] {
		t.Fatal("missing FR/SC ref must be caught")
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
		t.Fatal("two @domain-assert in one scenario must be caught")
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
		t.Fatal("waitForTimeout must be caught")
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
		t.Fatal("xpath= must be caught")
	}

	nthChild := `import { test } from '../../fixtures';
test('x [FR-1]', async ({ page, apiClient, cleanupTracker }) => {
  await page.locator('li:nth-child(3)').click();
  // @domain-assert:ticket
  const r = await apiClient.get('/v2/ticket/1'); expect(r.ok()).toBeTruthy();
  cleanupTracker.add(async () => {});
});`
	if !rules(LintSource("a.spec.ts", nthChild))["no-fragile-selector"] {
		t.Fatal("nth-child( must be caught")
	}
}
