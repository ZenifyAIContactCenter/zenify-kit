// internal/e2e/lint_test.go
package e2e

import (
	"strings"
	"testing"
)

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
	if fs := LintSource("ok.spec.ts", passing, nil); len(fs) != 0 {
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
	if !rules(LintSource("a.spec.ts", src, nil))["refetch"] {
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
	if !rules(LintSource("a.spec.ts", src, nil))["refetch"] {
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
	if !rules(LintSource("a.spec.ts", src, nil))["marker"] {
		t.Fatal("test.only missing @domain-assert must be caught — must not bypass")
	}
}

// I-3b: a *.spec.ts with no test(...) scenario at all must not be read as clean.
func TestLint_ZeroTestBlocksFlagged(t *testing.T) {
	src := `import { test } from '../../fixtures';
// every scenario is commented out
const helper = 1;`
	if !rules(LintSource("empty.spec.ts", src, nil))["no-test"] {
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
	if !rules(LintSource("a.spec.ts", src, nil))["no-networkidle"] {
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
	if !rules(LintSource("a.spec.ts", src, nil))["cleanup"] {
		t.Fatal("missing cleanup must be caught")
	}
}

func TestLint_MissingMarker(t *testing.T) {
	src := `import { test } from '../../fixtures';
test('x [FR-1]', async ({ page, apiClient, cleanupTracker }) => {
  const r = await apiClient.get('/v2/ticket/1'); expect(r.ok()).toBeTruthy();
  cleanupTracker.add(async () => {});
});`
	if !rules(LintSource("a.spec.ts", src, nil))["marker"] {
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
	if !rules(LintSource("a.spec.ts", src, nil))["traceability"] {
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
	if !rules(LintSource("a.spec.ts", src, nil))["marker"] {
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
	if !rules(LintSource("a.spec.ts", src, nil))["no-hardwait"] {
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
	if !rules(LintSource("a.spec.ts", xpath, nil))["no-fragile-selector"] {
		t.Fatal("xpath= must be caught")
	}

	nthChild := `import { test } from '../../fixtures';
test('x [FR-1]', async ({ page, apiClient, cleanupTracker }) => {
  await page.locator('li:nth-child(3)').click();
  // @domain-assert:ticket
  const r = await apiClient.get('/v2/ticket/1'); expect(r.ok()).toBeTruthy();
  cleanupTracker.add(async () => {});
});`
	if !rules(LintSource("a.spec.ts", nthChild, nil))["no-fragile-selector"] {
		t.Fatal("nth-child( must be caught")
	}
}

func scenarioFor(t *testing.T, src string) map[string]Scenario {
	t.Helper()
	sc, err := ParseScenario(src)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]Scenario{sc.Key: sc}
}

const scenTwoE2E = "# ZEN-9: t\nVai: a\n\n## @ac:1 [e2e] one\nThấy:\n- Vui lòng nhập tiêu đề\n\n## @ac:2 [e2e] two\n\n## @ac:3 [ngoài-e2e] three\n" //znf:allow-lang

func journey(header, title, body string) string {
	return header + "\nimport { test, expect } from '../../fixtures';\ntest('" + title + "', async ({ page, apiClient, cleanupTracker }) => {\n" + body + `
  // @domain-assert:ticket
  const r = await apiClient.get('/v2/ticket/abc');
  expect((await r.json()).subject).toBe(subject);
  cleanupTracker.add(async () => {});
});
`
}

func TestLintScenarioRules(t *testing.T) {
	const hdr = "// @scenario ZEN-9"
	cases := []struct {
		name  string
		src   string
		scens map[string]Scenario
		want  string // rule expected exactly once; "" = no scenario-rule/traceability finding
		not   string // rule that must be absent
	}{
		{"scenario-link missing", journey(hdr, "x @ac:1", ""), map[string]Scenario{}, "scenario-link", ""},
		{"ac-coverage missing ac2", journey(hdr, "x @ac:1", ""), nil, "ac-coverage", ""},
		{"expect-literal bad", journey(hdr, "x @ac:1 @ac:2", "  await expect(page.getByRole('alert')).toHaveText('Sai chữ');"), nil, "expect-literal", ""},              //znf:allow-lang
		{"expect-literal ok", journey(hdr, "x @ac:1 @ac:2", "  await expect(page.getByRole('alert')).toHaveText('Vui lòng nhập tiêu đề');"), nil, "", "expect-literal"}, //znf:allow-lang
		{"expect var ok", journey(hdr, "x @ac:1 @ac:2", ""), nil, "", "expect-literal"},
		{"no-skip skip", journey(hdr, "x @ac:1 @ac:2", "  test.skip(true, 'later');"), nil, "no-skip", ""},
		{"no-skip fixme", journey(hdr, "x @ac:1 @ac:2", "  test.fixme(true);"), nil, "no-skip", ""},
		{"no-skip only", journey(hdr, "x @ac:1 @ac:2", "  test.only(true);"), nil, "no-skip", ""},
		{"ac tag replaces FR", journey(hdr, "x @ac:1 @ac:2", ""), nil, "", "traceability"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			scens := c.scens
			if scens == nil {
				scens = scenarioFor(t, scenTwoE2E)
			}
			fs := LintSource("zen-9.spec.ts", c.src, scens)
			got := map[string]int{}
			for _, f := range fs {
				got[f.Rule]++
			}
			if c.want != "" && got[c.want] != 1 {
				t.Fatalf("want exactly one %s, got %+v", c.want, fs)
			}
			if c.not != "" && got[c.not] != 0 {
				t.Fatalf("%s must be absent, got %+v", c.not, fs)
			}
			if c.want == "" {
				for _, r := range []string{"scenario-link", "ac-coverage", "expect-literal", "no-skip"} {
					if got[r] != 0 {
						t.Fatalf("unexpected %s: %+v", r, fs)
					}
				}
			}
		})
	}

	t.Run("ac-coverage names the uncovered ac", func(t *testing.T) {
		fs := LintSource("zen-9.spec.ts", journey(hdr, "x @ac:1", ""), scenarioFor(t, scenTwoE2E))
		for _, f := range fs {
			if f.Rule == "ac-coverage" {
				if !strings.Contains(f.Msg, "@ac:2") || strings.Contains(f.Msg, "@ac:3") {
					t.Fatalf("msg should name @ac:2 only: %s", f.Msg)
				}
				return
			}
		}
		t.Fatal("no ac-coverage finding")
	})

	t.Run("without @scenario behaviour is unchanged", func(t *testing.T) {
		src := journey("", "x", "  test.skip(true);")
		got := rules(LintSource("a.spec.ts", src, nil))
		if !got["traceability"] || got["no-skip"] || got["scenario-link"] || got["ac-coverage"] {
			t.Fatalf("legacy journey must keep traceability only, got %+v", got)
		}
	})
}
