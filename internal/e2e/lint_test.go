// internal/e2e/lint_test.go
package e2e

import (
	"os"
	"path/filepath"
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
		{"getByText copy checked", journey(hdr, "x @ac:1 @ac:2", "  await expect(page.getByText('Sai chữ')).toBeVisible();"), nil, "expect-literal", ""},                //znf:allow-lang
		{"getByRole role only ok", journey(hdr, "x @ac:1 @ac:2", "  await expect(page.getByRole('alert')).toBeVisible();"), nil, "", "expect-literal"},
		{"getByRole name checked", journey(hdr, "x @ac:1 @ac:2", "  await expect(page.getByRole('button', { name: 'Lưu' })).toBeVisible();"), nil, "expect-literal", ""},         //znf:allow-lang
		{"getByLabel checked", journey(hdr, "x @ac:1 @ac:2", "  await expect(page.getByLabel('Nhãn lạ')).toBeVisible();"), nil, "expect-literal", ""},                            //znf:allow-lang
		{"filter hasText checked", journey(hdr, "x @ac:1 @ac:2", "  await expect(page.locator('tr').filter({ hasText: 'Dòng lạ' })).toBeVisible();"), nil, "expect-literal", ""}, //znf:allow-lang
		{"getByTestId exempt", journey(hdr, "x @ac:1 @ac:2", "  await expect(page.getByTestId('save-btn')).toBeVisible();"), nil, "", "expect-literal"},
		{"paren inside literal", journey(hdr, "x @ac:1 @ac:2", "  await expect(page.getByText('Tạo (mới)')).toBeVisible();"), nil, "expect-literal", ""}, //znf:allow-lang
		{"unknown ac number", journey(hdr, "x @ac:1 @ac:2 @ac:7", ""), nil, "ac-coverage", ""},
		{"expect var ok", journey(hdr, "x @ac:1 @ac:2", ""), nil, "", "expect-literal"},
		{"no-skip skip", journey(hdr, "x @ac:1 @ac:2", "  test.skip(true, 'later');"), nil, "no-skip", ""},
		{"no-skip fixme", journey(hdr, "x @ac:1 @ac:2", "  test.fixme(true);"), nil, "no-skip", ""},
		{"no-skip only", journey(hdr, "x @ac:1 @ac:2", "  test.only(true);"), nil, "no-skip", ""},
		{"ac tag replaces FR", journey(hdr, "x @ac:1 @ac:2", ""), nil, "", "traceability"},
		{"no-skip describe.skip", journey(hdr, "x @ac:1 @ac:2", "  test.describe.skip('later', () => {});"), nil, "no-skip", ""},
		{"no-skip describe.serial.only", journey(hdr, "x @ac:1 @ac:2", "  test.describe.serial.only('later', () => {});"), nil, "no-skip", ""},
		{"no-skip describe.fixme", journey(hdr, "x @ac:1 @ac:2", "  test.describe.fixme('later', () => {});"), nil, "no-skip", ""},
		{"no-skip fail", journey(hdr, "x @ac:1 @ac:2", "  test.fail();"), nil, "no-skip", ""},
		{"expect.soft checked", journey(hdr, "x @ac:1 @ac:2", "  await expect.soft(page.getByText('Sai chữ')).toBeVisible();"), nil, "expect-literal", ""},                           //znf:allow-lang
		{"expect.poll checked", journey(hdr, "x @ac:1 @ac:2", "  await expect.poll(async () => page.getByRole('alert').textContent()).toBe('Sai chữ');"), nil, "expect-literal", ""}, //znf:allow-lang
		{"expect.poll api path ok", journey(hdr, "x @ac:1 @ac:2", "  await expect.poll(async () => (await apiClient.get('/v2/ticket/abc')).status()).toBe(200);"), nil, "", "expect-literal"},
		{"regex matcher flagged", journey(hdr, "x @ac:1 @ac:2", "  await expect(page.getByRole('alert')).toHaveText(/.*/);"), nil, "expect-literal", ""},
		{"regex getByText flagged", journey(hdr, "x @ac:1 @ac:2", "  await expect(page.getByText(/Vui/)).toBeVisible();"), nil, "expect-literal", ""},
		{"regex name flagged", journey(hdr, "x @ac:1 @ac:2", "  await expect(page.getByRole('button', { name: /L/ })).toBeVisible();"), nil, "expect-literal", ""},
		{"regex hasText flagged", journey(hdr, "x @ac:1 @ac:2", "  await expect(page.locator('tr').filter({ hasText: /./ })).toBeVisible();"), nil, "expect-literal", ""},
		{"toHaveURL regex ok", journey(hdr, "x @ac:1 @ac:2", "  await expect(page).toHaveURL(/ticket/);"), nil, "", "expect-literal"},
		{"template static part checked", journey(hdr, "x @ac:1 @ac:2", "  await expect(page.getByText(`${''}Sai chữ`)).toBeVisible();"), nil, "expect-literal", ""}, //znf:allow-lang
		{"template static part ok", journey(hdr, "x @ac:1 @ac:2", "  await expect(page.getByText(`Vui lòng ${x}`)).toBeVisible();"), nil, "", "expect-literal"},     //znf:allow-lang
		{"locator text= checked", journey(hdr, "x @ac:1 @ac:2", "  await expect(page.locator('text=Other copy')).toBeVisible();"), nil, "expect-literal", ""},
		{"locator has-text checked", journey(hdr, "x @ac:1 @ac:2", "  await expect(page.locator('button:has-text(\"Sai chữ\")')).toBeVisible();"), nil, "expect-literal", ""},       //znf:allow-lang
		{"locator has-text ok", journey(hdr, "x @ac:1 @ac:2", "  await expect(page.locator('div:has-text(\"Vui lòng nhập tiêu đề\")')).toBeVisible();"), nil, "", "expect-literal"}, //znf:allow-lang
		{"locator internal:text checked", journey(hdr, "x @ac:1 @ac:2", "  await expect(page.locator('internal:text=\"Sai chữ\"i')).toBeVisible();"), nil, "expect-literal", ""},    //znf:allow-lang
		{"locator text-is checked", journey(hdr, "x @ac:1 @ac:2", "  await expect(page.locator('span:text-is(\"Sai chữ\")')).toBeVisible();"), nil, "expect-literal", ""},           //znf:allow-lang
		{"locator css ok", journey(hdr, "x @ac:1 @ac:2", "  await expect(page.locator('[data-text=foo] > .row')).toBeVisible();"), nil, "", "expect-literal"},
		{"url literal is not a comment", journey(hdr, "x @ac:1 @ac:2", "  await page.goto('https://x.test/'); test.skip(true);"), nil, "no-skip", ""},
		{"commented test does not cover", journey(hdr, "x @ac:1", "") + "// test('y @ac:2', async () => {\n//   // @domain-assert:ticket\n//   expect(apiClient.x).toBe(1);\n// });\n", nil, "ac-coverage", ""},
		{"block-commented test does not cover", journey(hdr, "x @ac:1", "") + "/*\ntest('y @ac:2', async () => {\n  // @domain-assert:ticket\n});\n*/\n", nil, "ac-coverage", ""},
		{"commented skip ignored", journey(hdr, "x @ac:1 @ac:2", "  // test.skip(true);"), nil, "", "no-skip"},
		{"apostrophe in title", strings.Replace(journey(hdr, "T", ""), "test('T'", "test(\"user's ticket @ac:1 @ac:2\"", 1), nil, "", "traceability"},
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

func TestLint_LoadsScenarios(t *testing.T) {
	write := func(t *testing.T, dir, rel, body string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run := func(dir string) string {
		var buf strings.Builder
		_, _ = Lint(dir, &buf)
		return buf.String()
	}
	t.Run("missing scenarios dir", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "a.spec.ts", passing)
		if out := run(dir); out != "" {
			t.Fatalf("legacy journey must lint clean, got %q", out)
		}
	})
	t.Run("malformed scenario", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "a.spec.ts", passing)
		write(t, dir, "scenarios/ZEN-7.md", "not a scenario\n")
		out := run(dir)
		if strings.Count(out, "[scenario-parse]") != 1 || !strings.Contains(out, "ZEN-7.md") || strings.Count(out, "\n") != 1 {
			t.Fatalf("want exactly one scenario-parse on ZEN-7.md, got %q", out)
		}
	})
	t.Run("filename and heading mismatch", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "a.spec.ts", passing)
		write(t, dir, "scenarios/ZEN-9.md", "# ZEN-8: t\nVai: a\n\n## @ac:1 [e2e] x\n")
		out := run(dir)
		if strings.Count(out, "[scenario-parse]") != 1 || !strings.Contains(out, "ZEN-9.md") {
			t.Fatalf("want scenario-parse on ZEN-9.md, got %q", out)
		}
	})
	t.Run("valid scenario is linked", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "zen-9.spec.ts", journey("// @scenario ZEN-9", "x @ac:1", ""))
		write(t, dir, "scenarios/ZEN-9.md", "# ZEN-9: t\nVai: a\n\n## @ac:1 [e2e] x\n")
		if out := run(dir); out != "" {
			t.Fatalf("want clean, got %q", out)
		}
	})
}
